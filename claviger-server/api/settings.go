package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"claviger-server/network"
	"claviger-server/storage" // Use your existing storage package!
)

// Add the 'Force' boolean
type EndpointReq struct {
	Endpoint string `json:"endpoint"`
	Force    bool   `json:"force"`
}

// HandleGetEndpoint returns the current custom domain/IP from the config table
func HandleGetEndpoint(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"status": "error", "message": "Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		// Use your existing GetConfig function
		endpoint := storage.GetConfig(database, "vpn_endpoint")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "success",
			"endpoint": endpoint,
		})
	}
}

// HandleSaveEndpoint updates the custom domain/IP with Soft Verification
func HandleSaveEndpoint(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"status": "error", "message": "Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req EndpointReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"status": "error", "message": "Invalid request"}`, http.StatusBadRequest)
			return
		}

		// --- THE SOFT VERIFICATION ENGINE ---
		// --- THE SOFT VERIFICATION ENGINE ---
		if !req.Force {
			// THE FIX: Fetch from local DB instead of pinging the internet!
			// This makes the UI respond in 1 millisecond instead of 10 seconds.
			serverIP := storage.GetConfig(database, "public_ip")
			if serverIP == "" {
				serverIP = "127.0.0.1" // Safe fallback if DB is empty
			}

			status, warningMsg := network.VerifyEndpoint(req.Endpoint, serverIP)

			// If it's a mismatch or missing record, reject it with a WARNING status
			if status == network.StatusMismatch || status == network.StatusNoRecord {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict) // 409 Conflict triggers the UI Modal
				json.NewEncoder(w).Encode(map[string]string{
					"status":  "warning",
					"message": warningMsg,
				})
				return
			}
		}
		// ------------------------------------

		// If it passed, or if the user clicked "Force Save", we write to DB
		err := storage.SetConfig(database, "vpn_endpoint", req.Endpoint)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "Database error"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}
}
