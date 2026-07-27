package api

import (
	"claviger-server/internal/security"
	"claviger-server/storage"
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// JSON Payloads
type GenerateMfaRequest struct {
	ClientID string `json:"client_id"`
}

type GenerateMfaResponse struct {
	Secret string `json:"secret"` // For manual entry in the app
	QRUrl  string `json:"qr_url"` // For generating the QR code on the frontend
}

type VerifySetupRequest struct {
	ClientID string `json:"client_id"`
	Passcode string `json:"passcode"`
}

type VerifySetupResponse struct {
	Message      string   `json:"message"`
	RecoveryKeys []string `json:"recovery_keys"` // Plaintext keys shown ONLY once
}

type ValidateMfaRequest struct {
	Code string `json:"code"` // Can be 6-digit TOTP code or recovery key
}

type ValidateMfaResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func HandleGenerateTOTP(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Get the client's IP from the Zero Trust perimeter
		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			sendJSONError(w, "Invalid Request IP", http.StatusBadRequest)
			return
		}

		// Handle localhost testing override
		if clientIP == "127.0.0.1" || clientIP == "::1" {
			clientIP = "10.8.0.1" // Default test IP
		}

		// 2. Look up the Client ID and Name securely from the database using the IP
		var clientID, clientName string
		err = db.QueryRow("SELECT id, name FROM clients WHERE ip_address = ? AND status = 'active'", clientIP).Scan(&clientID, &clientName)

		if err == sql.ErrNoRows {
			sendJSONError(w, "Active client not found for this IP address.", http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("⚠️ DB error looking up client for MFA setup: %v", err)
			sendJSONError(w, "Database error", http.StatusInternalServerError)
			return
		}

		// 3. Generate the TOTP Secret
		secret, qrURL, err := security.GenerateTOTPSecret(clientName)
		if err != nil {
			sendJSONError(w, "Failed to generate secret", http.StatusInternalServerError)
			return
		}

		// 4. Clear out any previous/abandoned MFA setup for this client first
		_, err = db.Exec("DELETE FROM client_mfa WHERE client_id = ?", clientID)
		if err != nil {
			log.Printf("⚠️ DB Error clearing old MFA state: %v", err)
			sendJSONError(w, "Database error", http.StatusInternalServerError)
			return
		}

		// 5. Insert the fresh unverified secret
		mfaID := uuid.New().String()
		insertQuery := `
            INSERT INTO client_mfa (id, client_id, secret, is_verified) 
            VALUES (?, ?, ?, 0)
        `

		if _, err := db.Exec(insertQuery, mfaID, clientID, secret); err != nil {
			log.Printf("⚠️ DB Error inserting MFA secret: %v", err)
			sendJSONError(w, "Failed to store secret", http.StatusInternalServerError)
			return
		}

		// 6. Return the setup data to the frontend
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GenerateMfaResponse{
			Secret: secret,
			QRUrl:  qrURL,
		})
	}
}

func HandleVerifyTOTPSetup(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req VerifySetupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSONError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// 1. Identify the device via Zero Trust WireGuard IP
		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			sendJSONError(w, "Invalid Request IP", http.StatusBadRequest)
			return
		}

		if clientIP == "127.0.0.1" || clientIP == "::1" {
			clientIP = "10.8.0.1"
		}

		// 2. Resolve Client ID securely from IP
		var clientID string
		err = db.QueryRow("SELECT id FROM clients WHERE ip_address = ? AND status = 'active'", clientIP).Scan(&clientID)
		if err == sql.ErrNoRows {
			sendJSONError(w, "Active client not found", http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("⚠️ DB error looking up client for MFA verify: %v", err)
			sendJSONError(w, "Database error", http.StatusInternalServerError)
			return
		}

		// 3. Fetch the unverified secret from the DB using the resolved clientID
		var secret string
		var isVerified bool
		err = db.QueryRow("SELECT secret, is_verified FROM client_mfa WHERE client_id = ?", clientID).
			Scan(&secret, &isVerified)

		if err == sql.ErrNoRows {
			sendJSONError(w, "No MFA setup in progress", http.StatusBadRequest)
			return
		}
		if isVerified {
			sendJSONError(w, "MFA is already verified and active", http.StatusConflict)
			return
		}

		// 4. Validate the 6-digit passcode
		isValid := security.ValidateTOTPCode(req.Passcode, secret)
		if !isValid {
			sendJSONError(w, "Invalid authenticator code", http.StatusUnauthorized)
			return
		}

		// 5. Setup successful! Generate and hash recovery keys
		plainKeys, err := security.GenerateRecoveryKeys()
		if err != nil {
			sendJSONError(w, "Error generating recovery keys", http.StatusInternalServerError)
			return
		}

		hashedKeysJSON, err := security.HashRecoveryKeys(plainKeys)
		if err != nil {
			sendJSONError(w, "Error securing recovery keys", http.StatusInternalServerError)
			return
		}

		// 6. Update the DB: Store hashed keys and set is_verified = 1 using the resolved clientID
		updateQuery := `
            UPDATE client_mfa 
            SET is_verified = 1, recovery_keys = ? 
            WHERE client_id = ?
        `
		if _, err := db.Exec(updateQuery, hashedKeysJSON, clientID); err != nil {
			log.Printf("⚠️ DB Error finalizing MFA setup: %v", err)
			sendJSONError(w, "Database error finalizing setup", http.StatusInternalServerError)
			return
		}

		// 7. Send the plaintext keys back to the frontend EXACTLY ONCE
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VerifySetupResponse{
			Message:      "MFA successfully enabled.",
			RecoveryKeys: plainKeys,
		})
	}
}

// POST /api/mfa/validate
func HandleValidateTOTP(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 1. Extract request body
		var req ValidateMfaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
			sendJSONError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// 2. Identify the device via WireGuard IP
		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			sendJSONError(w, "Invalid Request", http.StatusBadRequest)
			return
		}

		// Handle localhost testing override if applicable
		if clientIP == "127.0.0.1" || clientIP == "::1" {
			clientIP = "10.8.0.1"
		}

		// 3. Resolve client ID from IP
		var clientID string
		err = db.QueryRow("SELECT id FROM clients WHERE ip_address = ? AND status = 'active'", clientIP).Scan(&clientID)
		if err == sql.ErrNoRows {
			sendJSONError(w, "Unauthorized Device", http.StatusForbidden)
			return
		} else if err != nil {
			log.Printf("⚠️ DB error during MFA login IP lookup: %v", err)
			sendJSONError(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// 4. Fetch user's verified MFA record
		var secret string
		var recoveryKeysJSON sql.NullString
		query := `
			SELECT secret, recovery_keys 
			FROM client_mfa 
			WHERE client_id = ? AND is_verified = 1
		`
		err = db.QueryRow(query, clientID).Scan(&secret, &recoveryKeysJSON)
		if err == sql.ErrNoRows {
			sendJSONError(w, "MFA is not enabled for this client", http.StatusBadRequest)
			return
		} else if err != nil {
			log.Printf("⚠️ DB error fetching MFA record: %v", err)
			sendJSONError(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// 5. Check passcode against TOTP algorithm
		isValid := security.ValidateTOTPCode(req.Code, secret)
		isRecovery := false

		// 6. If TOTP failed, check if it's a valid Recovery Key
		if !isValid && recoveryKeysJSON.Valid && recoveryKeysJSON.String != "" {
			if security.VerifyRecoveryKey(req.Code, recoveryKeysJSON.String) {
				isValid = true
				isRecovery = true
				log.Printf("🔑 Client %s authenticated using a Recovery Key!", clientID)
			}
		}

		if !isValid {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(ValidateMfaResponse{
				Success: false,
				Message: "Invalid authenticator code or recovery key",
			})
			return
		}

		// Fetch session duration string from config
		sessionDurationStr := storage.GetConfig(db, "session_duration")
		if sessionDurationStr == "" {
			sessionDurationStr = "15m"
		}

		// Convert the string to a time.Duration type
		duration, err := time.ParseDuration(sessionDurationStr)
		if err != nil {
			log.Printf("⚠️ Invalid session_duration format in DB ('%s'): %v. Defaulting to 15m.", sessionDurationStr, err)
			duration = 15 * time.Minute
		}

		// 7. Generate JWT Session Token using the parsed duration
		expirationTime := time.Now().Add(duration)
		claims := &HubClaims{
			ClientID: clientID,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(expirationTime),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				Issuer:    "claviger-gateway",
			},
		}

		jwtSecret := security.EnsureJWTSecret(db)

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, err := token.SignedString(jwtSecret)
		if err != nil {
			log.Printf("⚠️ Failed to sign JWT session token: %v", err)
			sendJSONError(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Check if the request came in over HTTPS
		isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

		// 8. Set HTTP-Only Session Cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "claviger_hub_session",
			Value:    tokenString,
			Expires:  expirationTime,
			Path:     "/",
			HttpOnly: true,                    // XSS Protection
			Secure:   isSecure,                // True for HTTPS, False for HTTP
			SameSite: http.SameSiteStrictMode, // CSRF Protection
		})

		// 9. Respond Success
		w.Header().Set("Content-Type", "application/json")
		msg := "Authentication successful"
		if isRecovery {
			msg = "Authenticated via Recovery Key. Please generate new backup codes if needed."
		}

		json.NewEncoder(w).Encode(ValidateMfaResponse{
			Success: true,
			Message: msg,
		})
	}
}

// GET /api/mfa/status
func HandleGetMFAStatus(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 1. Identify the device via WireGuard IP
		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			sendJSONError(w, "Invalid Request IP", http.StatusBadRequest)
			return
		}

		// Handle localhost testing override
		if clientIP == "127.0.0.1" || clientIP == "::1" {
			clientIP = "10.8.0.1"
		}

		// 2. Check the user's specific MFA configuration
		var isVerifiedInt int
		queryMFA := `
			SELECT COALESCE(m.is_verified, 0)
			FROM clients c
			LEFT JOIN client_mfa m ON c.id = m.client_id
			WHERE c.ip_address = ? AND c.status = 'active'
		`
		err = db.QueryRow(queryMFA, clientIP).Scan(&isVerifiedInt)
		if err != nil {
			if err == sql.ErrNoRows {
				sendJSONError(w, "Active client not found", http.StatusNotFound)
			} else {
				log.Printf("⚠️ DB error looking up MFA status: %v", err)
				sendJSONError(w, "Database error", http.StatusInternalServerError)
			}
			return
		}

		// 3. Check the Global MFA configuration
		// Note: ensure your storage package is imported if it isn't already
		globalMfaStr := storage.GetConfig(db, "mfa_enabled")
		globalEnabled := (globalMfaStr == "true")

		// 4. Return the state to the UI
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{
			"is_configured":  isVerifiedInt == 1,
			"global_enabled": globalEnabled,
		})
	}
}

// Helper function to ensure errors are always sent as JSON to the frontend
func sendJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"message": message})
}
