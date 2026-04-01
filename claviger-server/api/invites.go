package api

import (
	"claviger-server/internal/auth"
	"claviger-server/storage"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

// InviteReq is the payload from the Hub UI when clicking "Generate Invite"
type InviteReq struct {
	RoleID string `json:"role_id"`
}

// Invitation represents a token in the database
type Invitation struct {
	Token      string `json:"token"`       // Hidden raw token for the DB
	SmartToken string `json:"smart_token"` // NEW: The Base64 string for the UI to display!
	RoleID     string `json:"role_id"`
	ExpiresAt  string `json:"expires_at"`
	IsUsed     bool   `json:"is_used"`
	CreatedAt  string `json:"created_at"`
}

// HandleInvites manages creating, listing, and revoking enrollment tokens
func HandleInvites(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Helper to grab connection details for the Smart Tokens
		getServerConfig := func() (string, string, string) {
			serverIP := storage.GetConfig(db, "public_ip")
			if serverIP == "" {
				serverIP = "127.0.0.1"
			}
			hubPort := storage.GetConfig(db, "hub_port")
			if hubPort == "" {
				hubPort = "18080"
			}

			// NEW: Grab the WireGuard Public Key from the database!
			serverKey := storage.GetConfig(db, "wg_public_key")
			return serverIP, hubPort, serverKey
		}

		// --- GET: List all invitations (for the UI table) ---
		if r.Method == http.MethodGet {
			rows, err := db.Query("SELECT token, role_id, expires_at, is_used, created_at FROM invitations WHERE is_used = 0 ORDER BY created_at DESC")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			serverIP, hubPort, serverKey := getServerConfig()
			invites := []Invitation{}

			for rows.Next() {
				var inv Invitation
				rows.Scan(&inv.Token, &inv.RoleID, &inv.ExpiresAt, &inv.IsUsed, &inv.CreatedAt)

				// Dynamically wrap the raw token into a Smart Token for the UI!
				inv.SmartToken, _ = auth.GenerateSmartToken(inv.Token, serverIP, hubPort, serverKey)

				invites = append(invites, inv)
			}
			json.NewEncoder(w).Encode(invites)
			return
		}

		// --- POST: Create a new invitation token ---
		if r.Method == http.MethodPost {
			var req InviteReq
			json.NewDecoder(r.Body).Decode(&req)

			if req.RoleID == "" {
				req.RoleID = "standard"
			}

			// --- SECURITY CHECK: Ensure the requested role actually exists ---
			var roleExists int
			db.QueryRow("SELECT 1 FROM roles WHERE id = ?", req.RoleID).Scan(&roleExists)
			if roleExists == 0 {
				http.Error(w, `{"status":"error", "message":"Invalid Role ID"}`, http.StatusBadRequest)
				return
			}

			// USE THE NEW AUTH ENGINE!
			token := auth.GenerateInviteToken()
			expiresAt := time.Now().Add(24 * time.Hour).Format(time.RFC3339)

			_, err := db.Exec(`
                INSERT INTO invitations (token, role_id, expires_at, is_used) 
                VALUES (?, ?, ?, 0)`,
				token, req.RoleID, expiresAt,
			)
			if err != nil {
				http.Error(w, `{"status":"error", "message":"Failed to save invitation"}`, http.StatusInternalServerError)
				return
			}

			serverIP, hubPort, serverKey := getServerConfig()
			smartToken, _ := auth.GenerateSmartToken(token, serverIP, hubPort, serverKey)

			res := Invitation{
				Token:      token,      // Keep raw for internal UI logic
				SmartToken: smartToken, // Send the Base64 wrapper for display
				RoleID:     req.RoleID,
				ExpiresAt:  expiresAt,
				IsUsed:     false,
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(res)
			return
		}

		// --- DELETE: Revoke an unused invitation ---
		if r.Method == http.MethodDelete {
			tokenToRevoke := r.URL.Query().Get("token")
			if tokenToRevoke == "" {
				http.Error(w, `{"status":"error", "message":"Token parameter is required"}`, http.StatusBadRequest)
				return
			}

			_, err := db.Exec("DELETE FROM invitations WHERE token = ?", tokenToRevoke)
			if err != nil {
				http.Error(w, `{"status":"error", "message":"Failed to delete invitation"}`, http.StatusInternalServerError)
				return
			}

			json.NewEncoder(w).Encode(map[string]string{
				"status":  "success",
				"message": "Invitation revoked successfully.",
			})
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
