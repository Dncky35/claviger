package network

import (
	"claviger-server/storage"
	"context"
	"database/sql"
	"log"
	"net"
	"time"
)

// GetPrimaryLocalIP dials a public DNS server via UDP to determine the
// preferred outbound local IP address of the host machine.
// This is much safer than iterating interfaces, as it skips Docker/VPN interfaces.
func GetPrimaryLocalIP() (string, error) {
	// We use UDP because it doesn't actually establish a connection/handshake.
	// It just asks the OS networking stack "Which IP would you use to reach 8.8.8.8?"
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// RunIPWatchdog runs as a background goroutine. It checks the local IP and triggers
// network self-healing if the router assigned a new IP after a reboot.
func RunIPWatchdog(ctx context.Context, db *sql.DB, checkInterval time.Duration) {
	log.Println("[Watchdog] IP Watchdog initialized. Monitoring for DHCP changes...")

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[Watchdog] Shutting down IP Watchdog...")
			return
		case <-ticker.C:

			watchdogEnabled := storage.GetConfig(db, "ip_watchdog_enabled")

			if watchdogEnabled != "true" {
				// Silently skip. We don't want to overwrite a VPS domain.
				continue
			}

			currentIP, err := GetPrimaryLocalIP()
			if err != nil {
				log.Printf("⚠️ [Watchdog] Failed to get local IP: %v", err)
				continue
			}

			lastKnownIP := storage.GetConfig(db, "vpn_endpoint")

			// 🛡️ ERROR HANDLING / INITIALIZATION:
			// If it's empty, it means this is the first time the watchdog is running,
			// or the database value was accidentally wiped.
			if lastKnownIP == "" {
				log.Printf("[Watchdog] No previous IP found in DB. Saving initial Local IP: %s", currentIP)
				err := storage.SetConfig(db, "vpn_endpoint", currentIP)
				if err != nil {
					log.Printf("❌ [Watchdog] Failed to save initial IP to database: %v", err)
				}
				// Skip the rest of the loop since there is nothing to compare against yet.
				continue
			}

			// 🚨 DETECTED A CHANGE (Electricity outage, router reboot, DHCP lease expired)
			if currentIP != lastKnownIP {
				log.Printf("[Watchdog] LOCAL IP CHANGED! Old: %s -> New: %s", lastKnownIP, currentIP)

				// Execute the Self-Healing Sequence
				// healNetwork(db, lastKnownIP, currentIP)

				// Update DB so we don't heal again until it changes again
				err := storage.SetConfig(db, "vpn_endpoint", currentIP)
				if err != nil {
					log.Printf("❌ [Watchdog] Failed to update new IP in database: %v", err)
				} else {
					log.Println("✅ [Watchdog] Database updated with new local IP.")
				}
			}
		}
	}
}

// // healNetwork orchestrates the updates when the IP shifts
// func healNetwork(db *sql.DB, oldIP, newIP string) {
// 	log.Println("🩹 [Watchdog] Initiating Self-Healing protocol...")

// 	// Step 1: (Future) Update AdGuard DNS rewrites
// 	// If panel.cloudrocean.com pointed to oldIP, we use the AdGuard API to change it to newIP
// 	// adguard.UpdateRewrite(oldIP, newIP)

// 	// Step 2: (Future) Ping Dynamic DNS or Cloudflare API
// 	// cloudflare.UpdateRecord(newIP)

// 	// Step 3: Trigger a UI notification event via IPC so the Admin sees it in the dashboard
// 	// ipc.BroadcastEvent("NETWORK_HEALED", "Local IP updated to " + newIP)

// 	log.Println("🩹 [Watchdog] Network Self-Healing complete.")
// }
