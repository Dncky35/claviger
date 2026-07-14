package cmd

import (
	"claviger-server/internal/crypto"
	"claviger-server/internal/firewall"
	"claviger-server/internal/system"
	"claviger-server/network"
	"claviger-server/storage"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

func RunRestore(backupFilePath string) {
	fmt.Println("==========================================================")
	fmt.Println("🛡️  CLAVIGER DISASTER RECOVERY")
	fmt.Println("==========================================================")

	// 1. Mandatory Root Check
	if os.Geteuid() != 0 {
		log.Fatal("❌ Access Denied: You must run this command with sudo.")
	}

	// 2. Guardrail: Is the system already provisioned?
	db := storage.InitDB()
	isSetup := storage.GetConfig(db, "node_id") != ""
	db.Close() // Close immediately to release any file locks before we overwrite!

	if isSetup {
		log.Fatal("❌ Node is already configured! If you are trying to recover, please run 'sudo claviger-server reset' to wipe the current state first.")
	}

	if _, err := net.InterfaceByName("wg0"); err == nil {
		log.Fatal("❌ The 'wg0' interface is currently active! Please stop the daemon ('sudo systemctl stop claviger-server') before restoring.")
	}

	// 3. Securely Prompt for the 12-Word Seed
	fmt.Print("Enter your 12-word Master Recovery Seed: ")
	seedBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("\n❌ Failed to read seed phrase securely: %v", err)
	}
	fmt.Println() // Manually print newline after ReadPassword

	seedPhrase := strings.TrimSpace(string(seedBytes))
	fmt.Println("⏳ Deriving keys and decrypting backup...")

	// 4. Derive the AES Key from the Seed
	aesKey, err := crypto.DeriveAESKey(seedPhrase)
	if err != nil {
		log.Fatalf("❌ Failed to derive keys: %v", err)
	}

	targetDBPath := "/var/lib/claviger/claviger.db"

	// 5. Execute the Secure File Swap
	err = performSecureRestore(backupFilePath, targetDBPath, aesKey)
	if err != nil {
		log.Fatalf("❌ Restore failed: %v", err)
	}

	// 🎯 THE FIX: Save the seed so future nightly backups can run autonomously!
	if err := os.WriteFile("/var/lib/claviger/seed.txt", []byte(seedPhrase), 0600); err != nil {
		log.Fatalf("❌ Failed to save recovery seed to disk: %v", err)
	}

	// 🎯 THE FIX: Ensure you are using the correct sqlite3 driver string
	restoredDB, err := sql.Open("sqlite3", targetDBPath)
	if err != nil {
		log.Fatalf("❌ Failed to open restored SQLite database: %v", err)
	}
	defer restoredDB.Close()

	if err := os.MkdirAll("/etc/wireguard", 0700); err != nil {
		log.Fatalf("❌ Failed to create WireGuard directory: %v", err)
	}

	serverPriv := storage.GetConfig(restoredDB, "wg_private_key")
	wgPort := storage.GetConfig(restoredDB, "wg_port")
	hubIP := storage.GetConfig(restoredDB, "hub_ip")

	wgConfContent := fmt.Sprintf(`[Interface]
PrivateKey = %s
ListenPort = %s
Address = %s/24
SaveConfig = false
`, serverPriv, wgPort, hubIP)

	if err := os.WriteFile("/etc/wireguard/wg0.conf", []byte(wgConfContent), 0600); err != nil {
		log.Fatalf("❌ Failed to write wg0.conf file: %v", err)
	}

	// 🎯 THE FIX: Perimeter secured first.
	fmt.Println("🛡️  Configuring firewall rules for VPN interface...")
	firewall.SetupFirewall(wgPort, false)

	// 🎯 THE FIX: Network brought online behind the shield.
	if err := network.StartWireGuard(); err != nil {
		log.Printf("⚠️ Warning: Could not start WireGuard automatically: %v", err)
	}

	fmt.Println("🔄 Installing background services...")
	if err := system.InstallSystemdService(); err != nil {
		log.Printf("⚠️ Warning: Could not install auto-start service: %v\n", err)
	}

	fmt.Println("==========================================================")
	fmt.Println("✅ SYSTEM RESTORED SUCCESSFULLY!")
	fmt.Println("==========================================================")
	fmt.Println("You may now start the system: sudo systemctl start claviger-server")
}

// performSecureRestore remains exactly as you wrote it, it is perfect.

func performSecureRestore(backupFilePath string, targetDBPath string, aesKey []byte) error {
	log.Printf("[Restore] 💾 Starting secure database restoration from: %s\n", backupFilePath)

	// 1. Validate Key
	if len(aesKey) != 32 {
		return fmt.Errorf("invalid AES key length: expected 32 bytes")
	}

	// 2. Read the Encrypted Backup
	encryptedData, err := os.ReadFile(backupFilePath)
	if err != nil {
		return fmt.Errorf("failed to read backup file: %v", err)
	}

	// 3. Set up AES-256-GCM
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return fmt.Errorf("cipher creation failed: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("GCM creation failed: %v", err)
	}

	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		return fmt.Errorf("backup file is corrupted or too short")
	}

	// 4. Split the Nonce and Ciphertext
	// In the backup function, we prepended the nonce using: gcm.Seal(nonce, nonce, plaintext, nil)
	nonce := encryptedData[:nonceSize]
	ciphertext := encryptedData[nonceSize:]

	// 5. Decrypt the Data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("decryption failed (wrong seed phrase or corrupted file): %v", err)
	}

	// 6. Atomic File Swap (Safety First!)
	// We write to a temporary file first, so if the disk gets full mid-write, we don't destroy the original DB.
	tempDBPath := targetDBPath + ".tmp"
	if err := os.WriteFile(tempDBPath, plaintext, 0600); err != nil {
		return fmt.Errorf("failed to write decrypted data to temp file: %v", err)
	}

	// Ensure the parent directory exists
	if err := os.MkdirAll(filepath.Dir(targetDBPath), 0700); err != nil {
		return fmt.Errorf("failed to create database directory: %v", err)
	}

	// Swap the temp file with the actual database file
	if err := os.Rename(tempDBPath, targetDBPath); err != nil {
		_ = os.Remove(tempDBPath) // Cleanup on failure
		return fmt.Errorf("failed to finalize database restore: %v", err)
	}

	log.Println("[Restore] ✅ Database restored successfully! Ready for configuration.")
	return nil
}
