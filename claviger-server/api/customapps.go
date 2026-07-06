package api

import (
	"claviger-server/internal/apps"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var strictAppIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// HandleAddCustomApp handles POST /api/apps/custom
func HandleAddCustomApp(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload apps.CustomAppPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		// 0. Auto-Correct the YAML!
		// Automatically add the missing networks and labels
		correctedYAML, err := apps.AutoCorrectZeroTrustYAML(payload.ComposeYAML, payload.Name, payload.HasCustomSetup, payload.SetupPort, payload.NeedsDynamicPort)
		if err == nil {
			payload.ComposeYAML = correctedYAML // Replace with the hardened version
		}

		// 1. Run the Zero Trust Validation
		// (This will now pass the network and label checks automatically)
		validationErrors := apps.ValidateZeroTrustYAML(payload)

		if len(validationErrors) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "error",
				"errors": validationErrors,
			})
			return
		}

		// 2. Generate an ID (if not provided by frontend)
		if payload.ID == "" {
			payload.ID = "custom_" + strings.ReplaceAll(strings.ToLower(payload.Name), " ", "_")
		}

		// 3. Save to Database
		stmt, err := db.Prepare("INSERT INTO custom_apps (id, name, description, icon, needs_dynamic_port, has_custom_setup, setup_port, compose_yaml) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
		if err != nil {
			http.Error(w, "Database preparation error", http.StatusInternalServerError)
			return
		}
		defer stmt.Close()

		_, err = stmt.Exec(payload.ID, payload.Name, payload.Description, payload.Icon, payload.NeedsDynamicPort, payload.HasCustomSetup, payload.SetupPort, payload.ComposeYAML)
		if err != nil {
			http.Error(w, "Failed to save custom app", http.StatusInternalServerError)
			return
		}

		// 4. Return Success
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "Custom app securely auto-corrected, validated, and added to the catalog!",
		})
	}
}

func HandleRemoveCustomApp(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Extract appID (Assuming it's passed as a URL query parameter like ?id=custom_nginx)
		// If you use a router like Chi/Mux, adapt this to chi.URLParam(r, "id")
		appID := r.URL.Query().Get("name")

		if appID == "" {
			http.Error(w, "Missing app ID parameter", http.StatusBadRequest)
			return
		}

		// 2. Zero-Trust Security Check (Path Traversal Prevention)
		if !strictAppIDRegex.MatchString(appID) {
			http.Error(w, "Invalid app ID format", http.StatusBadRequest)
			return
		}

		// 3. Database Verification: Does this custom app exist?
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM custom_apps WHERE id = ?)", appID).Scan(&exists)
		if err != nil {
			http.Error(w, "Database transaction failed during verification", http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "Custom app not found in the catalog", http.StatusNotFound)
			return
		}

		// 4. Infrastructure Teardown (Only if actually installed)
		appDir := filepath.Join("/var/lib/claviger/apps", appID)

		if _, err := os.Stat(appDir); err == nil {
			// Directory exists, which means it's installed. Initiate Scorched Earth.

			// A. Tear down Docker containers, networks, and anonymous volumes
			cmd := exec.Command("docker", "compose", "down", "-v")
			cmd.Dir = appDir
			if output, err := cmd.CombinedOutput(); err != nil {
				// We log the output for debugging, but return a clean 500 to the client
				fmt.Printf("[Orchestrator] Docker teardown failed for %s: %s\n", appID, string(output))
				http.Error(w, "Failed to stop and remove Docker containers", http.StatusInternalServerError)
				return
			}

			// B. Wipe the configuration and data directory
			if err := os.RemoveAll(appDir); err != nil {
				fmt.Printf("[Orchestrator] Failed to remove app directory %s: %v\n", appDir, err)
				http.Error(w, "Containers removed, but failed to wipe data directory", http.StatusInternalServerError)
				return
			}
		}

		// 5. State Persistence Purge
		// We only reach this point if the physical teardown succeeded (or wasn't needed)
		_, err = db.Exec("DELETE FROM custom_apps WHERE id = ?", appID)
		if err != nil {
			fmt.Printf("[Database] Failed to delete custom app record %s: %v\n", appID, err)
			http.Error(w, "Infrastructure wiped, but failed to remove database record", http.StatusInternalServerError)
			return
		}

		// 6. Return Success Context
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": fmt.Sprintf("App %s has been completely uninstalled and removed from the catalog.", appID),
		})
	}
}
