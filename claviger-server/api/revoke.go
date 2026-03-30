package api

import (
	"claviger-server/internal/firewall"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// RevokeReq is the payload sent when the Admin clicks "Revoke"
type RevokeReq struct {
	ClientID string `json:"client_id"`
}

// HandleRevoke permanently deletes a client and kicks them off the VPN
func HandleRevoke(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json") // Ensure all responses are JSON

		if r.Method != http.MethodPost {
			http.Error(w, `{"status": "error", "message": "Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req RevokeReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"status": "error", "message": "Invalid request payload"}`, http.StatusBadRequest)
			return
		}

		// 1. Fetch the client's public key before we delete them
		var pubKeyStr, clientName string
		var clientIP sql.NullString

		err := db.QueryRow("SELECT public_key, name, ip_address FROM clients WHERE id = ?", req.ClientID).Scan(&pubKeyStr, &clientName, &clientIP)
		if err == sql.ErrNoRows {
			http.Error(w, `{"status": "error", "message": "Client not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"status": "error", "message": "Database error"}`, http.StatusInternalServerError)
			return
		}

		// --- Clean up the Firewall ---
		if clientIP.Valid && clientIP.String != "" {
			firewall.RemoveRoleRules(clientIP.String)
		}

		// 2. Hot-Remove from the Linux Kernel
		wg, err := wgctrl.New()
		if err == nil {
			defer wg.Close()

			pubKey, parseErr := wgtypes.ParseKey(pubKeyStr)
			if parseErr == nil {
				// The 'Remove: true' flag tells WireGuard to instantly drop this peer
				peerConfig := wgtypes.PeerConfig{
					PublicKey: pubKey,
					Remove:    true,
				}

				// We intentionally ignore the error here so that even if the kernel fails
				// (e.g., interface is temporarily down), we still delete them from the DB.
				if wgErr := wg.ConfigureDevice("wg0", wgtypes.Config{Peers: []wgtypes.PeerConfig{peerConfig}}); wgErr == nil {
					log.Printf("🚫 Access Revoked: %s removed from kernel", clientName)
				} else {
					log.Printf("⚠️ Failed to remove %s from kernel: %v", clientName, wgErr)
				}
			}
		}

		// 3. Delete from the database to free up the IP address
		_, err = db.Exec("DELETE FROM clients WHERE id = ?", req.ClientID)
		if err != nil {
			http.Error(w, `{"status": "error", "message": "Failed to delete client from database"}`, http.StatusInternalServerError)
			return
		}

		// 4. Respond with Success
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": clientName + " has been permanently revoked and disconnected.",
		})
	}
}
