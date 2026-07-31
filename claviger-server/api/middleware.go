package api

import (
	"claviger-server/internal/security"
	"claviger-server/storage"
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type HubClaims struct {
	ClientID string `json:"client_id"`
	jwt.RegisteredClaims
}

func HubAccessBasicMiddleware(db *sql.DB, next http.HandlerFunc) http.HandlerFunc {
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

		// Check if the mfa is enabled for the server
		// endpoint := storage.GetConfig(db, "vpn_endpoint")
		// wgPort := storage.GetConfig(db, "wg_port")
		globalMfaStr := storage.GetConfig(db, "mfa_enabled")
		globalMfaEnabled := (globalMfaStr == "true")

		if globalMfaEnabled {

			var clientID string
			var isVerified bool

			// We use LEFT JOIN because they might not have an MFA record yet
			query := `
			SELECT c.id, COALESCE(m.is_verified, 0)
			FROM clients c
			LEFT JOIN client_mfa m ON c.id = m.client_id
			WHERE c.ip_address = ? AND c.status = 'active'
		`
			err = db.QueryRow(query, clientIP).Scan(&clientID, &isVerified)
			if err != nil {
				log.Printf("🔒 Blocked: Unknown IP in Step-Up check (%s)", clientIP)
				http.Error(w, "403 Forbidden - Unauthorized Device", http.StatusForbidden)
				return
			}

			// Look for the secure session cookie
			cookie, err := r.Cookie("claviger_hub_session")
			if err != nil {
				// No cookie present (or it expired). Trigger the frontend TOTP modal.
				sendStepUpChallenge(w)
				return
			}

			jwtSecret := security.EnsureJWTSecret(db)
			token, err := jwt.ParseWithClaims(cookie.Value, &HubClaims{}, func(t *jwt.Token) (interface{}, error) {
				return jwtSecret, nil
			})

			if err != nil || !token.Valid {
				// Token is expired, tampered with, or invalid
				sendStepUpChallenge(w)
				return
			}

			// Security Check: Does the cookie belong to the IP address using it?
			if claims, ok := token.Claims.(*HubClaims); ok {
				if claims.ClientID != clientID {
					log.Printf("⚠️ Session mismatch! Token ID %s doesn't match IP owner %s", claims.ClientID, clientID)
					sendStepUpChallenge(w)
					return
				}
			}

		}

		// 5. Passed! Send them to the requested page/API.
		next.ServeHTTP(w, r)
	}
}

// sendStepUpChallenge sends a specific 401 payload that tells the frontend
// to pause the current API request and show the "Enter Authenticator Code" modal.
func sendStepUpChallenge(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)

	payload := map[string]interface{}{
		"requires_stepup": true,
		"message":         "Elevated privileges required. Please enter your authenticator code.",
	}
	json.NewEncoder(w).Encode(payload)
}

// MasterAuthMiddleware protects Sub-server endpoints from unauthorized execution
func MasterAuthMiddleware(db *sql.DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. IP Security Check
		// Ensure the request is coming exactly from the Master's WireGuard IP (e.g., 10.8.0.1)
		clientIP := strings.Split(r.RemoteAddr, ":")[0]
		masterIP := storage.GetConfig(db, "master_vpn_ip") // Assume you save this during setup, or hardcode 10.8.0.1 for now

		if masterIP == "" {
			masterIP = "10.8.0.1" // Fallback to default Master IP
		}

		if clientIP != masterIP {
			http.Error(w, "Forbidden: Invalid origin IP", http.StatusForbidden)
			return
		}

		// 2. Fetch the expected secret key from the Sub-server's local database
		expectedKey := storage.GetConfig(db, "node_secret")
		if expectedKey == "" {
			http.Error(w, "Node not configured for remote access", http.StatusServiceUnavailable)
			return
		}

		// 3. Token Check
		// The Master will send the key in the Authorization header: "Bearer clvg_node_..."
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized: Missing or invalid token format", http.StatusUnauthorized)
			return
		}

		providedKey := strings.TrimPrefix(authHeader, "Bearer ")

		// 4. Final Comparison
		if providedKey != expectedKey {
			http.Error(w, "Unauthorized: Invalid node key", http.StatusUnauthorized)
			return
		}

		// Passed all checks! Allow the request to reach the actual handler
		next.ServeHTTP(w, r)
	}
}
