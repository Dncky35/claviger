package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// HandleNodeRegistration receives the node_secret from the Sub-server and sets it to 'pending'
func HandleNodeRegistration(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Method check
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 2. Decode the incoming JSON payload
		var req struct {
			VpnIP   string `json:"vpn_ip"`
			NodeKey string `json:"node_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		// 3. Zero Trust Security Check: Verify actual network source IP
		// r.RemoteAddr usually looks like "10.8.0.5:43928". We split it to get just the IP.
		actualIP := strings.Split(r.RemoteAddr, ":")[0]

		// If the HTTP request came from a different IP than what is in the payload, drop it.
		if actualIP != req.VpnIP {
			log.Printf("⚠️ Security Alert: Node registration IP spoofing attempt. Payload IP: %s, Actual IP: %s", req.VpnIP, actualIP)
			http.Error(w, "Security violation: IP address mismatch", http.StatusForbidden)
			return
		}

		// 4. Look up the exact client_id using the provided VPN IP
		var clientID string
		lookupQuery := `SELECT id FROM clients WHERE ip_address = ?`
		err := db.QueryRow(lookupQuery, req.VpnIP).Scan(&clientID)

		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("❌ Registration failed: No WireGuard client found with IP %s", req.VpnIP)
				http.Error(w, "Client not found in Master database", http.StatusNotFound)
			} else {
				log.Printf("❌ Database error looking up IP %s: %v", req.VpnIP, err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		// 5. Insert or Update the sub_servers table
		// We use ON CONFLICT so if the user registers the same node twice, it just updates the key and goes back to 'pending'
		subServerID := uuid.New().String()
		insertQuery := `
			INSERT INTO sub_servers (id, client_id, api_key, status)
			VALUES (?, ?, ?, 'pending')
			ON CONFLICT(client_id) DO UPDATE 
			SET api_key = excluded.api_key, status = 'pending';`

		_, err = db.Exec(insertQuery, subServerID, clientID, req.NodeKey)
		if err != nil {
			log.Printf("❌ Failed to insert sub_server record for client %s: %v", clientID, err)
			http.Error(w, "Failed to register sub-server", http.StatusInternalServerError)
			return
		}

		log.Printf("✅ Node %s (%s) registered successfully. Awaiting admin approval.", req.VpnIP, clientID)

		// 6. Return success to the Sub-server CLI
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "Node registered successfully. Awaiting approval on Master UI.",
		})
	}
}

func HandleNodeApproval(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			ClientID string `json:"client_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid payload", http.StatusBadRequest)
			return
		}

		// Update the status to active
		updateQuery := `UPDATE sub_servers SET status = 'active' WHERE client_id = ? AND status = 'pending'`
		result, err := db.Exec(updateQuery, req.ClientID)
		if err != nil {
			log.Printf("❌ Failed to approve node for client %s: %v", req.ClientID, err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			http.Error(w, "Node not found or already active", http.StatusNotFound)
			return
		}

		log.Printf("✅ Node %s approved and is now ACTIVE.", req.ClientID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "active"})
	}
}

func HandleNodeRemoval(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ClientID string `json:"client_id"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		_, err := db.Exec("DELETE FROM sub_servers WHERE client_id = ?", req.ClientID)
		if err != nil {
			http.Error(w, "Failed to remove node", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

// SubServerNode represents the minimal data needed for UI node listings
type SubServerNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	IPAddress string `json:"ip_address"` // Helpful to have for future UI expansions
}

// HandleGetSubServers returns a list of all registered nodes for the sidebar
func HandleGetSubServers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Perform an INNER JOIN to get the client name and IP tied to the sub-server
		query := `
			SELECT s.id, c.name, s.status, c.ip_address 
			FROM sub_servers s
			JOIN clients c ON s.client_id = c.id
			ORDER BY c.name ASC`

		rows, err := db.Query(query)
		if err != nil {
			log.Printf("❌ Failed to fetch sub-servers: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var nodes []SubServerNode
		for rows.Next() {
			var n SubServerNode
			var ip sql.NullString

			if err := rows.Scan(&n.ID, &n.Name, &n.Status, &ip); err != nil {
				log.Printf("⚠️ Error scanning sub_server row: %v", err)
				continue
			}

			if ip.Valid {
				n.IPAddress = ip.String
			}
			nodes = append(nodes, n)
		}

		// Ensure we return an empty array [] instead of null if there are no nodes
		if nodes == nil {
			nodes = []SubServerNode{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(nodes)
	}
}
