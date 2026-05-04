package api

import (
	"encoding/json"
	"net/http"
	"os"

	"claviger-server/internal/apps"
	"claviger-server/internal/docker"
	"claviger-server/internal/security"
)

// AppStatus is the smart, unified data packet we send to the Javascript UI
type AppStatus struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Category      string `json:"category"`
	Description   string `json:"description"` // ADD THIS
	Icon          string `json:"icon"`        // ADD THIS
	Status        string `json:"status"`
	SetupComplete bool   `json:"setup_complete"`
	ActionPort    int    `json:"action_port"`
	ActionText    string `json:"action_text"`
}

// HandleContainers merges Docker state, Native state, and the App Catalog
func HandleContainers(engine *docker.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var appList []AppStatus

		// ---------------------------------------------------------
		// 1. PROCESS NATIVE APPS (Fail2Ban)
		// ---------------------------------------------------------
		fail2banStatus := "not_installed"
		if security.IsFail2BanInstalled() {
			if security.IsFail2BanRunning() {
				fail2banStatus = "running"
			} else {
				fail2banStatus = "stopped"
			}
		}
		appList = append(appList, AppStatus{
			ID:            "fail2ban",
			Name:          "Fail2Ban",
			Category:      "system_core",
			Description:   "Fail2Ban is a daemon to prevent against brute-force attacks",
			Icon:          "🔐",
			Status:        fail2banStatus,
			SetupComplete: true, // Fail2Ban needs no UI setup wizard
			ActionPort:    0,
			ActionText:    "",
		})

		// ---------------------------------------------------------
		// 2. FETCH DOCKER STATE
		// ---------------------------------------------------------
		containerMap := make(map[string]string) // Quick lookup map: {"adguard": "running"}
		rawContainers := []docker.ContainerInfo{}
		dockerInstalled := false

		if engine != nil && engine.Client != nil {
			dockerInstalled = true
			if containers, err := engine.ListContainers(r.Context()); err == nil {
				rawContainers = containers
				// Map the live container states
				for _, c := range containers {
					containerMap[c.Name] = c.State
				}
			}
		}

		// ---------------------------------------------------------
		// 3. MERGE CATALOG WITH DOCKER STATE
		// ---------------------------------------------------------
		for id, manifest := range apps.Catalog {
			status := "not_installed"
			if liveState, exists := containerMap[id]; exists {
				status = liveState
			}

			// Default routing assumes setup is complete
			setupComplete := true
			actionPort := manifest.DashPort
			actionText := "Open Dashboard ↗"

			// Override routing if this specific app needs a setup wizard
			if manifest.HasCustomSetup && status != "not_installed" {
				if id == "adguard" {
					// Specific smart check for AdGuard
					if _, err := os.Stat("/var/lib/claviger/apps/adguard/conf/AdGuardHome.yaml"); err != nil {
						setupComplete = false
						actionPort = manifest.SetupPort
						actionText = "Finish Setup ↗"
					}
				}
			}

			appList = append(appList, AppStatus{
				ID:            id,
				Name:          manifest.Name,
				Category:      manifest.Category,
				Description:   manifest.Description, // ADD THIS
				Icon:          manifest.Icon,        // ADD THIS
				Status:        status,
				SetupComplete: setupComplete,
				ActionPort:    actionPort,
				ActionText:    actionText,
			})
		}

		// ---------------------------------------------------------
		// 4. SEND TO FRONTEND
		// ---------------------------------------------------------
		json.NewEncoder(w).Encode(map[string]interface{}{
			"docker_installed": dockerInstalled,
			"registry_apps":    appList,       // The clean list for the UI to build buttons
			"raw_containers":   rawContainers, // The raw list just in case we need deep Docker stats
		})
	}
}

type ContainerActionReq struct {
	ContainerID string `json:"container_id"`
	Action      string `json:"action"` // "start", "stop", "restart", "delete"
}

// HandleContainerAction processes POST requests to control Docker containers
func HandleContainerAction(engine *docker.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			http.Error(w, `{"status":"error", "message":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		if engine == nil || engine.Client == nil {
			http.Error(w, `{"status":"error", "message":"Docker is not installed or running"}`, http.StatusServiceUnavailable)
			return
		}

		var req ContainerActionReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"status":"error", "message":"Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if req.ContainerID == "" || req.Action == "" {
			http.Error(w, `{"status":"error", "message":"container_id and action are required"}`, http.StatusBadRequest)
			return
		}

		// Execute the command via the SDK
		err := engine.PerformAction(r.Context(), req.ContainerID, req.Action)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}
}

// HandleContainerLogs serves the last 100 lines of console output
func HandleContainerLogs(engine *docker.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			http.Error(w, `{"status":"error", "message":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		containerID := r.URL.Query().Get("id")
		if containerID == "" {
			http.Error(w, `{"status":"error", "message":"Missing container ID"}`, http.StatusBadRequest)
			return
		}

		if engine == nil || engine.Client == nil {
			http.Error(w, `{"status":"error", "message":"Docker is not available"}`, http.StatusServiceUnavailable)
			return
		}

		logs, err := engine.GetLogs(r.Context(), containerID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"status": "success",
			"logs":   logs,
		})
	}
}

// HandleContainerStats serves live CPU and Memory usage
func HandleContainerStats(engine *docker.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			http.Error(w, `{"status":"error", "message":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		containerID := r.URL.Query().Get("id")
		if containerID == "" {
			http.Error(w, `{"status":"error", "message":"Missing container ID"}`, http.StatusBadRequest)
			return
		}

		if engine == nil || engine.Client == nil {
			http.Error(w, `{"status":"error", "message":"Docker is not available"}`, http.StatusServiceUnavailable)
			return
		}

		stats, err := engine.GetStats(r.Context(), containerID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
			return
		}

		// Send the raw stats object to the UI
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"stats":  stats,
		})
	}
}
