package cli

import (
	"claviger-client/internal/config"
	"fmt"
	"log"
)

// HandleAutostart toggles the background daemon's boot-time auto-connect feature
func HandleAutoConnect(vault *config.ClientVault, action string) {
	switch action {
	case "enable":
		vault.AutoConnect = true
		fmt.Println("🔄 Auto-Connect ENABLED.")
		fmt.Println("The background daemon will now automatically connect your active profile on system boot.")
	case "disable":
		vault.AutoConnect = false
		fmt.Println("🛑 Auto-Connect DISABLED.")
		fmt.Println("Claviger will wait for a manual connection command after reboot.")
	default:
		log.Fatalf("❌ Invalid action: '%s'. Usage: claviger-client autostart <enable|disable>", action)
	}

	// Save the preference to the system-wide vault
	if err := config.Save(vault); err != nil {
		// If they didn't run with sudo, it will fail here with a permission denied error
		log.Fatalf("❌ Failed to save preference (Did you forget 'sudo'?): %v", err)
	}
}
