package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
)

// Define the expected JSON payload
type UpdateConfigRequest struct {
	Value string `json:"value"`
}

// POST /api/config/mfa_enabled
func HandleUpdateMFAConfig(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 1. Decode the JSON body
		var req UpdateConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSONError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// 2. Strict validation (only allow exact boolean strings)
		if req.Value != "true" && req.Value != "false" {
			sendJSONError(w, "Invalid value. Must be 'true' or 'false'", http.StatusBadRequest)
			return
		}

		// 3. Upsert the value into the config table.
		// INSERT OR REPLACE ensures that if the key already exists, it overwrites it.
		query := `INSERT OR REPLACE INTO config (key, value) VALUES ('mfa_enabled', ?)`

		if _, err := db.Exec(query, req.Value); err != nil {
			log.Printf("⚠️ DB error updating global MFA config: %v", err)
			sendJSONError(w, "Failed to update server configuration", http.StatusInternalServerError)
			return
		}

		// 4. Return success
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "Global MFA policy updated successfully",
		})
	}
}
