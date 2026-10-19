//go:build !windows

package daemon

import (
	"context"
	"log"
	"time"

	"claviger-client/internal/config"
	"claviger-client/internal/vpn"
)

func RunDaemon(ctx context.Context, vault *config.ClientVault, engine *vpn.Engine) {
	log.Println("🐧 Running Linux Daemon...")

	// AUTO-CONNECT LOGIC
	if vault.AutoConnect && vault.ActiveProfileID != "" {
		if profile, exists := vault.Profiles[vault.ActiveProfileID]; exists {
			log.Printf("🔄 Auto-Connect enabled. Queuing tunnel boot for %s...", profile.Name)

			go func() {
				retryDelay := 2 * time.Second
				maxDelay := 30 * time.Second

				for {
					// 1. Check if the daemon is shutting down before trying
					select {
					case <-ctx.Done():
						log.Println("🛑 Auto-Connect aborted due to daemon shutdown.")
						return
					default:
						// Proceed to connect
					}

					// 2. Attempt the connection
					err := engine.Connect(vault, profile, vault.UseGlobalRouting)
					if err == nil {
						log.Println("✅ Auto-Connect successful! Network established.")
						return // Break out of the loop once connected
					}

					// 3. Handle Failure and Backoff
					log.Printf("⏳ Network not ready (%v). Retrying in %v...", err, retryDelay)

					select {
					case <-ctx.Done():
						log.Println("🛑 Auto-Connect aborted during backoff sleep.")
						return
					case <-time.After(retryDelay):
						retryDelay *= 2
						if retryDelay > maxDelay {
							retryDelay = maxDelay
						}
					}
				}
			}()
		}
	}

	// Wait until systemd/SIGTERM cancels the context
	<-ctx.Done()
	log.Println("🛑 Linux systemd requested shutdown. Tearing down Claviger Engine...")

	// 🚨 CRITICAL: Clean up WireGuard adapters, routes, and DNS rules before process exit
	if engine != nil && engine.GetState() != "Disconnected" {
		if err := engine.Disconnect(); err != nil {
			log.Printf("⚠️ Engine teardown encountered an error: %v", err)
		} else {
			log.Println("✅ Network adapters and firewall rules cleanly restored.")
		}
	}

	log.Println("👋 Linux Daemon shutting down gracefully...")
}
