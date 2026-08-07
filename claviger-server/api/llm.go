package api

import (
	"claviger-server/internal/apps"
	"claviger-server/internal/hardware"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
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

// HandleGetModels routes: GET /api/llms/models?engine=ollama
func HandleGetModels(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		engineName := r.URL.Query().Get("engine")
		if engineName == "" {
			http.Error(w, `{"error": "engine parameter is required"}`, http.StatusBadRequest)
			return
		}

		adapter, err := apps.GetAdapter(engineName, db)
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
}

// HandlePullModel routes: POST /api/llms/models/pull
func HandlePullModel(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req apps.PullRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
			return
		}

		adapter, err := apps.GetAdapter(req.Engine, db)
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
}

// HandleDeleteModel routes: POST /api/llms/models/delete
func HandleDeleteModel(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Engine  string `json:"engine"`
			ModelID string `json:"model_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
			return
		}

		adapter, err := apps.GetAdapter(req.Engine, db)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		// Executes the engine-specific deletion (Ollama API, LocalAI file removal, etc.)
		err = adapter.DeleteModel(req.ModelID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to delete model: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}
}

// HandleToolkitInstall automates the NVIDIA Container Toolkit installation
func HandleToolkitInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"status": "error", "message": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// 1. The complete Debian/Ubuntu/Mint installation script
	// We run this without 'sudo' assuming the Claviger Go binary is running as root
	script := `
	set -e
	curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg --yes
	curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
	apt-get update
	apt-get install -y nvidia-container-toolkit
	nvidia-ctk runtime configure --runtime=docker
	systemctl restart docker
	`

	cmd := exec.Command("bash", "-c", script)
	output, err := cmd.CombinedOutput()

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "Installation failed: " + string(output),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
