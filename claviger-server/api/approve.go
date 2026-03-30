package api

import (
	"claviger-server/internal/firewall"
	"claviger-server/storage"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// ApproveReq is the payload sent when the Admin clicks "Approve"
type ApproveReq struct {
	ClientID string `json:"client_id"`
}

// getNextAvailableIP scans the database and finds the next empty 10.8.0.x address
func getNextAvailableIP(db *sql.DB) (string, error) {
	// Start at 10.8.0.2 because .1 is the Claviger Hub
	for i := 2; i <= 254; i++ {
		testIP := fmt.Sprintf("10.8.0.%d", i)
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM clients WHERE ip_address = ?)", testIP).Scan(&exists)
		if err != nil {
			return "", err
		}
		if !exists {
			return testIP, nil
		}
	}
	return "", fmt.Errorf("subnet full: no IP addresses available")
}

// HandleApprove promotes a pending client to active and injects them into WireGuard
func HandleApprove(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json") // Always return JSON

		if r.Method != http.MethodPost {
			http.Error(w, `{"status": "error", "message": "Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req ApproveReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"status": "error", "message": "Invalid request"}`, http.StatusBadRequest)
			return
		}

		// 1. Verify the client exists and is actually pending
		var pubKeyStr, status, clientName string
		err := db.QueryRow("SELECT public_key, status, name FROM clients WHERE id = ?", req.ClientID).Scan(&pubKeyStr, &status, &clientName)

		if err == sql.ErrNoRows {
			http.Error(w, `{"status": "error", "message": "Client not found"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"status": "error", "message": "Database error"}`, http.StatusInternalServerError)
			return
		}

		if status != "pending" {
			http.Error(w, `{"status": "error", "message": "Client is already active or suspended"}`, http.StatusBadRequest)
			return
		}

		// 2. IP Address Management (IPAM)
		assignIP, err := getNextAvailableIP(db)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"status": "error", "message": "%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		// 3. Promote the client in the database
		_, err = db.Exec("UPDATE clients SET status = 'active', ip_address = ? WHERE id = ?", assignIP, req.ClientID)
		if err != nil {
			http.Error(w, `{"status": "error", "message": "Failed to update client status"}`, http.StatusInternalServerError)
			return
		}

		// 4. Hot-Inject into the Linux Kernel
		wg, err := wgctrl.New()
		if err == nil {
			defer wg.Close()

			pubKey, parseErr := wgtypes.ParseKey(pubKeyStr)
			if parseErr == nil {
				_, ipNet, _ := net.ParseCIDR(assignIP + "/32")

				peerConfig := wgtypes.PeerConfig{
					PublicKey:         pubKey,
					ReplaceAllowedIPs: true,
					AllowedIPs:        []net.IPNet{*ipNet},
				}

				wg.ConfigureDevice("wg0", wgtypes.Config{
					Peers: []wgtypes.PeerConfig{peerConfig},
				})
				log.Printf("✅ Access Granted: %s injected into kernel at %s", clientName, assignIP)

				// =========================================================================
				// NEW: ENFORCE GRANULAR MICRO-SEGMENTATION
				// =========================================================================
				hubIP := storage.GetConfig(db, "hub_ip")
				if hubIP == "" {
					hubIP = "10.8.0.1"
				}
				hubPort := storage.GetConfig(db, "hub_port")
				if hubPort == "" {
					hubPort = "18080"
				}

				var allowInternet, allowIntranet, allowHub bool
				var allowedPorts, allowedIPs string

				// Fetch the granular rules from the assigned role
				err = db.QueryRow(`
					SELECT allow_global_internet, allow_intranet, allow_hub, allowed_ports, allowed_ips 
					FROM roles WHERE id = (SELECT role_id FROM clients WHERE id = ?)`,
					req.ClientID,
				).Scan(&allowInternet, &allowIntranet, &allowHub, &allowedPorts, &allowedIPs)

				if err == nil {
					ruleConfig := firewall.RoleConfig{
						ClientIP:      assignIP,
						HubIP:         hubIP,
						HubPort:       hubPort,
						AllowInternet: allowInternet,
						AllowIntranet: allowIntranet,
						AllowHub:      allowHub,
						AllowedIPs:    allowedIPs,
						AllowedPorts:  allowedPorts,
					}

					// Apply the strict iptables rules!
					if fwErr := firewall.ApplyRoleRules(ruleConfig); fwErr != nil {
						log.Printf("⚠️ Firewall warning for %s: %v", clientName, fwErr)
					}
				} else {
					log.Printf("⚠️ Failed to fetch role rules for %s: %v", clientName, err)
				}
				// =========================================================================

			} else {
				log.Printf("⚠️ Failed to parse public key for %s: %v", clientName, parseErr)
			}
		} else {
			log.Printf("⚠️ Failed to connect to WireGuard kernel module: %v", err)
		}

		// 5. Respond with Success
		json.NewEncoder(w).Encode(map[string]string{
			"status":     "success",
			"message":    fmt.Sprintf("%s has been approved and assigned %s", clientName, assignIP),
			"ip_address": assignIP,
		})
	}
}
