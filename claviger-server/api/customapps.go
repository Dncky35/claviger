package api

import (
	"claviger-server/internal/apps"
	"claviger-server/internal/types"
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
		var payload types.AppManifest // 🎯 USE THE UNIFIED TYPE

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		// 0. Auto-Correct the YAML!
		// 🎯 Note: Ensure this function in 'apps' is updated to accept types.AppManifest
		correctedYAML, err := apps.AutoCorrectZeroTrustYAML(payload)
		if err == nil {
			payload.ComposeYAML = correctedYAML
		}

		// 1. Run the Zero Trust Validation
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

		// 2. Generate Safe ID if missing (or if JSON sent "id" instead of our new "app_id")
		if payload.ID == "" {
			safeName := strings.ToLower(payload.Name)
			safeName = strings.ReplaceAll(safeName, " ", "_")
			safeName = strings.ReplaceAll(safeName, "-", "_")
			payload.ID = "custom_" + safeName
		}

		// 3. Save to Database (We explicitly DO NOT save Category, CatalogType, etc., because custom_apps doesn't have those columns)
		stmt, err := db.Prepare(`
			INSERT INTO custom_apps (
				id, name, description, icon, needs_dynamic_port, has_custom_setup, setup_port, compose_yaml
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			http.Error(w, "Database preparation error", http.StatusInternalServerError)
			return
		}
		defer stmt.Close()

		_, err = stmt.Exec(
			payload.ID,
			payload.Name,
			payload.Description,
			payload.Icon,
			payload.NeedsDynamicPort,
			payload.HasCustomSetup,
			payload.SetupPort,
			payload.ComposeYAML,
		)
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
		// 1. Extract appID
		// 🎯 FIX: Assuming frontend calls /api/apps/custom?id=... (Change back to "name" if your frontend strictly requires it)
		appID := r.URL.Query().Get("id")

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
			// A. Tear down Docker containers
			cmd := exec.Command("docker", "compose", "down", "-v")
			cmd.Dir = appDir
			if output, err := cmd.CombinedOutput(); err != nil {
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
