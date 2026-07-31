package cmd

import (
	"claviger-server/storage"
	"fmt"
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

	if action == "enable" {
		val = "true"
	} else if action == "disable" {
		val = "false"
	} else {
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
		fmt.Println("✅ Public Hub mode ENABLED.")
		fmt.Println("   Server will bind to 0.0.0.0 (allowing remote Master proxy connections) on next restart.")
	} else {
		fmt.Println("✅ Public Hub mode DISABLED.")
		fmt.Println("   Server will bind strictly to internal gateway IP on next restart.")
	}

	fmt.Println("\n---------------------------------------------------")
	fmt.Println("⚠️  ACTION REQUIRED: Please restart your Sub-server daemon to apply changes:")
	fmt.Println("   sudo systemctl restart claviger-server (or restart your binary)")
	fmt.Println("---------------------------------------------------")
}
