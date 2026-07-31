package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
)

// Client represents a VPN peer in the database + live kernel state
type Client struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PublicKey     string `json:"public_key"`
	IPAddress     string `json:"ip_address"`
	RoleID        string `json:"role_id"`
	Platform      string `json:"platform"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	IsOnline      bool   `json:"is_online"`      // NEW: Live presence detection
	LastHandshake string `json:"last_handshake"` // NEW: Exact time of last packet

	// --- New Sub-Server Fields ---
	IsSubServer     bool   `json:"is_sub_server"`
	SubServerStatus string `json:"sub_server_status,omitempty"`
	SubServerID     string `json:"sub_server_id,omitempty"`
}

// HandleClients manages listing the VPN peers for the Hub UI
func HandleClients(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodGet {

			// 1. Fetch the static configurations from SQLite (Now with LEFT JOIN for sub_servers)
			query := `
        SELECT 
            c.id, c.name, c.public_key, c.ip_address, c.role_id, c.platform, c.status, c.created_at,
            s.id AS sub_server_id, 
            s.status AS sub_server_status
        FROM clients c
        LEFT JOIN sub_servers s ON c.id = s.client_id
        ORDER BY c.created_at DESC`

			rows, err := db.Query(query)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			clients := []Client{}
			for rows.Next() {
				var c Client
				var ip, platform, subServerID, subServerStatus sql.NullString

				// Scan all 10 columns
				err := rows.Scan(
					&c.ID, &c.Name, &c.PublicKey, &ip, &c.RoleID, &platform, &c.Status, &c.CreatedAt,
					&subServerID, &subServerStatus,
				)

				if err != nil {
					log.Printf("Error scanning client row: %v", err)
					continue
				}

				if ip.Valid {
					c.IPAddress = ip.String
				}
				if platform.Valid {
					c.Platform = platform.String
				}

				// Check if this client is also registered as a sub-server
				if subServerID.Valid && subServerID.String != "" {
					c.IsSubServer = true
					c.SubServerID = subServerID.String
					c.SubServerStatus = subServerStatus.String // This will be 'pending' or 'active'
				}

				clients = append(clients, c)
			}

			// 2. Fetch the LIVE connection states from the Linux Kernel!
			wgPeers := make(map[string]time.Time)
			wg, err := wgctrl.New()
			if err == nil {
				defer wg.Close()
				dev, err := wg.Device("wg0")
				if err == nil {
					// Map every connected public key to its last handshake time
					for _, peer := range dev.Peers {
						wgPeers[peer.PublicKey.String()] = peer.LastHandshakeTime
					}
				}
			}

			// 3. Merge the DB data with the Kernel data
			for i := range clients {
				if clients[i].Status == "active" {
					if hsTime, exists := wgPeers[clients[i].PublicKey]; exists {
						// WireGuard handshakes happen every ~2 minutes.
						// If we saw one in the last 3 minutes, they are ONLINE!
						if !hsTime.IsZero() && time.Since(hsTime) < 3*time.Minute {
							clients[i].IsOnline = true
						}

						if !hsTime.IsZero() {
							clients[i].LastHandshake = hsTime.Format(time.RFC3339)
						}
					}
				}
			}

			// 4. Send the enriched data back to the UI
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(clients)
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
