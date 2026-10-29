package storage

import (
	"database/sql"
	"fmt"
	"log"
)

// RunMigrations safely handles SQLite schema versioning using PRAGMA user_version
func RunMigrations(db *sql.DB) error {
	var currentVersion int
	err := db.QueryRow("PRAGMA user_version;").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to read database user_version: %w", err)
	}

	log.Printf("📊 [Storage] Current database schema version: v%d", currentVersion)

	// Migration 0 -> 1: Add remote_apps table for cloudrocean.com sync
	if currentVersion < 1 {
		log.Println("🔄 [Storage] Migrating database to schema v1 (v0.4.4)...")
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start migration transaction: %w", err)
		}

		createRemoteAppsTable := `
		CREATE TABLE IF NOT EXISTS remote_apps (
			id TEXT PRIMARY KEY,
			catalog_type TEXT NOT NULL DEFAULT 'default',
			name TEXT NOT NULL,
			category TEXT NOT NULL,
			description TEXT,
			icon TEXT,
			needs_dynamic_port BOOLEAN DEFAULT 0,
			has_custom_setup BOOLEAN DEFAULT 0,
			setup_port INTEGER DEFAULT 0,
			compose_yaml TEXT NOT NULL,
			version_tag TEXT DEFAULT '1.0.0',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`

		if _, err := tx.Exec(createRemoteAppsTable); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create remote_apps table: %w", err)
		}

		// Bump schema version to 1
		if _, err := tx.Exec("PRAGMA user_version = 1;"); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update user_version: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration transaction: %w", err)
		}

		log.Println("✅ [Storage] Database schema successfully updated to v1")
		currentVersion = 1
	}

	// Migration 1 -> 2: Add config_file column for App Orchestrator JSON schemas
	if currentVersion < 2 {
		log.Println("🔄 [Storage] Migrating database to schema v2 (Adding config_file)...")
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to start migration v2 transaction: %w", err)
		}

		alterRemoteAppsTable := `
		ALTER TABLE remote_apps
		ADD COLUMN config_file TEXT;`

		if _, err := tx.Exec(alterRemoteAppsTable); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to add config_file to remote_apps: %w", err)
		}

		// Bump schema version to 2
		if _, err := tx.Exec("PRAGMA user_version = 2;"); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update user_version to 2: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration v2 transaction: %w", err)
		}

		log.Println("✅ [Storage] Database schema successfully updated to v2")
	}

	return nil
}
