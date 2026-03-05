package api

import (
	"encoding/json"
	"net/http"
)

// HandleStatus returns a handler that serves the VPN status to the local Hub.
func HandleStatus(nodeID string, hasToken bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		status := map[string]interface{}{
			"node_id":   nodeID,
			"has_token": hasToken,
			"vpn_state": "active", // We will make this dynamically reflect "Paused" later
		}

		json.NewEncoder(w).Encode(status)
	}
}
