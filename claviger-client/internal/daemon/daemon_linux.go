//go:build !windows

package daemon

import (
	"claviger-client/internal/config"
	"claviger-client/internal/vpn"
	"context"
	"log"
	"time"
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
						return // Break out of the loop, our job here is done
					}

					// 3. Handle Failure and Backoff
					log.Printf("⏳ Network not ready (%v). Retrying in %v...", err, retryDelay)

					// Wait for the delay duration, but also listen for shutdown signals
					select {
					case <-ctx.Done():
						log.Println("🛑 Auto-Connect aborted during backoff sleep.")
						return
					case <-time.After(retryDelay):
						// Sleep finished, calculate the next delay (Exponential Backoff)
						retryDelay *= 2
						if retryDelay > maxDelay {
							retryDelay = maxDelay // Cap the maximum wait time
						}
					}
				}
			}()
		}
	}

	// Keep the Linux daemon alive until context is cancelled
	<-ctx.Done()
	log.Println("👋 Linux Daemon shutting down gracefully...")
}
