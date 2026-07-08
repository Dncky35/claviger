//go:build !windows

package daemon

import (
	"claviger-client/internal/config"
	"claviger-client/internal/vpn"
	"log"
)

func RunDaemon(vault *config.ClientVault, engine *vpn.Engine) {
	log.Println("🐧 Running Linux Daemon...")

	// AUTO-CONNECT LOGIC
	if vault.AutoConnect && vault.ActiveProfileID != "" {
		if profile, exists := vault.Profiles[vault.ActiveProfileID]; exists {
			log.Printf("🔄 Auto-Connect enabled. Booting tunnel for %s...", profile.Name)

			go func() {
				err := engine.Connect(vault, profile, vault.UseGlobalRouting)
				if err != nil {
					log.Printf("❌ Auto-Connect failed: %v", err)
				} else {
					log.Println("✅ Auto-Connect successful!")
				}
			}()
		}
	}

	// Keep the Linux daemon alive (e.g., waiting for your context/channel lock)
	select {}
}
