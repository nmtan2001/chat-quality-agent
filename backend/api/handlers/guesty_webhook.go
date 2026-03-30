package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/ai"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/pkg"
)

const (
	guestyAccountIDHeader = "X-Guesty-Account-ID"
)

// GuestyWebhookPayload represents the incoming webhook payload from Guesty.
type GuestyWebhookPayload struct {
	Event       string                 `json:"event"`       // e.g., "reservation.messageReceived"
	Message     map[string]interface{} `json:"message"`
	Conversation map[string]interface{} `json:"conversation"`
	Reservation map[string]interface{} `json:"reservation"`
	AccountID   string                 `json:"accountId"` // May be in payload or header
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

	// Verify Svix signature if configured
	signature := c.GetHeader("svix-signature")
	timestamp := c.GetHeader("svix-timestamp")

	if signature != "" && timestamp != "" {
		// Get Svix secret from config or environment
		svixSecret := c.GetHeader("svix-secret") // Or from config/env
		if svixSecret == "" {
			svixSecret = c.GetString("svix_secret")
		}

		if svixSecret != "" {
			valid, err := pkg.VerifySvixSignature(string(bodyBytes), signature, timestamp, svixSecret)
			if err != nil || !valid {
				log.Printf("[Guesty Webhook] Signature verification failed: %v", err)
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
				return
			}

			// Verify timestamp to prevent replay attacks
			if err := pkg.VerifySvixTimestamp(timestamp, 300); err != nil {
				log.Printf("[Guesty Webhook] Timestamp verification failed: %v", err)
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid timestamp"})
				return
			}
		}
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
		// Still continue to process, but without tenant association
	}

	// Only proceed if we found a channel
	if channel.ID == "" {
		return fmt.Errorf("channel not found for account_id: %s", accountID)
	}

	// Persist conversation if not exists
	var conversation models.Conversation
	convResult := db.DB.Where("tenant_id = ? AND external_conversation_id = ?", channel.TenantID, conversationID).
		First(&conversation)

	if convResult.Error != nil {
		conversation = models.Conversation{
			ID:                     pkg.NewUUID(),
			TenantID:               channel.TenantID,
			ChannelID:              channel.ID,
			ExternalConversationID: conversationID,
			ExternalUserID:         reservationID,
			CustomerName:           guestName,
			Metadata:               mustMarshalJSON(map[string]interface{}{"reservation_id": reservationID, "listing_name": listingName}),
		}
		db.DB.Create(&conversation)
	}

	// Persist message
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
	db.DB.Create(&message)

	// Update conversation last message time
	now := time.Now()
	db.DB.Model(&conversation).Updates(map[string]interface{}{
		"last_message_at": &now,
		"message_count":   conversation.MessageCount + 1,
	})

	// Check for urgent issues (only for customer messages)
	if senderType == "customer" && channel.TenantID != "" {
		go checkUrgentIssue(channel.TenantID, conversation.ID, guestName, messageBody, listingName, reservationID)
	}

	return nil
}

// checkUrgentIssue uses AI to detect urgent issues in customer messages.
func checkUrgentIssue(tenantID, conversationID, guestName, message, listingName, reservationID string) {
	// Get AI provider from tenant settings
	var setting models.Setting
	if err := db.DB.Where("tenant_id = ? AND key = ?", tenantID, "ai").First(&setting).Error; err != nil {
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
	var err error

	switch aiConfig.Provider {
	case "claude":
		provider, err = ai.NewClaudeProvider(aiConfig.APIKey, aiConfig.Model)
	case "gemini":
		provider, err = ai.NewGeminiProvider(aiConfig.APIKey, aiConfig.Model)
	default:
		log.Printf("[Urgent Check] Unsupported AI provider: %s", aiConfig.Provider)
		return
	}

	if err != nil {
		log.Printf("[Urgent Check] Failed to create AI provider: %v", err)
		return
	}

	// Build urgency detection prompt
	prompt := buildUrgencyDetectionPrompt()

	// Analyze message
	response, err := provider.AnalyzeChat(nil, prompt, message)
	if err != nil {
		log.Printf("[Urgent Check] AI analysis failed: %v", err)
		return
	}

	// Parse response
	var result struct {
		IsUrgent bool   `json:"is_urgent"`
		Category string `json:"category"`
		Severity string `json:"severity"`
		Summary  string `json:"summary"`
	}

	if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
		log.Printf("[Urgent Check] Failed to parse AI response: %v", err)
		return
	}

	if result.IsUrgent {
		log.Printf("[Urgent Check] Urgent issue detected: %s - %s", result.Category, result.Summary)

		// Create notification log
		notification := models.NotificationLog{
			ID:        pkg.NewUUID(),
			TenantID:  tenantID,
			Subject:   fmt.Sprintf("[URGENT] %s issue at %s", result.Category, listingName),
			Body: fmt.Sprintf("Guest: %s\nListing: %s\nReservation: %s\nIssue: %s\n\nMessage: %s",
				guestName, listingName, reservationID, result.Summary, message),
			Status:    "pending",
			SentAt:    time.Now(),
			CreatedAt: time.Now(),
		}
		db.DB.Create(&notification)

		// TODO: Send instant alert via configured channels (Telegram/Email)
		// This uses the existing notification dispatcher
	}
}

// BuildUrgencyDetectionPrompt creates a prompt for detecting urgent issues.
func BuildUrgencyDetectionPrompt() string {
	return `You are an urgent issue detection system for vacation rental properties.

Analyze the guest message and determine if it reports an urgent issue that requires immediate attention.

Urgent categories:
1. CLEANING: Dirty rooms, bathroom issues, pests, trash, linen problems
2. MAINTENANCE: No hot water, AC/heat not working, leaks, broken appliances, power outages
3. PAYMENT: Guest refuses to pay, payment disputes, extra charges
4. SERVICE_REQUEST: Guest asks for special services (early check-in, late check-out, extra amenities)
5. SECURITY: Locks not working, safety concerns, unauthorized access
6. NOISE: Noise complaints from neighbors or construction
7. OTHER: Issues requiring immediate attention

Return JSON:
{
  "is_urgent": true/false,
  "category": "CLEANING|MAINTENANCE|PAYMENT|SERVICE_REQUEST|SECURITY|NOISE|OTHER",
  "severity": "high|medium|low",
  "summary": "Brief description of the issue (1 sentence)"
}

Consider as urgent if:
- Guest reports something broken, dirty, or not working
- Guest mentions refusing to pay or payment issues
- Guest requests immediate action or special service
- Guest expresses strong frustration or threat to leave bad review

ONLY return JSON, no additional text.`
}

func mustMarshalJSON(v interface{}) string {
	b, _ := json.Marshal(v)
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
