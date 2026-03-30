package migrations

import (
	"log"

	"gorm.io/gorm"
)

// CreateGuestyNotificationSettingsTable creates the guesty_notification_settings table
func CreateGuestyNotificationSettingsTable(db *gorm.DB) error {
	sql := `
	CREATE TABLE IF NOT EXISTS guesty_notification_settings (
		id CHAR(36) PRIMARY KEY,
		tenant_id CHAR(36) NOT NULL,
		channel_id CHAR(36) NOT NULL,
		is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
		telegram_enabled BOOLEAN NOT NULL DEFAULT FALSE,
		telegram_config TEXT,
		email_enabled BOOLEAN NOT NULL DEFAULT FALSE,
		email_config TEXT,
		use_custom_template BOOLEAN NOT NULL DEFAULT FALSE,
		custom_template TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_gnotif_tenant_channel (tenant_id, channel_id),
		UNIQUE INDEX idx_gnotif_channel (channel_id)
	);
	`

	if err := db.Exec(sql).Error; err != nil {
		log.Printf("[Migration] Failed to create guesty_notification_settings table: %v", err)
		return err
	}

	log.Println("[Migration] guesty_notification_settings table created")
	return nil
}

// DropGuestyNotificationSettingsTable drops the guesty_notification_settings table
func DropGuestyNotificationSettingsTable(db *gorm.DB) error {
	sql := `DROP TABLE IF EXISTS guesty_notification_settings;`

	if err := db.Exec(sql).Error; err != nil {
		log.Printf("[Migration] Failed to drop guesty_notification_settings table: %v", err)
		return err
	}

	log.Println("[Migration] guesty_notification_settings table dropped")
	return nil
}
