package api

import (
	"encoding/json"
	"net/http"

	"claviger-server/internal/docker"
)

// HandleContainers returns the list of Docker containers, or tells the UI Docker isn't installed
func HandleContainers(engine *docker.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// SCENARIO 1: Docker is not installed or crashed
		if engine == nil || engine.Client == nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"docker_installed": false,
				"containers":       []docker.ContainerInfo{},
			})
			return
		}

		// SCENARIO 2: Docker is running perfectly
		containers, err := engine.ListContainers(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"docker_installed": true,
				"error":            err.Error(),
				"containers":       []docker.ContainerInfo{},
			})
			return
		}

		// Success! Send the clean list to the frontend
		json.NewEncoder(w).Encode(map[string]interface{}{
			"docker_installed": true,
			"containers":       containers,
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
