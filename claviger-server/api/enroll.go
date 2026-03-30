package api

import (
	"claviger-server/storage"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// EnrollReq is the payload sent by the Claviger Client App
type EnrollReq struct {
	Token     string `json:"token"`
	PublicKey string `json:"public_key"`
	Name      string `json:"name"`      // e.g., "Ercan's Mint OS"
	Platform  string `json:"platform"`  // e.g., "linux", "ios", "windows"
	DeviceID  string `json:"device_id"` // Hardware UUID
}

// HandleEnroll processes a client's request to join the network
func HandleEnroll(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json") // Standardize JSON for all responses

		if r.Method != http.MethodPost {
			http.Error(w, `{"status": "error", "message": "Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req EnrollReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"status": "error", "message": "Invalid request"}`, http.StatusBadRequest)
			return
		}

		// 1. Verify the Token
		var roleID, expiresAt string
		var isUsed bool
		err := db.QueryRow("SELECT role_id, expires_at, is_used FROM invitations WHERE token = ?", req.Token).Scan(&roleID, &expiresAt, &isUsed)

		if err == sql.ErrNoRows {
			http.Error(w, `{"status": "error", "message": "Invalid invitation token"}`, http.StatusUnauthorized)
			return
		} else if err != nil {
			http.Error(w, `{"status": "error", "message": "Database error"}`, http.StatusInternalServerError)
			return
		}

		// 2. Security Checks
		if isUsed {
			http.Error(w, `{"status": "error", "message": "This invitation token has already been used"}`, http.StatusForbidden)
			return
		}

		expTime, _ := time.Parse(time.RFC3339, expiresAt)
		if time.Now().After(expTime) {
			http.Error(w, `{"status": "error", "message": "This invitation token has expired"}`, http.StatusForbidden)
			return
		}

		// 3. Burn the Token (Mark as used)
		_, err = db.Exec("UPDATE invitations SET is_used = 1 WHERE token = ?", req.Token)
		if err != nil {
			http.Error(w, `{"status": "error", "message": "Failed to update token"}`, http.StatusInternalServerError)
			return
		}

		clientID := uuid.New().String()

		// =========================================================================
		// 4A. THE ADMIN BYPASS (Auto-Approve Zero Trust Admins)
		// =========================================================================
		if roleID == "admin" {
			// Because approve.go and enroll.go are in the same 'api' package,
			// we can safely call the getNextAvailableIP function from approve.go!
			assignIP, err := getNextAvailableIP(db)
			if err != nil {
				http.Error(w, `{"status": "error", "message": "Subnet full"}`, http.StatusInternalServerError)
				return
			}

			// Save as ACTIVE
			_, err = db.Exec(`
				INSERT INTO clients (id, name, public_key, ip_address, role_id, platform, device_id, status) 
				VALUES (?, ?, ?, ?, ?, ?, ?, 'active')`,
				clientID, req.Name, req.PublicKey, assignIP, roleID, req.Platform, req.DeviceID,
			)

			// Hot-Inject into Kernel
			wg, err := wgctrl.New()
			if err == nil {
				defer wg.Close()
				pubKey, _ := wgtypes.ParseKey(req.PublicKey)
				_, ipNet, _ := net.ParseCIDR(assignIP + "/32")

				wg.ConfigureDevice("wg0", wgtypes.Config{
					Peers: []wgtypes.PeerConfig{{
						PublicKey:         pubKey,
						ReplaceAllowedIPs: true,
						AllowedIPs:        []net.IPNet{*ipNet},
					}},
				})
			}

			// Fetch dynamic server info to hand to the Client App
			serverPubKey := storage.GetConfig(db, "wg_public_key")
			wgPort := storage.GetConfig(db, "wg_port")
			if wgPort == "" {
				wgPort = "51820"
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"status":            "active",
				"message":           "Admin Auto-Approved.",
				"assigned_ip":       assignIP,
				"server_public_key": serverPubKey,
				"wg_port":           wgPort,
			})
			return
		}

		// =========================================================================
		// 4B. NORMAL USER FLOW (Waiting Room)
		// =========================================================================
		_, err = db.Exec(`
			INSERT INTO clients (id, name, public_key, role_id, platform, device_id, status) 
			VALUES (?, ?, ?, ?, ?, ?, 'pending')`,
			clientID, req.Name, req.PublicKey, roleID, req.Platform, req.DeviceID,
		)

		if err != nil {
			http.Error(w, `{"status": "error", "message": "Failed to register client or Public Key already exists"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"status":    "pending",
			"message":   "Successfully enrolled. Waiting for Administrator approval.",
			"client_id": clientID,
		})
	}
}
