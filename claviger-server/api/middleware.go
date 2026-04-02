package api

import (
	"database/sql"
	"log"
	"net"
	"net/http"
)

// HubAccessMiddleware acts as the Bouncer for the Web Hub
func HubAccessMiddleware(db *sql.DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Extract the IP Address (ignoring the port number)
		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "Invalid Request", http.StatusBadRequest)
			return
		}

		// 2. Failsafe: Always allow the server to talk to itself
		if clientIP == "127.0.0.1" || clientIP == "10.8.0.1" {
			next.ServeHTTP(w, r)
			return
		}

		// 3. Check the database: Does this IP own an active session with Hub privileges?
		var allowHub int
		query := `
			SELECT r.allow_hub 
			FROM clients c
			JOIN roles r ON c.role_id = r.id
			WHERE c.ip_address = ? AND c.status = 'active'
		`

		err = db.QueryRow(query, clientIP).Scan(&allowHub)

		if err == sql.ErrNoRows {
			log.Printf("🔒 Blocked: Unknown or Inactive IP attempted Hub access (%s)", clientIP)
			http.Error(w, "403 Forbidden - Unauthorized Device", http.StatusForbidden)
			return
		} else if err != nil {
			log.Printf("⚠️ Database error checking middleware: %v", err)
			http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
			return
		}

		// 4. The Final Check
		if allowHub == 0 {
			log.Printf("🔒 Blocked: Standard User attempted Hub access (%s)", clientIP)
			http.Error(w, "403 Forbidden - Your role does not have administrative Hub access.", http.StatusForbidden)
			return
		}

		// 5. Passed! Send them to the requested page/API.
		next.ServeHTTP(w, r)
	}
}
