package storage

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// InitDB initializes the local SQLite database and creates the necessary tables.
func InitDB() *sql.DB {
	db, err := sql.Open("sqlite", "claviger.db")
	if err != nil {
		log.Fatalf("❌ Failed to open SQLite database: %v", err)
	}

	// 1. Config Table (Strictly Key/Value)
	createConfigTable := `
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`
	if _, err := db.Exec(createConfigTable); err != nil {
		log.Fatalf("❌ Failed to create config table: %v", err)
	}

	// Pre-seed default configuration values
	db.Exec(`INSERT OR IGNORE INTO config (key, value) VALUES ('allow_global_internet', 'false')`)
	db.Exec(`INSERT OR IGNORE INTO config (key, value) VALUES ('wg_port', '51820')`)
	db.Exec(`INSERT OR IGNORE INTO config (key, value) VALUES ('hub_ip', '10.8.0.1')`)
	db.Exec(`INSERT OR IGNORE INTO config (key, value) VALUES ('hub_port', '18080')`)

	// 2. Roles Table
	createRolesTable := `
	CREATE TABLE IF NOT EXISTS roles (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		allowed_ports TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(createRolesTable); err != nil {
		log.Fatalf("❌ Failed to create roles table: %v", err)
	}

	db.Exec(`INSERT OR IGNORE INTO roles (id, name, allowed_ports) VALUES ('admin', 'Administrator', 'ALL')`)
	db.Exec(`INSERT OR IGNORE INTO roles (id, name, allowed_ports) VALUES ('standard', 'Standard User', '80,443')`)

	// 3. Clients Table
	createClientsTable := `
	CREATE TABLE IF NOT EXISTS clients (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		public_key TEXT UNIQUE NOT NULL, 
		ip_address TEXT UNIQUE,       
		role_id TEXT NOT NULL,
		platform TEXT,                
		device_id TEXT,               
		status TEXT DEFAULT 'pending',
		last_seen DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (role_id) REFERENCES roles(id)
	);`
	if _, err := db.Exec(createClientsTable); err != nil {
		log.Fatalf("❌ Failed to create clients table: %v", err)
	}

	// 4. Invitations Table
	createInvitationsTable := `
	CREATE TABLE IF NOT EXISTS invitations (
		token TEXT PRIMARY KEY,       
		role_id TEXT NOT NULL,        
		expires_at DATETIME NOT NULL, 
		is_used BOOLEAN DEFAULT 0,    
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (role_id) REFERENCES roles(id)
	);`
	if _, err := db.Exec(createInvitationsTable); err != nil {
		log.Fatalf("❌ Failed to create invitations table: %v", err)
	}

	return db
}

func GetConfig(db *sql.DB, key string) string {
	var value string
	err := db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err != nil {
		return "" // Return empty string if not found, allowing defaults to take over
	}
	return value
}

func SetConfig(db *sql.DB, key, value string) error {
	_, err := db.Exec(`
		INSERT INTO config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

func ClearConfig(db *sql.DB) {
	db.Exec("DELETE FROM config")
	db.Exec("DELETE FROM clients")
	db.Exec("DELETE FROM invitations")
}
