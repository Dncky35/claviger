package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// Client represents a VPN peer in the database
type Client struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	IPAddress string `json:"ip_address"`
	RoleID    string `json:"role_id"`
	Platform  string `json:"platform"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// HandleClients manages listing the VPN peers for the Hub UI
func HandleClients(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// --- GET: List all current clients (Pending, Active, Suspended) ---
		if r.Method == http.MethodGet {
			// Notice we are querying the new columns: platform and status
			rows, err := db.Query("SELECT id, name, public_key, ip_address, role_id, platform, status, created_at FROM clients ORDER BY created_at DESC")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			clients := []Client{}
			for rows.Next() {
				var c Client
				// We use sql.NullString because pending clients don't have IPs or platforms yet
				var ip, platform sql.NullString

				rows.Scan(&c.ID, &c.Name, &c.PublicKey, &ip, &c.RoleID, &platform, &c.Status, &c.CreatedAt)

				// Safely parse the null strings
				if ip.Valid {
					c.IPAddress = ip.String
				}
				if platform.Valid {
					c.Platform = platform.String
				}

				clients = append(clients, c)
			}

			json.NewEncoder(w).Encode(clients)
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
