package controller

import (
	"claviger-client/internal/api"
	"claviger-client/internal/config"
	"context"
	"fmt"
	"runtime"
	"strings"

	"log"
	"strconv"
	"time"
)

const (
	SyncStable   = "Stable"
	SyncChecking = "Checking for updates..."
	SyncUpdating = "Syncing: Applying update..."
	SyncFailed   = "Sync failed, retrying..."
	SyncFallback = "Syncing: Fallback initiated..."
)

type VPNController interface {
	HotSwapEndpoint(key, newEndpoint, newDNS, interfaceName string) error
	SetSyncStatus(status string)
	GetState() string
}

func StartSyncManager(ctx context.Context, vault *config.ClientVault, vpn VPNController) {
	log.Println("🔍 [SyncManager] Background worker spawned.")

	go func() {
		// ==========================================
		// 🎯 1. REUSABLE SYNC LOGIC (Closure Function)
		// ==========================================

		performSync := func() {
			// 1. Initial State
			vpn.SetSyncStatus(SyncChecking)

			currentState := vpn.GetState()
			if currentState != "Secured" {
				log.Println("⚠️ [SyncManager] Tunnel is not secured. Skipping sync check.")
				vpn.SetSyncStatus(SyncStable) // Reset to stable if we aren't running
				return
			}

			if vault.ActiveProfileID == "" {
				vpn.SetSyncStatus(SyncStable)
				return
			}
			profile, exists := vault.Profiles[vault.ActiveProfileID]
			if !exists {
				vpn.SetSyncStatus(SyncStable)
				return
			}

			currentRev := 0
			if profile.ConfigRevision != "" {
				currentRev, _ = strconv.Atoi(profile.ConfigRevision)
			}

			hubIP := "10.8.0.1"
			if strings.HasSuffix(profile.BaseSubnet, ".0/24") {
				hubIP = strings.TrimSuffix(profile.BaseSubnet, ".0/24") + ".1"
			}

			apiPort := profile.HubPort
			if apiPort == "" {
				apiPort = "10880"
			}

			apiBaseURL := fmt.Sprintf("http://%s:%s", hubIP, apiPort)
			log.Printf("🌐 [SyncManager] Reaching out to INTERNAL API: %s (Current Rev: %d)", apiBaseURL, currentRev)

			syncer := api.NewSyncClient(apiBaseURL, "wg0", currentRev)
			state, err := syncer.FetchServerState(vault.DeviceID)

			if err != nil {
				if err.Error() == "REVOKED" {
					log.Printf("🚨 [SyncManager] CRITICAL: Device revoked by server for profile %s!", profile.Name)
					// Handle revocation (e.g., call vpnEngine.Disconnect())
				} else {
					log.Printf("❌ [SyncManager] Fetch failed: %v", err)
				}
				vpn.SetSyncStatus(SyncStable)
				return
			}

			serverRev, err := strconv.Atoi(state.Revision)
			if err != nil {
				log.Printf("❌ [SyncManager] Server returned invalid revision string: '%s'", state.Revision)
				vpn.SetSyncStatus(SyncStable)
				return
			}

			log.Printf("📊 [SyncManager] Comparison -> Local: v%d | Server: v%d", currentRev, serverRev)

			if serverRev > currentRev {
				log.Printf("🔄 [SyncManager] Update found for %s! Reconciling...", profile.Name)
				vpn.SetSyncStatus(SyncUpdating)

				oldEndpoint := profile.ServerEndpoint
				oldDNS := profile.DNS
				oldRevision := profile.ConfigRevision

				interfaceName := "claviger0"
				if runtime.GOOS == "darwin" {
					interfaceName = "utun"
				}

				err = vpn.HotSwapEndpoint(profile.ServerKey, state.Endpoint, state.DNS, interfaceName)
				if err != nil {
					log.Printf("❌ [SyncManager] Failed to apply new config to interface: %v", err)
					return
				}

				log.Println("⏳ [SyncManager] Hot-Swap applied. Waiting 3 seconds for WG Handshake...")
				time.Sleep(3 * time.Second)

				log.Println("🛡️ [SyncManager] Running Fallback Health Check...")
				_, healthErr := syncer.FetchServerState(vault.DeviceID)

				if healthErr != nil {
					log.Printf("🚨 [SyncManager] HEALTH CHECK FAILED: %v", healthErr)
					log.Println("⏪ [SyncManager] INITIATING FALLBACK to previous known-good endpoint...")
					vpn.SetSyncStatus(SyncFallback)

					revertErr := vpn.HotSwapEndpoint(profile.ServerKey, oldEndpoint, oldDNS, interfaceName)
					if revertErr != nil {
						log.Printf("🔥 [SyncManager] CRITICAL: Fallback failed to apply: %v", revertErr)
					} else {
						log.Println("✅ [SyncManager] Fallback successful. Tunnel restored to previous state.")
					}
					profile.ConfigRevision = oldRevision
					vpn.SetSyncStatus(SyncStable)
					return
				}

				log.Println("✅ [SyncManager] Health Check Passed! Committing new state to Vault.")
				profile.ServerEndpoint = state.Endpoint
				profile.DNS = state.DNS
				profile.ConfigRevision = state.Revision

				if err := config.Save(vault); err != nil {
					log.Printf("⚠️ [SyncManager] Interface updated, but failed to save vault to disk: %v", err)
				} else {
					log.Println("✅ [SyncManager] Tunnel hot-swapped and vault saved successfully.")
				}
			} else {
				log.Println("✅ [SyncManager] Local configuration is up to date.")
			}
			vpn.SetSyncStatus(SyncStable)
		}

		// ==========================================
		// 🎯 2. WAIT FOR INITIAL HANDSHAKE
		// ==========================================
		log.Println("⏳ [SyncManager] Waiting for VPN handshake to complete before first check...")
		for vpn.GetState() != "Secured" {
			select {
			case <-ctx.Done():
				log.Println("🛑 [SyncManager] Aborted before handshake completed.")
				return
			case <-time.After(1 * time.Second): // Check again every 1 second
			}
		}

		// ==========================================
		// 🎯 3. FIRST IMMEDIATE CHECK
		// ==========================================
		log.Println("⚡ [SyncManager] Handshake confirmed! Running initial sync...")
		performSync()

		// ==========================================
		// 🎯 4. START THE BACKGROUND TICKER
		// ==========================================
		log.Println("⏰ [SyncManager] Starting 5-minute interval timer.")
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("🛑 [SyncManager] Context canceled. Shutting down background sync loop.")
				return
			case <-ticker.C:
				log.Println("⏰ [SyncManager] Timer tick! Waking up to check sync state...")
				performSync()
			}
		}
	}()
}
