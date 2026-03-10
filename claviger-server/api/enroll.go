package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
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
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req EnrollReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// 1. Verify the Token
		var roleID, expiresAt string
		var isUsed bool
		err := db.QueryRow("SELECT role_id, expires_at, is_used FROM invitations WHERE token = ?", req.Token).Scan(&roleID, &expiresAt, &isUsed)

		if err == sql.ErrNoRows {
			http.Error(w, "Invalid invitation token", http.StatusUnauthorized)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		// 2. Security Checks
		if isUsed {
			http.Error(w, "This invitation token has already been used", http.StatusForbidden)
			return
		}

		expTime, _ := time.Parse(time.RFC3339, expiresAt)
		if time.Now().After(expTime) {
			http.Error(w, "This invitation token has expired", http.StatusForbidden)
			return
		}

		// 3. Burn the Token (Mark as used)
		_, err = db.Exec("UPDATE invitations SET is_used = 1 WHERE token = ?", req.Token)
		if err != nil {
			http.Error(w, "Failed to update token", http.StatusInternalServerError)
			return
		}

		// 4. Place the Client in the "Waiting Room" (status = 'pending')
		// Notice: We do NOT assign an IP address or inject them into WireGuard yet!
		clientID := uuid.New().String()
		_, err = db.Exec(`
			INSERT INTO clients (id, name, public_key, role_id, platform, device_id, status) 
			VALUES (?, ?, ?, ?, ?, ?, 'pending')`,
			clientID, req.Name, req.PublicKey, roleID, req.Platform, req.DeviceID,
		)

		if err != nil {
			http.Error(w, "Failed to register client or Public Key already exists", http.StatusInternalServerError)
			return
		}

		// 5. Respond to the Client App
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"status":    "success",
			"message":   "Successfully enrolled. Waiting for Administrator approval.",
			"client_id": clientID,
		})
	}
}
