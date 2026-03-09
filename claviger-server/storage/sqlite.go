package storage

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// InitDB initializes the local SQLite database and creates the necessary tables.
func InitDB() *sql.DB {
	// Open or create the local database file
	db, err := sql.Open("sqlite", "claviger.db")
	if err != nil {
		log.Fatalf("❌ Failed to open SQLite database: %v", err)
	}

	// 1. Config Table (Stores Node Identity & Settings)
	createConfigTable := `
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`
	if _, err := db.Exec(createConfigTable); err != nil {
		log.Fatalf("❌ Failed to create config table: %v", err)
	}

	// 2. Roles Table (For Micro-segmentation & Firewall rules)
	createRolesTable := `
	CREATE TABLE IF NOT EXISTS roles (
		id TEXT PRIMARY KEY,          -- e.g., 'admin', 'developer', 'restricted'
		name TEXT NOT NULL,           -- e.g., 'Administrator'
		allowed_ports TEXT NOT NULL,  -- e.g., '22,80,443,18080' or 'ALL'
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(createRolesTable); err != nil {
		log.Fatalf("❌ Failed to create roles table: %v", err)
	}

	// Insert default roles if they don't exist
	db.Exec(`INSERT OR IGNORE INTO roles (id, name, allowed_ports) VALUES ('admin', 'Administrator', 'ALL')`)
	db.Exec(`INSERT OR IGNORE INTO roles (id, name, allowed_ports) VALUES ('standard', 'Standard User', '80,443')`)

	// 3. Clients Table (Stores WireGuard Peers)
	createClientsTable := `
	CREATE TABLE IF NOT EXISTS clients (
		id TEXT PRIMARY KEY,          -- UUID
		name TEXT NOT NULL,           -- e.g., "Ercan's iPhone"
		public_key TEXT UNIQUE NOT NULL, 
		ip_address TEXT UNIQUE NOT NULL, -- e.g., "10.8.0.2"
		role_id TEXT NOT NULL,        -- Links to roles table
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (role_id) REFERENCES roles(id)
	);`
	if _, err := db.Exec(createClientsTable); err != nil {
		log.Fatalf("❌ Failed to create clients table: %v", err)
	}

	return db
}

// GetConfig reads a setting from the database.
func GetConfig(db *sql.DB, key string) string {
	var value string
	err := db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err != nil {
		return ""
	}
	return value
}

// SetConfig saves or updates a setting in the database.
func SetConfig(db *sql.DB, key, value string) error {
	_, err := db.Exec(`
		INSERT INTO config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// ClearConfig wipes the database (used for the Poison Pill).
func ClearConfig(db *sql.DB) {
	db.Exec("DELETE FROM config")
	db.Exec("DELETE FROM clients")
	// We leave the roles table alone as it is structural
}
