package cli

import (
	"claviger-client/internal/config"
	"fmt"
	"log"
)

func HandleGlobalRouting(vault *config.ClientVault, action string) {
	switch action {
	case "enable":
		vault.UseGlobalRouting = true
		fmt.Println("🌐 Global Routing ENABLED.")

	case "disable":
		vault.UseGlobalRouting = false
		fmt.Println("🌗 Global Routing DISABLED.")
	default:
		log.Fatalf("❌ Invalid action: '%s'. Usage: claviger-client global <enable|disable>", action)
	}

	if err := config.Save(vault); err != nil {
		log.Fatalf("❌ Failed to save preference (Did you forget 'sudo'?): %v", err)
	}
}
