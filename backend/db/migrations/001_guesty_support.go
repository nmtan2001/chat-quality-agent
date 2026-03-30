package migrations

import (
	"log"

	"github.com/nmtan2001/chat-quality-agent/db"
)

// AddJSONBIndexesForGuesty adds indexes for JSONB fields used by Guesty (PostgreSQL).
func AddJSONBIndexesForGuesty() error {
	// PostgreSQL GIN indexes for JSONB
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_channels_metadata_account_id
			ON channels USING gin ((metadata->>'account_id'))`,
		`CREATE INDEX IF NOT EXISTS idx_conversations_metadata_listing_id
			ON conversations USING gin ((metadata->>'listing_id'))`,
		`CREATE INDEX IF NOT EXISTS idx_conversations_metadata_reservation_id
			ON conversations USING gin ((metadata->>'reservation_id'))`,
		`CREATE INDEX IF NOT EXISTS idx_conversations_metadata_listing_nickname
			ON conversations USING gin ((metadata->>'listing_nickname'))`,
	}

	for _, idx := range indexes {
		if err := db.DB.Exec(idx).Error; err != nil {
			log.Printf("[Migration] Failed to create index: %v, SQL: %s", err, idx)
		}
	}

	log.Println("[Migration] Guesty JSONB indexes added for PostgreSQL")
	return nil
}

// DropJSONBIndexesForGuesty removes the Guesty-specific indexes (PostgreSQL).
func DropJSONBIndexesForGuesty() error {
	dropIndexes := []string{
		`DROP INDEX IF EXISTS idx_channels_metadata_account_id`,
		`DROP INDEX IF EXISTS idx_conversations_metadata_listing_id`,
		`DROP INDEX IF EXISTS idx_conversations_metadata_reservation_id`,
		`DROP INDEX IF EXISTS idx_conversations_metadata_listing_nickname`,
	}

	for _, idx := range dropIndexes {
		if err := db.DB.Exec(idx).Error; err != nil {
			log.Printf("[Migration] Failed to drop index: %v", err)
		}
	}

	log.Println("[Migration] Guesty JSONB indexes dropped")
	return nil
}
