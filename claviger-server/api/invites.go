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

// HandleInvites manages creating and listing enrollment tokens
func HandleInvites(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// --- GET: List all invitations (for the UI table) ---
		if r.Method == http.MethodGet {
			rows, err := db.Query("SELECT token, role_id, expires_at, is_used, created_at FROM invitations ORDER BY created_at DESC")
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
			// We try to decode, but if the body is empty, we just default the role
			json.NewDecoder(r.Body).Decode(&req)

			if req.RoleID == "" {
				req.RoleID = "standard"
			}

			// USE THE NEW AUTH ENGINE!
			token := auth.GenerateInviteToken()
			expiresAt := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
			createdAt := time.Now().Format(time.RFC3339)

			_, err := db.Exec(`
				INSERT INTO invitations (token, role_id, expires_at, is_used, created_at) 
				VALUES (?, ?, ?, 0, ?)`,
				token, req.RoleID, expiresAt, createdAt,
			)
			if err != nil {
				http.Error(w, "Failed to save invitation to database", http.StatusInternalServerError)
				return
			}

			// Return the generated token back to the UI
			res := Invitation{
				Token:     token,
				RoleID:    req.RoleID,
				ExpiresAt: expiresAt,
				IsUsed:    false,
				CreatedAt: createdAt,
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(res)
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
