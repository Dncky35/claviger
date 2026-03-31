package api

import (
	"claviger-server/storage"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
)

// HandleClientStatus allows a waiting client to poll for its approval status
func HandleClientStatus(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			http.Error(w, `{"status": "error", "message": "Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		// The client will pass its ID in the URL, e.g., /api/client/status?id=1234-abcd
		clientID := r.URL.Query().Get("id")
		if clientID == "" {
			http.Error(w, `{"status": "error", "message": "Client ID is required"}`, http.StatusBadRequest)
			return
		}

		var status string
		var assignedIP sql.NullString

		err := db.QueryRow("SELECT status, ip_address FROM clients WHERE id = ?", clientID).Scan(&status, &assignedIP)

		if err == sql.ErrNoRows {
			// If they were denied and deleted by the admin
			json.NewEncoder(w).Encode(map[string]string{"status": "rejected"})
			return
		} else if err != nil {
			fmt.Printf("🚨 CLIENT STATUS DB ERROR: %v\n", err)
			http.Error(w, `{"status": "error", "message": "Database error"}`, http.StatusInternalServerError)
			return
		}

		if status == "pending" {
			json.NewEncoder(w).Encode(map[string]string{"status": "pending"})
			return
		}

		// If they are active, hand over the keys to the kingdom!
		if status == "active" {
			serverPubKey := storage.GetConfig(db, "wg_public_key")
			wgPort := storage.GetConfig(db, "wg_port")
			if wgPort == "" {
				wgPort = "51820"
			}
			hubIP := storage.GetConfig(db, "hub_ip")
			if hubIP == "" {
				hubIP = "10.8.0.1"
			}

			json.NewEncoder(w).Encode(map[string]string{
				"status":            "active",
				"assigned_ip":       assignedIP.String,
				"server_public_key": serverPubKey,
				"wg_port":           wgPort,
				"hub_ip":            hubIP,
			})
			return
		}
	}
}
