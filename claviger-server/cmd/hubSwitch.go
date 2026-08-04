package cmd

import (
	"claviger-server/storage"
	"fmt"
	"os/exec"
	"strings"
)

// RunSwitchPublicHub enables or disables public-listening mode for the sub-server
func RunSwitchPublicHub(args []string) {
	if len(args) < 1 {
		fmt.Println("❌ Usage: claviger-server public-hub [enable|disable]")
		return
	}

	db := storage.InitDB()
	defer db.Close()

	action := strings.ToLower(args[0])
	var val string

	switch action {
	case "enable":
		val = "true"
	case "disable":
		val = "false"
	default:
		fmt.Println("❌ Invalid action. Use 'enable' or 'disable'.")
		fmt.Println("   Example: claviger-server public-hub enable")
		return
	}

	// Update the public_hub flag in the config table
	if err := storage.SetConfig(db, "public_hub", val); err != nil {
		fmt.Printf("❌ Failed to update public_hub config: %v\n", err)
		return
	}

	if val == "true" {
		fmt.Println("🛡️  Opening UFW port 18080 on interface claviger0...")
		// Equivalent to: sudo ufw allow in on claviger0 proto tcp to any port 18080
		cmd := exec.Command("ufw", "allow", "in", "on", "claviger0", "proto", "tcp", "to", "any", "port", "18080")
		if err := cmd.Run(); err != nil {
			fmt.Printf("⚠️  Could not automatically configure UFW (is UFW installed?): %v\n", err)
		}

		fmt.Println("✅ Public Hub mode ENABLED.")
		fmt.Println("   Server will bind to 0.0.0.0 (allowing remote Master proxy connections) on next restart.")
	} else {
		fmt.Println("🛡️  Closing UFW port 18080 on interface claviger0...")
		// Equivalent to: sudo ufw delete allow in on claviger0 proto tcp to any port 18080
		cmd := exec.Command("ufw", "delete", "allow", "in", "on", "claviger0", "proto", "tcp", "to", "any", "port", "18080")
		if err := cmd.Run(); err != nil {
			// It might error if the rule didn't exist in the first place, which is fine
			fmt.Printf("⚠️  Note: UFW rule removal returned an error or was already removed: %v\n", err)
		}

		fmt.Println("✅ Public Hub mode DISABLED.")
		fmt.Println("   Server will bind strictly to internal gateway IP on next restart.")
	}

	fmt.Println("\n---------------------------------------------------")
	fmt.Println("⚠️  ACTION REQUIRED: Please restart your Sub-server daemon to apply changes:")
	fmt.Println("   sudo systemctl restart claviger-server (or restart your binary)")
	fmt.Println("---------------------------------------------------")
}
