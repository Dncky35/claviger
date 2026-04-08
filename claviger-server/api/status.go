package api

import (
	"encoding/json"
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
