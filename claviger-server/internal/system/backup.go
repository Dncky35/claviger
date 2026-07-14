package system

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// PerformSecureBackup creates a safe, atomic snapshot of the DB and encrypts it.
func PerformSecureBackup(db *sql.DB, backupDir string, aesKey []byte) error {
	log.Println("[Backup] 💾 Starting secure database backup...")

	// 1. Ensure the backup directory exists
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return fmt.Errorf("failed to create backup directory: %v", err)
	}

	// 2. Validate Key
	if len(aesKey) != 32 {
		return fmt.Errorf("invalid AES key length: expected 32 bytes")
	}

	// 3. Create a safe, live snapshot using SQLite's VACUUM INTO
	// This is atomic and handles locking automatically.
	tempDBPath := "/tmp/claviger_snapshot.db"
	_ = os.Remove(tempDBPath)

	_, err := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", tempDBPath))
	if err != nil {
		return fmt.Errorf("failed to create sqlite snapshot: %v", err)
	}
	// Ensure cleanup even if encryption fails
	defer os.Remove(tempDBPath)

	// 4. Read the unencrypted snapshot
	plaintext, err := os.ReadFile(tempDBPath)
	if err != nil {
		return fmt.Errorf("failed to read snapshot: %v", err)
	}

	// 5. Encrypt it with AES-256-GCM
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	// Prepare nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	// Seal the data
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// 6. Save the encrypted file with a timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	finalPath := filepath.Join(backupDir, fmt.Sprintf("claviger_%s.enc", timestamp))

	if err := os.WriteFile(finalPath, ciphertext, 0600); err != nil {
		return fmt.Errorf("failed to save encrypted backup: %v", err)
	}

	log.Printf("[Backup] ✅ Encrypted backup saved successfully to: %s\n", finalPath)

	// 7. Prune old backups
	pruneOldBackups(backupDir)

	return nil
}

func pruneOldBackups(backupDir string) {
	files, err := os.ReadDir(backupDir)
	if err != nil {
		log.Printf("[Backup] ⚠️ Could not read backup dir for pruning: %v", err)
		return
	}

	cutoff := time.Now().AddDate(0, 0, -7)

	for _, file := range files {
		// Only touch our .enc files
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
