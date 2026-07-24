package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"time"
)

// POST /api/mfa/remove
func HandleRemoveMFA(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 1. Identify the device via Zero Trust WireGuard IP
		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			sendJSONError(w, "Invalid Request IP", http.StatusBadRequest)
			return
		}

		// Handle localhost testing override
		if clientIP == "127.0.0.1" || clientIP == "::1" {
			clientIP = "10.8.0.1"
		}

		// 2. Resolve Client ID from the active connection
		var clientID string
		err = db.QueryRow("SELECT id FROM clients WHERE ip_address = ? AND status = 'active'", clientIP).Scan(&clientID)
		if err == sql.ErrNoRows {
			sendJSONError(w, "Active client not found", http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("⚠️ DB error looking up client for MFA removal: %v", err)
			sendJSONError(w, "Database error", http.StatusInternalServerError)
			return
		}

		// 3. Delete the MFA record from the database
		_, err = db.Exec("DELETE FROM client_mfa WHERE client_id = ?", clientID)
		if err != nil {
			log.Printf("⚠️ DB error deleting MFA record: %v", err)
			sendJSONError(w, "Failed to remove MFA record", http.StatusInternalServerError)
			return
		}

		// 4. Actively destroy the session cookie
		// Setting the expiration to the Unix Epoch (1970) forces the browser to delete it immediately.
		isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

		http.SetCookie(w, &http.Cookie{
			Name:     "claviger_hub_session",
			Value:    "",              // Clear the token payload
			Expires:  time.Unix(0, 0), // Expire instantly
			Path:     "/",
			HttpOnly: true,
			Secure:   isSecure,
			SameSite: http.SameSiteStrictMode,
		})

		// 5. Respond with success
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "MFA removed successfully. Elevated session cleared.",
		})
	}
}
