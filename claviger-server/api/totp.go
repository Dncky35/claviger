package api

import (
	"claviger-server/internal/security"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

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

func HandleGenerateTOTPHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req GenerateMfaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// 1. Get the client's name for the Authenticator app label
		var clientName string
		err := db.QueryRow("SELECT name FROM clients WHERE id = ?", req.ClientID).Scan(&clientName)
		if err == sql.ErrNoRows {
			http.Error(w, "Client not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		// 2. Generate the TOTP Secret
		secret, qrURL, err := security.GenerateTOTPSecret(clientName)
		if err != nil {
			http.Error(w, "Failed to generate secret", http.StatusInternalServerError)
			return
		}

		// 3. Upsert into client_mfa (Resetting if they abandoned a previous setup)
		mfaID := uuid.New().String()
		query := `
			INSERT INTO client_mfa (id, client_id, secret, is_verified) 
			VALUES (?, ?, ?, 0)
			ON CONFLICT(client_id) DO UPDATE SET 
				secret = excluded.secret, 
				is_verified = 0,
				recovery_keys = NULL;
		`
		// Note: To use ON CONFLICT(client_id), ensure client_id has a UNIQUE constraint in the client_mfa table.

		if _, err := db.Exec(query, mfaID, req.ClientID, secret); err != nil {
			log.Printf("⚠️ DB Error inserting MFA secret: %v", err)
			http.Error(w, "Failed to store secret", http.StatusInternalServerError)
			return
		}

		// 4. Return the setup data to the frontend
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GenerateMfaResponse{
			Secret: secret,
			QRUrl:  qrURL,
		})
	}
}

func HandleVerifyTOTPSetupHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req VerifySetupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// 1. Fetch the unverified secret from the DB
		var secret string
		var isVerified bool
		err := db.QueryRow("SELECT secret, is_verified FROM client_mfa WHERE client_id = ?", req.ClientID).
			Scan(&secret, &isVerified)

		if err == sql.ErrNoRows {
			http.Error(w, "No MFA setup in progress", http.StatusBadRequest)
			return
		}
		if isVerified {
			http.Error(w, "MFA is already verified and active", http.StatusConflict)
			return
		}

		// 2. Validate the 6-digit passcode
		isValid := security.ValidateTOTPCode(req.Passcode, secret)
		if !isValid {
			http.Error(w, "Invalid authenticator code", http.StatusUnauthorized)
			return
		}

		// 3. Setup successful! Generate and hash recovery keys
		plainKeys, err := security.GenerateRecoveryKeys()
		if err != nil {
			http.Error(w, "Error generating recovery keys", http.StatusInternalServerError)
			return
		}

		hashedKeysJSON, err := security.HashRecoveryKeys(plainKeys)
		if err != nil {
			http.Error(w, "Error securing recovery keys", http.StatusInternalServerError)
			return
		}

		// 4. Update the DB: Store hashed keys and set is_verified = 1
		updateQuery := `
			UPDATE client_mfa 
			SET is_verified = 1, recovery_keys = ? 
			WHERE client_id = ?
		`
		if _, err := db.Exec(updateQuery, hashedKeysJSON, req.ClientID); err != nil {
			log.Printf("⚠️ DB Error finalizing MFA setup: %v", err)
			http.Error(w, "Database error finalizing setup", http.StatusInternalServerError)
			return
		}

		// 5. Send the plaintext keys back to the frontend EXACTLY ONCE
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VerifySetupResponse{
			Message:      "MFA successfully enabled.",
			RecoveryKeys: plainKeys, // The frontend must force the user to download/copy these
		})
	}
}
