package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/ai"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/notifications"
	"github.com/nmtan2001/chat-quality-agent/pkg"
	"gorm.io/gorm"
)

const (
	guestyAccountIDHeader = "X-Guesty-Account-ID"
)

var urgentCheckSemaphore = make(chan struct{}, 100)

// GuestyWebhookPayload represents the incoming webhook payload from Guesty.
type GuestyWebhookPayload struct {
	Event        string                 `json:"event"` // e.g., "reservation.messageReceived"
	Message      map[string]interface{} `json:"message"`
	Conversation map[string]interface{} `json:"conversation"`
	Reservation  map[string]interface{} `json:"reservation"`
	AccountID    string                 `json:"accountId"` // May be in payload or header
}

// GuestyWebhook handles incoming webhooks from Guesty via Svix.
func GuestyWebhook(c *gin.Context) {
	// Get raw body for signature verification
	bodyBytes, err := c.GetRawData()
	if err != nil {
		log.Printf("[Guesty Webhook] Failed to read body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	// Get config from context (injected by middleware)
	cfg := middleware.GetConfig(c)
	if cfg == nil {
		log.Printf("[Guesty Webhook] Config not available in context")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server configuration error"})
		return
	}

	// Verify Svix signature
	signature := c.GetHeader("svix-signature")
	timestamp := c.GetHeader("svix-timestamp")

	// If svix secret is configured, signature verification is REQUIRED
	if cfg.SvixSecret != "" {
		// Require both signature and timestamp when secret is configured
		if signature == "" || timestamp == "" {
			log.Printf("[Guesty Webhook] Signature verification required but not provided (svix-secret is configured)")
			c.JSON(http.StatusForbidden, gin.H{
				"error":  "signature verification required",
				"detail": "webhook signature verification is required when SVIX_SECRET is configured",
			})
			return
		}

		// Verify signature using ONLY the config secret (NEVER from headers)
		valid, err := pkg.VerifySvixSignature(string(bodyBytes), signature, timestamp, cfg.SvixSecret)
		if err != nil || !valid {
			log.Printf("[Guesty Webhook] Signature verification failed: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}

		// Verify timestamp to prevent replay attacks
		if err := pkg.VerifySvixTimestamp(timestamp, 60); err != nil {
			log.Printf("[Guesty Webhook] Timestamp verification failed: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid timestamp"})
			return
		}

		log.Printf("[Guesty Webhook] Signature verified successfully")
	} else {
		// No svix secret configured
		if cfg.IsProduction() {
			// In production, require signature verification
			log.Printf("[Guesty Webhook] SVIX_SECRET not configured in production - rejecting webhook")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  "webhook not configured",
				"detail": "SVIX_SECRET must be configured in production environment",
			})
			return
		}
		// In development, allow unsigned webhooks with warning
		log.Printf("[Guesty Webhook] WARNING: Processing webhook without signature verification (SVIX_SECRET not configured)")
	}

	var payload GuestyWebhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		log.Printf("[Guesty Webhook] Failed to parse payload: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	// Extract account ID from header or payload
	accountID := c.GetHeader(guestyAccountIDHeader)
	if accountID == "" && payload.AccountID != "" {
		accountID = payload.AccountID
	}

	log.Printf("[Guesty Webhook] Received event: %s for account: %s", payload.Event, accountID)

	// Handle message events
	if payload.Event == "reservation.messageReceived" || payload.Event == "reservation.messageSent" {
		if err := processMessageWebhook(payload, accountID); err != nil {
			log.Printf("[Guesty Webhook] Failed to process message: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "processing failed"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// processMessageWebhook persists the message and checks for urgent issues.
func processMessageWebhook(payload GuestyWebhookPayload, accountID string) error {
	// Extract message data
	messageID, _ := payload.Message["id"].(string)
	messageBody, _ := payload.Message["body"].(string)
	conversationID, _ := payload.Conversation["id"].(string)

	// Extract reservation data
	reservationID, _ := payload.Reservation["id"].(string)
	listingName := ""
	if listing, ok := payload.Reservation["listingNickname"].(string); ok {
		listingName = listing
	} else if listing, ok := payload.Reservation["listing"].(map[string]interface{}); ok {
		if nickname, ok := listing["nickname"].(string); ok {
			listingName = nickname
		}
	}

	guestName := ""
	if meta, ok := payload.Conversation["meta"].(map[string]interface{}); ok {
		guestName, _ = meta["guestName"].(string)
	}

	// Determine sender type
	senderType := "customer"
	if payload.Event == "reservation.messageSent" {
		senderType = "agent"
	}

	// Find tenant by Guesty account ID
	var channel models.Channel
	if err := db.DB.Where("channel_type = ? AND metadata->>'account_id' = ?", "guesty", accountID).First(&channel).Error; err != nil {
		log.Printf("[Guesty Webhook] No channel found for account %s: %v", accountID, err)
		return fmt.Errorf("channel not found for account_id: %s: %w", accountID, err)
	}

	// Capture values for goroutine before transaction
	var conversationIDForCheck string
	tenantIDForCheck := channel.TenantID

	// Execute all database operations in a single transaction
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		// Find or create conversation atomically to prevent race condition
		conversation := models.Conversation{
			TenantID:               channel.TenantID,
			ChannelID:              channel.ID,
			ExternalConversationID: conversationID,
			ExternalUserID:         reservationID,
			CustomerName:           guestName,
			Metadata:               mustMarshalJSON(map[string]interface{}{"reservation_id": reservationID, "listing_name": listingName}),
		}
		if err := tx.Where("tenant_id = ? AND external_conversation_id = ?", channel.TenantID, conversationID).
			FirstOrCreate(&conversation).Error; err != nil {
			return fmt.Errorf("find/create conversation: %w", err)
		}

		// Save conversation ID for urgent check goroutine
		conversationIDForCheck = conversation.ID

		// Create message with proper error checking
		message := models.Message{
			ID:                pkg.NewUUID(),
			TenantID:          channel.TenantID,
			ConversationID:    conversation.ID,
			ExternalMessageID: messageID,
			SenderType:        senderType,
			SenderName:        guestName,
			Content:           messageBody,
			ContentType:       "text",
			SentAt:            time.Now(),
		}
		if err := tx.Create(&message).Error; err != nil {
			return fmt.Errorf("create message: %w", err)
		}

		// Update conversation with proper error checking
		now := time.Now()
		if err := tx.Model(&conversation).Updates(map[string]interface{}{
			"last_message_at": &now,
			"message_count":   conversation.MessageCount + 1,
		}).Error; err != nil {
			return fmt.Errorf("update conversation: %w", err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	// Launch goroutine AFTER successful transaction commit with captured values
	if senderType == "customer" {
		// Capture variables by value to avoid race conditions
		guest := guestName
		msg := messageBody
		listing := listingName
		resID := reservationID

		// Try to acquire semaphore slot, skip if queue is full
		select {
		case urgentCheckSemaphore <- struct{}{}:
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[Urgent Check] Panic recovered: %v", r)
					}
					<-urgentCheckSemaphore
				}()

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				checkUrgentIssue(ctx, tenantIDForCheck, conversationIDForCheck, guest, msg, listing, resID)
			}()
		default:
			log.Printf("[Urgent Check] Semaphore queue full, skipping urgent check for conversation %s", conversationIDForCheck)
		}
	}

	return nil
}

// checkUrgentIssue uses AI to detect urgent issues in customer messages.
func checkUrgentIssue(ctx context.Context, tenantID, conversationID, guestName, message, listingName, reservationID string) {
	// Get AI provider from tenant settings
	var setting models.AppSetting
	if err := db.DB.WithContext(ctx).Where("tenant_id = ? AND key = ?", tenantID, "ai").First(&setting).Error; err != nil {
		log.Printf("[Urgent Check] No AI settings for tenant %s: %v", tenantID, err)
		return
	}

	var aiConfig struct {
		Provider string `json:"provider"`
		APIKey   string `json:"api_key"`
		Model    string `json:"model"`
	}
	if err := json.Unmarshal([]byte(setting.Value), &aiConfig); err != nil {
		log.Printf("[Urgent Check] Invalid AI config: %v", err)
		return
	}

	var provider ai.AIProvider

	switch aiConfig.Provider {
	case "claude":
		// Claude requires maxTokens parameter (use 4096 as default)
		provider = ai.NewClaudeProvider(aiConfig.APIKey, aiConfig.Model, 4096)
	case "gemini":
		provider = ai.NewGeminiProvider(aiConfig.APIKey, aiConfig.Model)
	default:
		log.Printf("[Urgent Check] Unsupported AI provider: %s", aiConfig.Provider)
		return
	}

	// Build urgency detection prompt (using centralized enhanced version)
	prompt := ai.BuildUrgencyDetectionPrompt()

	// Analyze message
	response, err := provider.AnalyzeChat(nil, prompt, message)
	if err != nil {
		log.Printf("[Urgent Check] AI analysis failed: %v", err)
		return
	}

	// Parse response using enhanced parsing with retry
	var result struct {
		IsUrgent   bool    `json:"is_urgent"`
		Category   string  `json:"category"`
		Severity   string  `json:"severity"`
		Confidence float64 `json:"confidence"`
		Summary    string  `json:"summary"`
	}

	if err := ai.ParseAIResponseWithRetry(response.Content, &result); err != nil {
		log.Printf("[Urgent Check] Failed to parse AI response: %v", err)
		return
	}

	// Filter by confidence threshold to reduce false positives
	if result.IsUrgent && result.Confidence < 0.5 {
		log.Printf("[Urgent Check] Low confidence urgency (%.2f), skipping", result.Confidence)
		return
	}

	if result.IsUrgent {
		log.Printf("[Urgent Check] Urgent issue detected: %s - %s (confidence: %.2f)", result.Category, result.Summary, result.Confidence)

		// Send instant alert via configured channels (Telegram/Email)
		// Launch in separate goroutine to avoid blocking webhook response
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Guesty Alert] Panic in SendGuestyAlert: %v", r)
				}
			}()

			alertCtx, alertCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer alertCancel()

			urgentDetails := notifications.UrgentIssueDetails{
				TenantID:       tenantID,
				ChannelID:      channel.ID,
				GuestName:      guestName,
				ListingName:    listingName,
				ReservationID:  reservationID,
				Category:       result.Category,
				Severity:       result.Severity,
				Summary:        result.Summary,
				Confidence:     result.Confidence,
				MessageContent: message,
			}

			if err := notifications.SendGuestyAlert(alertCtx, urgentDetails); err != nil {
				log.Printf("[Guesty Alert] Failed to send urgent alert: %v", err)
			} else {
				log.Printf("[Guesty Alert] Urgent alert sent successfully for channel %s", channel.ID)
			}
		}()
	}
}

func mustMarshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Panicf("[Guesty Webhook] Failed to marshal JSON: %v", err)
	}
	return string(b)
}

// GuestyWebhookChallenge handles Guesty webhook verification
func GuestyWebhookChallenge(c *gin.Context) {
	// Guesty may send a verification challenge
	challenge := c.Query("challenge")
	if challenge != "" {
		log.Printf("[Guesty Webhook] Verification challenge received")
		c.JSON(http.StatusOK, gin.H{"challenge": challenge})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
