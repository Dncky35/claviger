package api

import (
	"claviger-server/storage"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// These MUST be outside the function so heartbeat.go can modify them!
var (
	CloudSyncStatus string = "Waiting for first sync..."
	LastCloudSync   time.Time
)

// HandleStatus returns a handler that serves the VPN status to the local Hub.
func HandleStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		lastSyncStr := ""
		if !LastCloudSync.IsZero() {
			lastSyncStr = LastCloudSync.Format(time.RFC3339)
		}

		status := map[string]interface{}{
			"node_id":           "test",
			"has_token":         "true",
			"vpn_state":         "active",
			"cloud_sync_status": CloudSyncStatus,
			"last_cloud_sync":   lastSyncStr,
		}

		json.NewEncoder(w).Encode(status)
	}
}

func HandleSyncState(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Validate the Request Sender
		// Extract an identifier from the client (e.g., a device key or auth token)
		deviceKey := r.Header.Get("X-Device-Key")
		if deviceKey == "" {
			http.Error(w, "Unauthorized: Missing device identifier", http.StatusUnauthorized)
			return
		}

		// Verify the sender exists in the database.
		exists, err := storage.DeviceExists(db, deviceKey)
		if err != nil {
			log.Printf("Database error during device validation: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "Forbidden: Device not registered or revoked", http.StatusForbidden)
			return
		}

		// 2. Fetch Configurations
		endpoint := storage.GetConfig(db, "vpn_endpoint")
		wgPort := storage.GetConfig(db, "wg_port")

		// 🎯 ADAPTIVE HUB IP
		hubIP := storage.GetConfig(db, "hub_ip")
		if hubIP == "" {
			hubIP = "10.8.0.1"
		}

		dns := hubIP // AdGuard runs directly on the Hub IP!
		if storage.GetConfig(db, "app_adguard_port") == "" {
			dns = "1.1.1.1, 1.0.0.1" // AdGuard missing, use Cloudflare
		}

		revision := storage.GetConfig(db, "config_revision")

		// 3. Validate Required Data
		if endpoint == "" {
			// Abort the flow and inform the client that the server configuration is incomplete
			log.Println("Sync state failed: vpn_endpoint is missing in database")
			http.Error(w, "Service Unavailable: VPN endpoint not configured", http.StatusServiceUnavailable)
			return
		}

		if revision == "" {
			// Provide a fallback or error out depending on how strict you want to be
			revision = "1"
		}

		mtu := "1420" // Default WireGuard MTU

		// 4. Construct the State Manifest
		state := map[string]string{
			"server_endpoint": fmt.Sprintf("%s:%s", endpoint, wgPort),
			"dns":             dns,
			"mtu":             mtu,
			"revision":        revision,
		}

		// 5. Send Response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(state); err != nil {
			log.Printf("Error encoding sync state JSON: %v", err)
			// Note: We don't write an http.Error here because WriteHeader(200) was already sent
		}
	}
}
