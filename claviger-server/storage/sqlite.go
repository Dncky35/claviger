package storage

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// InitDB initializes the local SQLite database and creates the necessary tables.
func InitDB() *sql.DB {
	// Define a global, absolute path for production state data
	dbDir := "/var/lib/claviger"
	dbPath := filepath.Join(dbDir, "claviger.db")

	// Ensure the directory exists (equivalent to 'mkdir -p /var/lib/claviger')
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Fatalf("❌ Failed to create database directory %s: %v", dbDir, err)
	}

	// 1. Open the DB using the absolute system path
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("❌ Failed to open SQLite database: %v", err)
	}

	// 2. Apply strict PRAGMAS for concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("⚠️ Failed to set WAL mode: %v", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000;"); err != nil {
		log.Printf("⚠️ Failed to set busy timeout: %v", err)
	}

	// 3. Give Go enough connections to handle concurrent API requests!
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)

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
		allow_global_internet BOOLEAN DEFAULT 0,
		allow_intranet BOOLEAN DEFAULT 0,
		allow_hub BOOLEAN DEFAULT 0,
		allowed_ports TEXT DEFAULT 'ALL',
		allowed_ips TEXT DEFAULT 'ALL',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(createRolesTable); err != nil {
		log.Fatalf("❌ Failed to create roles table: %v", err)
	}

	db.Exec(`INSERT OR IGNORE INTO roles (id, name, allow_global_internet, allow_intranet, allow_hub, allowed_ports, allowed_ips) 
		VALUES ('admin', 'Administrator', 1, 1, 1, 'ALL', 'ALL')`)

	db.Exec(`INSERT OR IGNORE INTO roles (id, name, allow_global_internet, allow_intranet, allow_hub, allowed_ports, allowed_ips) 
		VALUES ('standard', 'Standard User', 1, 0, 0, '80,443', 'ALL')`)

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

	createClientMFATable := `
    CREATE TABLE IF NOT EXISTS client_mfa (
        id TEXT PRIMARY KEY,
        client_id TEXT NOT NULL,
        mfa_type TEXT DEFAULT 'totp',
        secret TEXT NOT NULL,
        recovery_keys TEXT, 
        is_verified BOOLEAN DEFAULT 0, 
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE
    );`
	if _, err := db.Exec(createClientMFATable); err != nil {
		log.Fatalf("❌ Failed to create client_mfa table: %v", err)
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

	createCustomAppsTable := `
	CREATE TABLE IF NOT EXISTS custom_apps (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		icon TEXT,
		needs_dynamic_port BOOLEAN DEFAULT 0,
		has_custom_setup BOOLEAN DEFAULT 0,
		setup_port INTEGER DEFAULT 0,
		compose_yaml TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = db.Exec(createCustomAppsTable)
	if err != nil {
		log.Fatalf("❌ Failed to create custom_apps table: %v", err)
	}

	createSubServersTable := `
    CREATE TABLE IF NOT EXISTS sub_servers (
        id TEXT PRIMARY KEY,
        client_id TEXT NOT NULL UNIQUE,
        api_key TEXT NOT NULL,
        status TEXT DEFAULT 'unknown',
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE
    );`

	_, err = db.Exec(createSubServersTable)
	if err != nil {
		log.Fatalf("❌ Failed to create sub_servers table: %v", err)
	}

	return db
}

func IncrementConfigRevision(db *sql.DB) (string, error) {
	currentRevStr := GetConfig(db, "config_revision")
	currentRev := 0

	if currentRevStr != "" {
		parsedRev, parseErr := strconv.Atoi(currentRevStr)
		if parseErr == nil {
			currentRev = parsedRev
		}
	}

	newRev := currentRev + 1
	newRevStr := strconv.Itoa(newRev)

	err := SetConfig(db, "config_revision", newRevStr)
	if err != nil {
		return currentRevStr, err // Return the old revision if it fails
	}

	return newRevStr, nil
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

// DeviceExists checks if a client is registered and currently authorized on the network.
func DeviceExists(db *sql.DB, deviceID string) (bool, error) {
	var exists bool

	// We check for both the device_id and ensure the status is 'active'.
	// This ensures that any suspended or deleted devices are immediately blocked.
	query := "SELECT EXISTS(SELECT 1 FROM clients WHERE device_id = ? AND status = 'active')"

	err := db.QueryRow(query, deviceID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("database error checking device existence: %v", err)
	}

	return exists, nil
}
