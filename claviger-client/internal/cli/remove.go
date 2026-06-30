package cli

import (
	"claviger-client/internal/config"
	"fmt"
	"log"
)

func HandleRemove(vault *config.ClientVault, profileID string) {
	if _, exists := vault.Profiles[profileID]; !exists {
		log.Fatalf("❌ Server profile '%s' not found.", profileID)
	}

	delete(vault.Profiles, profileID)
	if vault.ActiveProfileID == profileID {
		vault.ActiveProfileID = ""
	}

	if err := config.Save(vault); err != nil {
		log.Fatalf("❌ Failed to update vault: %v", err)
	}
	fmt.Println("✅ Server deleted successfully.")
}
