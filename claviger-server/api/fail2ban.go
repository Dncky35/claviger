package api

import (
	"encoding/json"
	"net/http"

	"claviger-server/internal/security"
)

// Fail2BanConfigReq defines the exact JSON structure we expect from the UI
type Fail2BanConfigReq struct {
	Port       int `json:"port"`
	MaxRetries int `json:"max_retries"`
	FindTime   int `json:"find_time"`
	BanTime    int `json:"ban_time"`
}

// HandleFail2BanConfig parses the UI request and applies the firewall rules
func HandleFail2BanConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"status": "error", "message": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req Fail2BanConfigReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"status": "error", "message": "Invalid request parameters"}`, http.StatusBadRequest)
		return
	}

	// 1. Basic validation so users don't break the firewall
	if req.Port <= 0 || req.Port > 65535 {
		http.Error(w, `{"status": "error", "message": "Invalid port number"}`, http.StatusBadRequest)
		return
	}
	if req.MaxRetries <= 0 {
		req.MaxRetries = 3 // Force a safe default
	}

	// 2. Call the native kernel generator we built earlier
	err := security.ConfigureFail2Ban(req.Port, req.MaxRetries, req.FindTime, req.BanTime)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}

	// 3. Return success to the UI!
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
