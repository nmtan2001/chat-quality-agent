package models

import "time"

// GuestyNotificationSetting stores notification configuration for Guesty urgent alerts
type GuestyNotificationSetting struct {
	ID        string    `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID  string    `gorm:"type:char(36);not null;index:idx_gnotif_tenant_channel" json:"tenant_id"`
	ChannelID string    `gorm:"type:char(36);not null;index:idx_gnotif_tenant_channel;uniqueIndex:idx_gnotif_channel" json:"channel_id"`

	// Enable/disable notifications
	IsEnabled bool `gorm:"not null;default:true" json:"is_enabled"`

	// Telegram configuration (JSON)
	TelegramEnabled bool   `gorm:"not null;default:false" json:"telegram_enabled"`
	TelegramConfig   string `gorm:"type:text" json:"telegram_config"` // {"bot_token":"", "chat_id":""}

	// Email configuration (JSON)
	EmailEnabled bool   `gorm:"not null;default:false" json:"email_enabled"`
	EmailConfig   string `gorm:"type:text" json:"email_config"` // {"smtp_host":"", "smtp_port":587, "smtp_user":"", "smtp_pass":"", "from":"", "to":""}

	// Custom template
	UseCustomTemplate bool   `gorm:"not null;default:false" json:"use_custom_template"`
	CustomTemplate   string `gorm:"type:text" json:"custom_template"`

	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}
