package system

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"claviger-server/storage" // Update this to your actual storage import path
)

const backupDir = "/var/lib/claviger/backups"

// PerformSecureBackup creates a safe snapshot of the DB, encrypts it, and saves it.
func PerformSecureBackup(db *sql.DB) error {
	log.Println("[Backup] 💾 Starting secure database backup...")

	// 1. Ensure the backup directory exists
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return fmt.Errorf("failed to create backup directory: %v", err)
	}

	// 2. Retrieve or Generate the 32-byte AES Recovery Key
	recoveryKeyHex := storage.GetConfig(db, "backup_recovery_key")
	if recoveryKeyHex == "" {
		// First time running! Let's generate a strong key and save it.
		keyBytes := make([]byte, 32) // AES-256 requires exactly 32 bytes
		if _, err := io.ReadFull(rand.Reader, keyBytes); err != nil {
			return fmt.Errorf("failed to generate secure key: %v", err)
		}
		recoveryKeyHex = hex.EncodeToString(keyBytes)
		storage.SetConfig(db, "backup_recovery_key", recoveryKeyHex)
		log.Println("[Backup] 🔑 New Recovery Key generated! (Admin must save this)")
	}

	key, _ := hex.DecodeString(recoveryKeyHex)

	// 3. Create a safe, live snapshot using SQLite's VACUUM INTO
	tempDBPath := "/tmp/claviger_snapshot.db"
	_ = os.Remove(tempDBPath) // Clean up any old failed attempts

	_, err := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", tempDBPath))
	if err != nil {
		return fmt.Errorf("failed to create sqlite snapshot: %v", err)
	}
	defer os.Remove(tempDBPath) // Always delete the raw temp file when done!

	// 4. Read the unencrypted snapshot
	plaintext, err := os.ReadFile(tempDBPath)
	if err != nil {
		return fmt.Errorf("failed to read snapshot: %v", err)
	}

	// 5. Encrypt it with AES-256-GCM
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// 6. Save the encrypted file with a timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	finalPath := filepath.Join(backupDir, fmt.Sprintf("claviger_%s.enc", timestamp))

	if err := os.WriteFile(finalPath, ciphertext, 0600); err != nil {
		return fmt.Errorf("failed to save encrypted backup: %v", err)
	}

	log.Printf("[Backup] ✅ Encrypted backup saved successfully to: %s\n", finalPath)

	// 7. Prune old backups (Keep only the last 7 days)
	pruneOldBackups()

	return nil
}

// pruneOldBackups deletes any .enc files older than 7 days to save disk space
func pruneOldBackups() {
	files, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -7)

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".enc" {
			info, err := file.Info()
			if err == nil && info.ModTime().Before(cutoff) {
				oldFilePath := filepath.Join(backupDir, file.Name())
				os.Remove(oldFilePath)
				log.Printf("[Backup] 🗑️ Deleted old backup: %s\n", file.Name())
			}
		}
	}
}
