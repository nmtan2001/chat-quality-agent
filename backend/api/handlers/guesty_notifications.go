package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/notifications"
	"github.com/nmtan2001/chat-quality-agent/pkg"
)

const (
	maxCustomTemplateLength = 10000
	maxBotTokenLength       = 256
	maxChatIDLength         = 128
	maxEmailLength          = 256
	minSMTPPort             = 1
	maxSMTPPort             = 65535
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func validateEmailList(emails string) error {
	if emails == "" {
		return http.ErrNotSupported
	}
	for _, email := range strings.Split(emails, ",") {
		email = strings.TrimSpace(email)
		if email != "" && !emailRegex.MatchString(email) {
			return http.ErrNotSupported
		}
	}
	return nil
}

func validateEmail(email string) bool {
	return emailRegex.MatchString(email)
}

type GuestyNotificationSettings struct {
	IsEnabled         bool                   `json:"is_enabled"`
	TelegramEnabled   bool                   `json:"telegram_enabled"`
	TelegramConfig    map[string]string      `json:"telegram_config,omitempty"`
	EmailEnabled      bool                   `json:"email_enabled"`
	EmailConfig       map[string]interface{} `json:"email_config,omitempty"`
	UseCustomTemplate bool                   `json:"use_custom_template"`
	CustomTemplate    string                 `json:"custom_template,omitempty"`
}

type GuestyNotificationSettingsResponse struct {
	IsEnabled         bool                   `json:"is_enabled"`
	TelegramEnabled   bool                   `json:"telegram_enabled"`
	TelegramConfig    map[string]string      `json:"telegram_config"`
	EmailEnabled      bool                   `json:"email_enabled"`
	EmailConfig       map[string]interface{} `json:"email_config"`
	UseCustomTemplate bool                   `json:"use_custom_template"`
	CustomTemplate    string                 `json:"custom_template"`
}

// GetGuestyNotificationSettings returns notification settings for a Guesty channel
func GetGuestyNotificationSettings(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	channelID := c.Param("channelId")

	var settings models.GuestyNotificationSetting
	err := db.DB.Where("tenant_id = ? AND channel_id = ?", tenantID, channelID).
		First(&settings).Error

	if err != nil {
		// Return default settings if not found
		c.JSON(http.StatusOK, GuestyNotificationSettingsResponse{
			IsEnabled:         true,
			TelegramEnabled:   false,
			TelegramConfig:    make(map[string]string),
			EmailEnabled:      false,
			EmailConfig:       make(map[string]interface{}),
			UseCustomTemplate: false,
			CustomTemplate:    "",
		})
		return
	}

	// Parse configs
	var telegramCfg map[string]string
	json.Unmarshal([]byte(settings.TelegramConfig), &telegramCfg)

	var emailCfg map[string]interface{}
	json.Unmarshal([]byte(settings.EmailConfig), &emailCfg)

	// Mask sensitive data for response
	response := GuestyNotificationSettingsResponse{
		IsEnabled:         settings.IsEnabled,
		TelegramEnabled:   settings.TelegramEnabled,
		TelegramConfig:    telegramCfg,
		EmailEnabled:      settings.EmailEnabled,
		EmailConfig:       emailCfg,
		UseCustomTemplate: settings.UseCustomTemplate,
		CustomTemplate:    settings.CustomTemplate,
	}

	if response.TelegramConfig != nil {
		if token, ok := response.TelegramConfig["bot_token"]; ok && len(token) > 8 {
			response.TelegramConfig["bot_token"] = token[:8] + "****"
		}
	}

	// Mask email password
	if response.EmailConfig != nil {
		if pass, ok := response.EmailConfig["smtp_pass"].(string); ok && len(pass) > 4 {
			response.EmailConfig["smtp_pass"] = "****"
		}
	}

	c.JSON(http.StatusOK, response)
}

// UpdateGuestyNotificationSettings updates notification settings for a Guesty channel
func UpdateGuestyNotificationSettings(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	channelID := c.Param("channelId")

	var req GuestyNotificationSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	// Validate custom template length
	if len(req.CustomTemplate) > maxCustomTemplateLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": "custom_template_too_long", "details": "custom_template exceeds maximum length"})
		return
	}

	// Validate Telegram config
	if req.TelegramEnabled && req.TelegramConfig != nil {
		botToken, _ := req.TelegramConfig["bot_token"]
		chatID, _ := req.TelegramConfig["chat_id"]

		if botToken == "" || chatID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_telegram_config", "details": "bot_token and chat_id are required when telegram is enabled"})
			return
		}

		if len(botToken) > maxBotTokenLength {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_bot_token", "details": "bot_token exceeds maximum length"})
			return
		}

		if len(chatID) > maxChatIDLength {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_chat_id", "details": "chat_id exceeds maximum length"})
			return
		}
	}

	// Validate Email config
	if req.EmailEnabled && req.EmailConfig != nil {
		smtpHost, _ := req.EmailConfig["smtp_host"].(string)
		smtpPort, _ := req.EmailConfig["smtp_port"].(float64)
		from, _ := req.EmailConfig["from"].(string)
		to, _ := req.EmailConfig["to"].(string)

		if smtpHost == "" || from == "" || to == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_email_config", "details": "smtp_host, from, and to are required when email is enabled"})
			return
		}

		if int(smtpPort) < minSMTPPort || int(smtpPort) > maxSMTPPort {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_smtp_port", "details": "smtp_port must be between 1 and 65535"})
			return
		}

		if len(from) > maxEmailLength {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_from_email", "details": "from email exceeds maximum length"})
			return
		}

		if len(to) > maxEmailLength {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_to_email", "details": "to email exceeds maximum length"})
			return
		}

		// Validate email formats
		if !validateEmail(from) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_from_email", "details": "from email is invalid"})
			return
		}

		if err := validateEmailList(to); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_to_email", "details": "to email addresses are invalid"})
			return
		}
	}

	// Verify channel exists and is Guesty type
	var channel models.Channel
	if err := db.DB.Where("id = ? AND tenant_id = ? AND channel_type = ?", channelID, tenantID, "guesty").
		First(&channel).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "guesty_channel_not_found"})
		return
	}

	// Prepare configs
	telegramConfigJSON := ""
	if req.TelegramEnabled && req.TelegramConfig != nil {
		bytes, _ := json.Marshal(req.TelegramConfig)
		telegramConfigJSON = string(bytes)
	}

	emailConfigJSON := ""
	if req.EmailEnabled && req.EmailConfig != nil {
		bytes, _ := json.Marshal(req.EmailConfig)
		emailConfigJSON = string(bytes)
	}

	// Update or create settings
	var settings models.GuestyNotificationSetting
	err := db.DB.Where("tenant_id = ? AND channel_id = ?", tenantID, channelID).
		First(&settings).Error

	now := time.Now()
	if err != nil {
		// Create new settings
		settings = models.GuestyNotificationSetting{
			ID:                pkg.NewUUID(),
			TenantID:          tenantID,
			ChannelID:         channelID,
			IsEnabled:         req.IsEnabled,
			TelegramEnabled:   req.TelegramEnabled,
			TelegramConfig:    telegramConfigJSON,
			EmailEnabled:      req.EmailEnabled,
			EmailConfig:       emailConfigJSON,
			UseCustomTemplate: req.UseCustomTemplate,
			CustomTemplate:    req.CustomTemplate,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := db.DB.Create(&settings).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_create_settings"})
			return
		}
	} else {
		// Update existing
		updates := map[string]interface{}{
			"is_enabled":          req.IsEnabled,
			"telegram_enabled":    req.TelegramEnabled,
			"telegram_config":     telegramConfigJSON,
			"email_enabled":       req.EmailEnabled,
			"email_config":        emailConfigJSON,
			"use_custom_template": req.UseCustomTemplate,
			"custom_template":     req.CustomTemplate,
			"updated_at":          now,
		}
		if err := db.DB.Model(&settings).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_update_settings"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "settings_updated"})
}

// TestGuestyNotification sends a test notification
func TestGuestyNotification(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	channelID := c.Param("channelId")

	// Get settings
	var settings models.GuestyNotificationSetting
	err := db.DB.Where("tenant_id = ? AND channel_id = ?", tenantID, channelID).
		First(&settings).Error

	if err != nil || !settings.IsEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "notifications_not_enabled"})
		return
	}

	// Create test urgent details
	testDetails := notifications.UrgentIssueDetails{
		TenantID:       tenantID,
		ChannelID:      channelID,
		GuestName:      "Test Guest",
		ListingName:    "Test Property",
		ReservationID:  "TEST-123",
		Category:       "MAINTENANCE",
		Severity:       "high",
		Summary:        "Test notification - This is only a test",
		Confidence:     0.95,
		MessageContent: "This is a test urgent issue message to verify your notification configuration.",
	}

	if err := notifications.SendGuestyAlert(c.Request.Context(), testDetails); err != nil {
		log.Printf("[Guesty Alert] Test notification failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "test_notification_failed", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "test_notification_sent"})
}
