package cli

import (
	"claviger-client/internal/config"
	"fmt"
)

func HandleList(vault *config.ClientVault) {
	fmt.Println("\n🌐 ENROLLED CLAVIGER SERVERS:")
	fmt.Println("---------------------------------------------------")
	if len(vault.Profiles) == 0 {
		fmt.Println("  No servers enrolled.")
		return
	}

	for id, profile := range vault.Profiles {
		activeMarker := "  "
		if id == vault.ActiveProfileID {
			activeMarker = "👉"
		}
		statusIcon := "🟢"
		if profile.Status != "active" {
			statusIcon = "🟡"
		}

		shortID := id
		if len(id) > 8 {
			shortID = id[:8]
		}

		fmt.Printf("%s [%s] %s %s | IP: %s\n", activeMarker, shortID, statusIcon, profile.Name, profile.AssignedIP)
		fmt.Printf("      Full ID: %s\n", id)
		fmt.Println("---------------------------------------------------")
	}
}
