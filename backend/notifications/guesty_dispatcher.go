package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/pkg"
)

// UrgentIssueDetails represents details about an urgent Guesty issue
type UrgentIssueDetails struct {
	TenantID       string
	ChannelID      string
	GuestName      string
	ListingName    string
	ReservationID  string
	Category       string
	Severity       string
	Summary        string
	Confidence     float64
	MessageContent string
}

// SendGuestyAlert sends an urgent issue alert via configured channels
func SendGuestyAlert(ctx context.Context, details UrgentIssueDetails) error {
	// Get notification settings for this channel
	var settings models.GuestyNotificationSetting
	err := db.DB.WithContext(ctx).
		Where("channel_id = ? AND is_enabled = ?", details.ChannelID, true).
		First(&settings).Error

	if err != nil {
		return fmt.Errorf("notification settings not found for channel %s: %w", details.ChannelID, err)
	}

	var outputs []OutputConfig

	// Add Telegram output if enabled
	if settings.TelegramEnabled && settings.TelegramConfig != "" {
		var telegramCfg map[string]string
		if err := json.Unmarshal([]byte(settings.TelegramConfig), &telegramCfg); err == nil {
			outputs = append(outputs, OutputConfig{
				Type:     "telegram",
				BotToken: telegramCfg["bot_token"],
				ChatID:   telegramCfg["chat_id"],
			})
		}
	}

	// Add Email output if enabled
	if settings.EmailEnabled && settings.EmailConfig != "" {
		var emailCfg map[string]interface{}
		if err := json.Unmarshal([]byte(settings.EmailConfig), &emailCfg); err == nil {
			port := 587
			if p, ok := emailCfg["smtp_port"].(float64); ok {
				port = int(p)
			}
			outputs = append(outputs, OutputConfig{
				Type:     "email",
				SMTPHost: emailCfg["smtp_host"].(string),
				SMTPPort: port,
				SMTPUser: emailCfg["smtp_user"].(string),
				SMTPPass: emailCfg["smtp_pass"].(string),
				From:     emailCfg["from"].(string),
				To:       emailCfg["to"].(string),
			})
		}
	}

	if len(outputs) == 0 {
		return fmt.Errorf("no notification outputs configured for channel %s", details.ChannelID)
	}

	// Build subject and body
	subject := fmt.Sprintf("[URGENT] %s issue at %s", details.Category, details.ListingName)

	// Use custom template if configured, otherwise use default
	body := buildDefaultGuestyAlertBody(details)
	if settings.UseCustomTemplate && settings.CustomTemplate != "" {
		// Sanitize custom template for security
		sanitizedTemplate := SanitizeCustomTemplate(settings.CustomTemplate)
		rendered := renderGuestyCustomTemplate(sanitizedTemplate, details)
		body = rendered
	}

	// Send to each output and log
	for _, output := range outputs {
		notifier, err := createNotifier(output)
		if err != nil {
			log.Printf("[Guesty Alert] Failed to create notifier for %s: %v", output.Type, err)
			continue
		}

		// For Telegram, sanitize to remove HTML
		sendBody := body
		if output.Type == "telegram" {
			sendBody = SanitizeForTelegram(body)
		}

		sendErr := notifier.Send(ctx, subject, sendBody)
		status := "sent"
		errMsg := ""
		if sendErr != nil {
			status = "failed"
			errMsg = sendErr.Error()
			log.Printf("[Guesty Alert] Send failed for %s: %v", output.Type, sendErr)
		}

		// Log notification
		recipient := output.ChatID
		if output.Type == "email" {
			recipient = output.To
		}

		logEntry := models.NotificationLog{
			ID:           pkg.NewUUID(),
			TenantID:     details.TenantID,
			ChannelType:  output.Type,
			Recipient:    recipient,
			Subject:      subject,
			Body:         body,
			Status:       status,
			ErrorMessage: errMsg,
			SentAt:       time.Now(),
			CreatedAt:    time.Now(),
		}
		db.DB.WithContext(ctx).Create(&logEntry)
	}

	return nil
}

// buildDefaultGuestyAlertBody creates the default notification body for urgent issues
func buildDefaultGuestyAlertBody(details UrgentIssueDetails) string {
	return fmt.Sprintf(
		"🚨 <b>URGENT: %s issue detected</b>\n\n"+
			"🏠 <b>Property:</b> %s\n"+
			"👤 <b>Guest:</b> %s\n"+
			"📅 <b>Reservation:</b> %s\n\n"+
			"❌ <b>Issue:</b> %s\n"+
			"⚠️ <b>Severity:</b> %s\n"+
			"📊 <b>Confidence:</b> %.0f%%\n\n"+
			"💬 <b>Message:</b>\n%s",
		details.Category,
		details.ListingName,
		details.GuestName,
		details.ReservationID,
		details.Summary,
		details.Severity,
		details.Confidence*100,
		details.MessageContent,
	)
}

// renderGuestyCustomTemplate renders a custom template with Guesty issue details
func renderGuestyCustomTemplate(tmpl string, details UrgentIssueDetails) string {
	replacements := map[string]string{
		"{{category}}":       details.Category,
		"{{listing_name}}":   details.ListingName,
		"{{guest_name}}":     details.GuestName,
		"{{reservation_id}}": details.ReservationID,
		"{{summary}}":        details.Summary,
		"{{severity}}":       details.Severity,
		"{{confidence}}":     fmt.Sprintf("%.0f%%", details.Confidence*100),
		"{{message}}":        details.MessageContent,
		"{{timestamp}}":      time.Now().Format("2006-01-02 15:04:05"),
	}

	result := tmpl
	for k, v := range replacements {
		result = strings.ReplaceAll(result, k, v)
	}
	return result
}
