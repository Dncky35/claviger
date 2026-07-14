package system

import (
	"claviger-server/internal/crypto"
	"claviger-server/storage"
	"database/sql"
	"log"
	"os"
)

func EnsureIdentity(db *sql.DB) ([]byte, error) {
	seedPath := "/var/lib/claviger/seed.txt"

	// 1. Check if seed already exists
	if _, err := os.Stat(seedPath); err == nil {
		log.Println("✅ Identity seed found. Loading...")
		mnemonicBytes, _ := os.ReadFile(seedPath)
		key, _ := crypto.DeriveAESKey(string(mnemonicBytes))
		return key, nil
	}

	// 2. No seed found? Check for legacy key
	log.Println("⚠️ No seed file found. Checking for legacy configuration...")
	legacyKey := storage.GetConfig(db, "backup_recovery_key")

	if legacyKey != "" {
		log.Println("🔄 Migration Mode: Legacy key found. Generating new deterministic identity...")

		// Generate new seed
		mnemonic, err := crypto.GenerateNewMnemonic()
		if err != nil {
			return nil, err
		}

		// Save seed
		if err := os.WriteFile(seedPath, []byte(mnemonic), 0600); err != nil {
			return nil, err
		}

		log.Println("✅ Successfully migrated to Seed-Linked identity.")

		// Optional: You could wipe the old key from DB here,
		// but it's safer to keep it for one cycle while users re-enroll.
		// storage.ClearConfigKey(db, "backup_recovery_key")
	} else {
		log.Println("🆕 Fresh install detected. Generating new identity...")
		mnemonic, _ := crypto.GenerateNewMnemonic()
		os.WriteFile(seedPath, []byte(mnemonic), 0600)
	}

	// Return the key for the current session
	mnemonicBytes, _ := os.ReadFile(seedPath)
	key, _ := crypto.DeriveAESKey(string(mnemonicBytes))
	return key, nil
}
