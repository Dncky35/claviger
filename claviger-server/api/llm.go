package api

import (
	"claviger-server/internal/apps"
	"claviger-server/internal/hardware"
	"encoding/json"
	"fmt"
	"net/http"
)

// /api/hardware/ai
func HandleAIHardware(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodGet {
		// 1. Call the native Go profiler we already wrote
		profile, err := hardware.RunProfiler()

		if err != nil {
			http.Error(w, "Failed to profile hardware", http.StatusInternalServerError)
			return
		}

		// 2. Return the profile as JSON
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(profile)
	}

}

func HandleGetModels(w http.ResponseWriter, r *http.Request) {
	engineName := r.URL.Query().Get("engine")
	if engineName == "" {
		http.Error(w, `{"error": "engine parameter is required"}`, http.StatusBadRequest)
		return
	}

	adapter, err := apps.GetAdapter(engineName)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	models, err := adapter.ListModels()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to fetch models: %s"}`, err.Error()), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"engine": engineName,
		"models": models,
	})
}

// HandlePullModel routes: POST /api/llms/models/pull
func HandlePullModel(w http.ResponseWriter, r *http.Request) {
	var req apps.PullRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	adapter, err := apps.GetAdapter(req.Engine)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	// This blocks until the pull is complete (or triggered in background for vLLM)
	// We will look at WebSockets/Streaming later for the progress bar
	err = adapter.PullModel(req.ModelID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to pull model: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
