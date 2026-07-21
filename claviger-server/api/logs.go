package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

// HandleGetLogs reads recent system logs using journalctl.
func HandleGetLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 1. Windows Local Dev Fallback
		if runtime.GOOS == "windows" {
			dummyLogs := []string{
				"Jul 21 12:00:00 claviger-dev [INFO] System log viewer initialized",
				"Jul 21 12:00:05 claviger-dev [WARN] Skipping journalctl on Windows environment",
				"Jul 21 12:00:10 claviger-dev [INFO] 🐕 Watchdog Engine active...",
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"lines": dummyLogs})
			return
		}

		// 2. Parse Query Parameters
		// Let the UI decide how many lines it wants (default to 100)
		lines := r.URL.Query().Get("lines")
		if lines == "" {
			lines = "100"
		}

		// Let the UI request a specific service (e.g., ?service=claviger or ?service=docker)
		service := r.URL.Query().Get("service")

		var cmd *exec.Cmd
		if service != "" {
			// Read specific service logs
			cmd = exec.Command("journalctl", "-u", service, "-n", lines, "--no-pager")
		} else {
			// Read general system logs (syslog equivalent)
			cmd = exec.Command("journalctl", "-n", lines, "--no-pager")
		}

		// 3. Execute and Parse
		out, err := cmd.Output()
		if err != nil {
			log.Printf("⚠️ Failed to read logs: %v", err)
			http.Error(w, `{"error": "Failed to read system logs. Is journalctl available?"}`, http.StatusInternalServerError)
			return
		}

		// Split the raw text block into an array of individual lines
		logLines := strings.Split(strings.TrimSpace(string(out)), "\n")

		// 4. Send to UI
		json.NewEncoder(w).Encode(map[string]interface{}{
			"lines": logLines,
		})
	}
}
