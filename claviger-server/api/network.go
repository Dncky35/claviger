package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"claviger-server/internal/firewall"
	"claviger-server/storage"
)

// InternetReq is the payload expected from the UI toggle
type InternetReq struct {
	Enable bool `json:"enable"`
}

// HandleNetworkSettings manages global routing configurations
func HandleNetworkSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// --- GET: Check current status ---
		if r.Method == http.MethodGet {
			// Read from database. Default is "false" (isolated mode) if not set.
			statusStr := storage.GetConfig(db, "allow_global_internet")
			isEnabled := statusStr == "true"

			json.NewEncoder(w).Encode(map[string]bool{"internet_enabled": isEnabled})
			return
		}

		// --- POST: Toggle Internet Routing ---
		if r.Method == http.MethodPost {
			var req InternetReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"status":"error", "message":"Invalid request payload"}`, http.StatusBadRequest)
				return
			}

			// 1. Update the actual Linux Firewall
			var err error
			if req.Enable {
				err = firewall.EnableInternet()
			} else {
				err = firewall.DisableInternet()
			}

			if err != nil {
				http.Error(w, `{"status":"error", "message":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}

			// 2. Save state to SQLite so it survives reboots
			stateStr := "false"
			if req.Enable {
				stateStr = "true"
			}
			storage.SetConfig(db, "allow_global_internet", stateStr)

			json.NewEncoder(w).Encode(map[string]string{
				"status":  "success",
				"message": "Global routing updated successfully.",
			})
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
