package api

import (
	"claviger-server/internal/auth"
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
	Token     string `json:"token"`
	RoleID    string `json:"role_id"`
	ExpiresAt string `json:"expires_at"`
	IsUsed    bool   `json:"is_used"`
	CreatedAt string `json:"created_at"`
}

// HandleInvites manages creating, listing, and revoking enrollment tokens
func HandleInvites(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// --- GET: List all invitations (for the UI table) ---
		if r.Method == http.MethodGet {
			// Note: We only fetch tokens that haven't been used yet to keep the UI clean
			rows, err := db.Query("SELECT token, role_id, expires_at, is_used, created_at FROM invitations WHERE is_used = 0 ORDER BY created_at DESC")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			invites := []Invitation{}
			for rows.Next() {
				var inv Invitation
				rows.Scan(&inv.Token, &inv.RoleID, &inv.ExpiresAt, &inv.IsUsed, &inv.CreatedAt)
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

			res := Invitation{
				Token:     token,
				RoleID:    req.RoleID,
				ExpiresAt: expiresAt,
				IsUsed:    false,
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(res)
			return
		}

		// --- DELETE: Revoke an unused invitation ---
		if r.Method == http.MethodDelete {
			// Extract token from URL query (e.g., /api/invites?token=clav-xyz)
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
