package api

import (
	"claviger-server/internal/apps"
	"claviger-server/internal/types"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
)

type APIResponse struct {
	Status  string      `json:"status"` // "success" or "error"
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func jsonResponse(w http.ResponseWriter, status int, resp APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// HandleUpdateAppSetting receives the fetch request from the UI
func HandleUpdateAppSetting(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{
			Status:  "error",
			Message: "Method not allowed",
		})
		return
	}

	// 1. Decode the payload from the frontend
	var payload types.UpdatePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		jsonResponse(w, http.StatusBadRequest, APIResponse{
			Status:  "error",
			Message: "Invalid JSON payload",
		})
		return
	}

	// 2. Look up the action rules from the app's JSON schema in memory
	action, err := apps.GetActionFromSchema(payload.AppID, payload.FieldID)
	if err != nil {
		// FIXED: Send a 404 response so the frontend doesn't hang
		jsonResponse(w, http.StatusNotFound, APIResponse{
			Status:  "error",
			Message: fmt.Sprintf("Setting field not found: %v", err),
		})
		return
	}

	// 3. Define the directory where the app's docker-compose.yml and .env live
	appDir := filepath.Join("/var/lib/claviger/apps", payload.AppID)

	// 4. Fire the core execution engine (FIXED: Removed the duplicate call)
	err = apps.ExecuteAppAction(appDir, action, payload.NewValue)
	if err != nil {
		fmt.Printf("[ERROR] App orchestration failed: %v\n", err)
		jsonResponse(w, http.StatusInternalServerError, APIResponse{
			Status:  "error",
			Message: "Failed to apply settings: " + err.Error(),
		})
		return
	}

	// 5. Return success to the frontend
	jsonResponse(w, http.StatusOK, APIResponse{
		Status:  "success",
		Message: "Settings updated successfully",
	})
}

// HandleGetAppSchema serves the dynamic JSON schema to the frontend UI
func HandleGetAppSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{
			Status:  "error",
			Message: "Method not allowed",
		})
		return
	}

	// 1. Get the app_id from the URL query string (e.g., ?app_id=vaultwarden)
	appID := r.URL.Query().Get("app_id")
	if appID == "" {
		jsonResponse(w, http.StatusBadRequest, APIResponse{
			Status:  "error",
			Message: "Missing app_id parameter",
		})
		return
	}

	// 2. Fetch the hydrated schema from our engine
	schema, err := apps.GetHydratedSchema(appID)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, APIResponse{
			Status:  "error",
			Message: "Configuration schema not available for this app.",
		})
		return
	}

	// 3. Return the schema directly to the frontend
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schema)
}
