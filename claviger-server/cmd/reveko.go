package cmd

import (
	"claviger-server/storage"
	"database/sql"
	"fmt"
	"log"
	"os/exec"
)

func RunRevokeClient(clientID string) {
	db := storage.InitDB()
	defer db.Close()

	// 1. Fetch the Public Key and IP before deletion (Zero Trust Requirement)
	var pubKey, ipAddress string
	err := db.QueryRow("SELECT public_key, ip_address FROM clients WHERE id = ?", clientID).Scan(&pubKey, &ipAddress)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Fatalf("❌ Client ID '%s' not found.", clientID)
		}
		log.Fatalf("❌ Database error: %v", err)
	}

	// 2. Delete from SQLite Database
	_, err = db.Exec("DELETE FROM clients WHERE id = ?", clientID)
	if err != nil {
		log.Fatalf("❌ Failed to delete client from database: %v", err)
	}

	// 3. Rip the public key out of the live WireGuard interface
	// Assuming your interface is named wg0. Adjust if configurable.
	cmd := exec.Command("wg", "set", "wg0", "peer", pubKey, "remove")
	if err := cmd.Run(); err != nil {
		log.Printf("⚠️ Client removed from DB, but failed to drop from live WireGuard interface: %v", err)
		log.Printf("⚠️ Ensure you are running as root and 'wg0' is active.")
	} else {
		fmt.Println("✅ Successfully dropped from live WireGuard interface.")
	}

	fmt.Printf("✅ Revoked client '%s'. IP %s is now freed up.\n", clientID, ipAddress)
}
