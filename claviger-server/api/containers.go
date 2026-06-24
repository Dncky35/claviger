package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"claviger-server/internal/apps"
	"claviger-server/internal/docker"
	"claviger-server/internal/security"
	"claviger-server/storage"
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

		// 🎯 OPEN DB CONNECTION TO FETCH DYNAMIC PORTS
		db := storage.InitDB()
		defer db.Close()

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
		containerMap := make(map[string]string) // Quick lookup map
		rawContainers := []docker.ContainerInfo{}
		dockerInstalled := false

		if engine != nil && engine.Client != nil {
			dockerInstalled = true
			if containers, err := engine.ListContainers(r.Context()); err == nil {
				rawContainers = containers
				for _, c := range containers {
					containerMap[c.Name] = c.State
				}
			}
		}

		// ---------------------------------------------------------
		// 3. MERGE CATALOG WITH DOCKER STATE & DB PORTS
		// ---------------------------------------------------------
		for id, manifest := range apps.Catalog {
			status := "not_installed"

			// 🎯 THE FIX: Handle multi-container apps like RustDesk
			if id == "rustdesk" {
				// Safely check for standard name or the Docker-API-slashed name
				stateHbbs := containerMap["rustdesk-hbbs"]
				if stateHbbs == "" {
					stateHbbs = containerMap["/rustdesk-hbbs"]
				}

				stateHbbr := containerMap["rustdesk-hbbr"]
				if stateHbbr == "" {
					stateHbbr = containerMap["/rustdesk-hbbr"]
				}

				if stateHbbs != "" || stateHbbr != "" {
					if stateHbbs == "running" && stateHbbr == "running" {
						status = "running"
					} else if stateHbbs == "running" || stateHbbr == "running" {
						status = "degraded"
					} else if stateHbbs != "" {
						status = stateHbbs
					} else {
						status = stateHbbr
					}
				}
			} else {
				// Standard single-container check (vaultwarden, gitea, etc.)
				if liveState, exists := containerMap[id]; exists {
					status = liveState
				}
			}

			setupComplete := true
			actionPort := 0
			actionText := "Open Dashboard ↗"

			// 🎯 RUSTDESK EXCEPTION: It does not have a web UI to open
			if id == "rustdesk" {
				actionText = "How to Use ↗"
				// Optional: You could make clicking this open a modal with their ID server IP.
			}

			// 🎯 FETCH PORT FROM DATABASE OR ASSIGN STATIC
			if manifest.NeedsDynamicPort {
				portStr := storage.GetConfig(db, fmt.Sprintf("app_%s_port", id))
				if portStr != "" {
					actionPort, _ = strconv.Atoi(portStr)
				}
			}

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
				Description:   manifest.Description,
				Icon:          manifest.Icon,
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
			"registry_apps":    appList,
			"raw_containers":   rawContainers,
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
