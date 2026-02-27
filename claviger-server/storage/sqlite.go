package storage

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"
)

// InitDB creates the file and tables if they don't exist
func InitDB() *sql.DB {
	db, err := sql.Open("sqlite", "claviger-server.db")
	if err != nil {
		log.Fatalf("❌ Failed to open database: %v", err)
	}

	createTableSQL := `
    CREATE TABLE IF NOT EXISTS config (
        key TEXT PRIMARY KEY,
        value TEXT NOT NULL
    );`
	if _, err = db.Exec(createTableSQL); err != nil {
		log.Fatalf("❌ Failed to create tables: %v", err)
	}

	return db
}

// SaveConfig saves a key-value pair to SQLite
func SaveConfig(db *sql.DB, key string, value string) {
	insertSQL := `INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)`
	_, err := db.Exec(insertSQL, key, value)
	if err != nil {
		log.Fatalf("❌ Failed to save config %s: %v", key, err)
	}
}

func GetConfig(db *sql.DB, key string) string {
	var value string
	err := db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err != nil {
		return "" // Returns empty if the key doesn't exist yet
	}
	return value
}

func ClearConfig(db *sql.DB) {
	_, err := db.Exec("DELETE FROM config")
	if err != nil {
		log.Fatalf("❌ Failed to clear local configuration: %v", err)
	}
}
