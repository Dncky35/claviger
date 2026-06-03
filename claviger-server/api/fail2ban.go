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

func HandleFail2BanStatus(w http.ResponseWriter, r *http.Request) {
	installed := security.IsFail2BanInstalled()
	running := false

	// NEW: Prepare stats object
	var stats security.Fail2BanStats

	if installed {
		running = security.IsFail2BanRunning()
		if running {
			stats = security.GetFail2BanStats() // 🎯 Fetch live stats!
		}
	}

	response := map[string]interface{}{
		"installed": installed,
		"running":   running,
		"stats":     stats, // 🎯 Append stats to response
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// 2. ADD the new Unban Endpoint
func HandleFail2BanUnban(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"status": "error"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"status": "error"}`, http.StatusBadRequest)
		return
	}

	if err := security.UnbanIP(req.IP); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
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
