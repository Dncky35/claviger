package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Client represents a VPN peer in the database
type Client struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	IPAddress string `json:"ip_address"`
	RoleID    string `json:"role_id"`
	CreatedAt string `json:"created_at"`
}

// CreateClientReq is the payload from the Frontend UI
type CreateClientReq struct {
	Name   string `json:"name"`
	RoleID string `json:"role_id"`
}

// CreateClientRes returns the "Burn After Reading" config details
type CreateClientRes struct {
	Client     Client `json:"client"`
	PrivateKey string `json:"private_key"` // NEVER saved to DB
	ServerIP   string `json:"server_ip"`
}

// getNextAvailableIP scans the database and finds the next 10.8.0.x address
func getNextAvailableIP(db *sql.DB) (string, error) {
	// Start at 10.8.0.2 because .1 is the Claviger Server
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

// HandleClients manages listing and creating VPN peers
func HandleClients(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// --- GET: List all current clients ---
		if r.Method == http.MethodGet {
			rows, err := db.Query("SELECT id, name, public_key, ip_address, role_id, created_at FROM clients ORDER BY created_at DESC")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			clients := []Client{}
			for rows.Next() {
				var c Client
				rows.Scan(&c.ID, &c.Name, &c.PublicKey, &c.IPAddress, &c.RoleID, &c.CreatedAt)
				clients = append(clients, c)
			}
			json.NewEncoder(w).Encode(clients)
			return
		}

		// --- POST: Create a new client ---
		if r.Method == http.MethodPost {
			var req CreateClientReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			// 1. Generate WireGuard Keys
			privKey, err := wgtypes.GeneratePrivateKey()
			if err != nil {
				http.Error(w, "Failed to generate keys", http.StatusInternalServerError)
				return
			}
			pubKey := privKey.PublicKey()

			// 2. Get the next available IP Address
			assignIP, err := getNextAvailableIP(db)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// 3. Save to Local SQLite Database (Notice we DO NOT save the private key)
			clientID := uuid.New().String()
			_, err = db.Exec(`
				INSERT INTO clients (id, name, public_key, ip_address, role_id) 
				VALUES (?, ?, ?, ?, ?)`,
				clientID, req.Name, pubKey.String(), assignIP, req.RoleID,
			)
			if err != nil {
				http.Error(w, "Failed to save client to database", http.StatusInternalServerError)
				return
			}

			// 4. Hot-Inject into the Linux Kernel
			wg, err := wgctrl.New()
			if err == nil {
				defer wg.Close()
				_, ipNet, _ := net.ParseCIDR(assignIP + "/32")

				peerConfig := wgtypes.PeerConfig{
					PublicKey:         pubKey,
					ReplaceAllowedIPs: true,
					AllowedIPs:        []net.IPNet{*ipNet},
				}

				wg.ConfigureDevice("wg0", wgtypes.Config{
					Peers: []wgtypes.PeerConfig{peerConfig},
				})
				log.Printf("🛡️ New peer injected into kernel: %s (%s)", req.Name, assignIP)
			}

			// 5. Send the "Burn After Reading" payload back to the UI
			res := CreateClientRes{
				Client: Client{
					ID:        clientID,
					Name:      req.Name,
					PublicKey: pubKey.String(),
					IPAddress: assignIP,
					RoleID:    req.RoleID,
					CreatedAt: time.Now().Format(time.RFC3339),
				},
				PrivateKey: privKey.String(),
				// Hardcoding server IP for the moment, we will make this dynamic later!
				ServerIP: "YOUR_SERVER_PUBLIC_IP",
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(res)
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
