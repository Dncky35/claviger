package api

import (
	"claviger-server/internal/auth"
	"claviger-server/storage"
	"database/sql"
	"encoding/json"
	"net/http"
)

// PreviewReq catches the base64 token from the UI
type PreviewReq struct {
	Token string `json:"token"`
}

// ConfirmReq catches the final decision from the Admin UI
type ConfirmReq struct {
	RequestData auth.ConnectionRequest `json:"request_data"`
	RoleID      string                 `json:"role_id"`
}

// HandleRegisterPreview decodes the token so the UI can show the Admin who is asking to join
func HandleRegisterPreview() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			http.Error(w, `{"message": "Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req PreviewReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"message": "Invalid request payload"}`, http.StatusBadRequest)
			return
		}

		// Use our secure auth package to crack open the Base64 token
		connReq, err := auth.DecodeConnectionRequest(req.Token)
		if err != nil {
			http.Error(w, `{"message": "Invalid or corrupted Connection Request token"}`, http.StatusBadRequest)
			return
		}

		// Return the decoded data to the frontend so it can populate the UI!
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "success",
			"request": connReq,
		})
	}
}

// HandleRegisterConfirm processes the final approval, applies firewalls, and generates the Visa
func HandleRegisterConfirm(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			http.Error(w, `{"message": "Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req ConfirmReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"message": "Invalid request payload"}`, http.StatusBadRequest)
			return
		}

		// Fetch the Server's Public IP to put into the token
		serverIP := storage.GetConfig(db, "vpn_endpoint")
		if serverIP == "" {
			http.Error(w, `{"message": "Server public IP not configured. Run setup again."}`, http.StatusInternalServerError)
			return
		}

		// Execute the massive Engine function we built earlier!
		approvalData, err := auth.EnrollStandardUser(db, &req.RequestData, req.RoleID, serverIP)
		if err != nil {
			// If duplicate key or full subnet, it fails safely here
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		// Encode the Visa token for the Admin to copy
		finalToken, err := auth.EncodeConnectionApproval(approvalData)
		if err != nil {
			http.Error(w, `{"message": "Failed to encode approval token"}`, http.StatusInternalServerError)
			return
		}

		// Success! Send the Visa back to the UI.
		json.NewEncoder(w).Encode(map[string]string{
			"status":         "success",
			"approval_token": finalToken,
		})
	}
}

// HandleMobileEnrollment processes a request to generate a QR code for a new mobile device.
func HandleMobileEnrollment(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			http.Error(w, `{"message": "Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		// 1. PARSE INCOMING JSON (Includes Name, Role, and UseGlobalRouting)
		var req auth.MobileEnrollReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"message": "Invalid request payload"}`, http.StatusBadRequest)
			return
		}

		// 2. BASIC VALIDATION
		if req.Name == "" || req.Role == "" {
			http.Error(w, `{"message": "Device name and role are required"}`, http.StatusBadRequest)
			return
		}

		// 3. FETCH SERVER CONFIG
		// The auth package handles fallbacks if it's empty
		serverIP := storage.GetConfig(db, "vpn_endpoint")

		// 4. EXECUTE "BURN AFTER READING" FLOW
		// Generates keys, injects to wg0, saves ONLY pubkey to DB, and returns QR Base64
		resp, err := auth.EnrollMobileDevice(db, &req, serverIP)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		// (Optional) Increment the global revision config
		// so desktop sync managers realize the peer list has changed
		// _ = storage.IncrementConfigRevision(db)

		// 5. RETURN SECURE DATA TO UI
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"data":   resp, // Contains QRCodeBase64, PublicKey, and AssignedIP
		})
	}
}
