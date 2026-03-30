package migrations

import (
	"log"

	"github.com/nmtan2001/chat-quality-agent/db"
)

// AddJSONBIndexesForGuesty adds indexes for JSONB fields used by Guesty.
func AddJSONBIndexesForGuesty() error {
	// Detect database type
	dbType := ""
	rows, err := db.DB.Raw("SELECT version()").Rows()
	if err == nil {
		var version string
		if rows.Next() {
			rows.Scan(&version)
			if contains(version, "PostgreSQL") {
				dbType = "postgres"
			}
		}
		rows.Close()
	}

	if dbType == "postgres" {
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

	// For MySQL, use generated column indexes (JSON doesn't support direct indexing)
	// Add generated columns for frequently accessed JSON fields
	alterStatements := []string{
		// Add account_id column if not exists
		`ALTER TABLE channels ADD COLUMN IF NOT EXISTS account_id VARCHAR(255)
		 AS (JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.account_id'))) STORED`,
		`CREATE INDEX idx_channels_account_id ON channels(account_id)`,

		// Add listing_id column to conversations
		`ALTER TABLE conversations ADD COLUMN IF NOT EXISTS listing_id VARCHAR(255)
		 AS (JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.listing_id'))) STORED`,
		`CREATE INDEX idx_conversations_listing_id ON conversations(listing_id)`,
	}

	for _, stmt := range alterStatements {
		if err := db.DB.Exec(stmt).Error; err != nil {
			// Log but don't fail - column might already exist
			log.Printf("[Migration] Note (may already exist): %v", err)
		}
	}

	log.Println("[Migration] Guesty JSON indexes added for MySQL")
	return nil
}

// DropJSONBIndexesForGuesty removes the Guesty-specific indexes.
func DropJSONBIndexesForGuesty() error {
	// Detect database type
	dbType := ""
	rows, err := db.DB.Raw("SELECT version()").Rows()
	if err == nil {
		var version string
		if rows.Next() {
			rows.Scan(&version)
			if contains(version, "PostgreSQL") {
				dbType = "postgres"
			}
		}
		rows.Close()
	}

	if dbType == "postgres" {
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

	// For MySQL, drop the generated columns
	dropStatements := []string{
		`ALTER TABLE channels DROP INDEX IF EXISTS idx_channels_account_id`,
		`ALTER TABLE conversations DROP INDEX IF EXISTS idx_conversations_listing_id`,
		`ALTER TABLE channels DROP COLUMN IF EXISTS account_id`,
		`ALTER TABLE conversations DROP COLUMN IF EXISTS listing_id`,
	}

	for _, stmt := range dropStatements {
		if err := db.DB.Exec(stmt).Error; err != nil {
			log.Printf("[Migration] Note (may not exist): %v", err)
		}
	}

	log.Println("[Migration] Guesty JSON indexes dropped")
	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
