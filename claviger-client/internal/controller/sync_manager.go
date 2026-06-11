package controller

import (
	"claviger-client/internal/api"
	"claviger-client/internal/config"
	"context"

	"log"
	"strconv"
	"time"
)

type VPNController interface {
	HotSwapEndpoint(key, endpoint string) error
	GetState() string
}

func StartSyncManager(ctx context.Context, vault *config.ClientVault, vpn VPNController) {
	ticker := time.NewTicker(5 * time.Minute)

	go func() {
		for {
			<-ticker.C

			// 1. Ensure we are actually connected before we poll
			if vpn.GetState() != "Connected" {
				continue // Tunnel isn't active, no need to sync
			}

			// 2. Dynamic Profile Injection
			if vault.ActiveProfileID == "" {
				continue
			}
			profile, exists := vault.Profiles[vault.ActiveProfileID]
			if !exists {
				continue
			}

			currentRev := 0
			if profile.ConfigRevision != "" {
				currentRev, _ = strconv.Atoi(profile.ConfigRevision)
			}

			// 3. Initialize API Client & Fetch
			syncer := api.NewSyncClient(profile.ServerEndpoint, "wg0", currentRev)
			state, err := syncer.FetchServerState(vault.DeviceID)

			if err != nil {
				if err.Error() == "REVOKED" {
					log.Printf("🚨 CRITICAL: Device revoked by server for profile %s!", profile.Name)
					// Handle revocation (e.g., call vpnEngine.Disconnect())
				}
				continue
			}

			// 4. Revision Comparison
			serverRev, err := strconv.Atoi(state.Revision)
			if err != nil {
				continue // Bad data from server
			}

			if serverRev > currentRev {
				log.Printf("🔄 Update found for %s (v%d -> v%d). Reconciling...", profile.Name, currentRev, serverRev)

				// 5. 🎯 HOT-SWAP VIA EMBEDDED ENGINE
				err = vpn.HotSwapEndpoint(profile.ServerKey, state.Endpoint)
				if err != nil {
					log.Printf("❌ Failed to apply new config to interface: %v", err)
					continue
				}

				// 6. Save the new state
				profile.ServerEndpoint = state.Endpoint
				profile.DNS = state.DNS
				profile.ConfigRevision = state.Revision

				if err := config.Save(vault); err != nil {
					log.Printf("⚠️ Interface updated, but failed to save vault to disk: %v", err)
				} else {
					log.Println("✅ Tunnel hot-swapped and vault saved successfully.")
				}
			}
		}
	}()
}
