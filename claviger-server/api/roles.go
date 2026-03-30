package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Role defines the structure sent to the UI
type Role struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	AllowGlobalInternet bool   `json:"allow_global_internet"`
	AllowIntranet       bool   `json:"allow_intranet"`
	AllowHub            bool   `json:"allow_hub"`
	AllowedPorts        string `json:"allowed_ports"`
	AllowedIPs          string `json:"allowed_ips"`
	CreatedAt           string `json:"created_at"`
}

// generateRoleID creates a URL-safe ID from the name (e.g., "Web Dev" -> "web-dev")
func generateRoleID(name string) string {
	id := strings.ToLower(strings.TrimSpace(name))
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	id = reg.ReplaceAllString(id, "-")
	return strings.Trim(id, "-")
}

func HandleRoles(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// --- GET: List all Roles for the UI Table ---
		if r.Method == http.MethodGet {
			// Fetch all our new granular columns
			rows, err := db.Query(`
				SELECT id, name, allow_global_internet, allow_intranet, allow_hub, allowed_ports, allowed_ips, created_at 
				FROM roles ORDER BY created_at ASC
			`)
			if err != nil {
				http.Error(w, `{"status":"error", "message":"Database error"}`, http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var roles []Role
			for rows.Next() {
				var role Role
				rows.Scan(&role.ID, &role.Name, &role.AllowGlobalInternet, &role.AllowIntranet, &role.AllowHub, &role.AllowedPorts, &role.AllowedIPs, &role.CreatedAt)
				roles = append(roles, role)
			}
			json.NewEncoder(w).Encode(roles)
			return
		}

		// --- POST: Create a new Role ---
		if r.Method == http.MethodPost {
			var req Role
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
				http.Error(w, `{"status":"error", "message":"Invalid request. Role Name is required."}`, http.StatusBadRequest)
				return
			}

			// Clean up blank inputs to default to "ALL"
			if req.AllowedPorts == "" {
				req.AllowedPorts = "ALL"
			}
			if req.AllowedIPs == "" {
				req.AllowedIPs = "ALL"
			}

			roleID := generateRoleID(req.Name)

			_, err := db.Exec(`
				INSERT INTO roles (id, name, allow_global_internet, allow_intranet, allow_hub, allowed_ports, allowed_ips) 
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				roleID, req.Name, req.AllowGlobalInternet, req.AllowIntranet, req.AllowHub, req.AllowedPorts, req.AllowedIPs,
			)

			if err != nil {
				http.Error(w, `{"status":"error", "message":"A role with this name already exists."}`, http.StatusConflict)
				return
			}

			json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Role created successfully"})
			return
		}

		// --- PUT: Update an existing Role ---
		if r.Method == http.MethodPut {
			var req Role
			json.NewDecoder(r.Body).Decode(&req)

			// Clean up blank inputs
			if req.AllowedPorts == "" {
				req.AllowedPorts = "ALL"
			}
			if req.AllowedIPs == "" {
				req.AllowedIPs = "ALL"
			}

			// GUARD: The Admin role must never be accidentally locked out!
			if req.ID == "admin" {
				req.AllowGlobalInternet = true
				req.AllowIntranet = true
				req.AllowHub = true
				req.AllowedPorts = "ALL"
				req.AllowedIPs = "ALL"
			}

			_, err := db.Exec(`
				UPDATE roles SET 
					name = ?, allow_global_internet = ?, allow_intranet = ?, allow_hub = ?, allowed_ports = ?, allowed_ips = ? 
				WHERE id = ?`,
				req.Name, req.AllowGlobalInternet, req.AllowIntranet, req.AllowHub, req.AllowedPorts, req.AllowedIPs, req.ID,
			)
			if err != nil {
				http.Error(w, `{"status":"error", "message":"Failed to update role"}`, http.StatusInternalServerError)
				return
			}

			json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Role updated successfully"})
			return
		}

		// --- DELETE: Remove a Role ---
		if r.Method == http.MethodDelete {
			var req Role
			json.NewDecoder(r.Body).Decode(&req)

			if req.ID == "admin" || req.ID == "standard" {
				http.Error(w, `{"status":"error", "message":"System default roles cannot be deleted."}`, http.StatusForbidden)
				return
			}

			var activeUsers, activeInvites int
			db.QueryRow("SELECT count(*) FROM clients WHERE role_id = ?", req.ID).Scan(&activeUsers)
			db.QueryRow("SELECT count(*) FROM invitations WHERE role_id = ?", req.ID).Scan(&activeInvites)

			if activeUsers > 0 || activeInvites > 0 {
				errMsg := fmt.Sprintf("Cannot delete role. It is currently assigned to %d users and %d pending invites.", activeUsers, activeInvites)
				http.Error(w, fmt.Sprintf(`{"status":"error", "message":"%s"}`, errMsg), http.StatusConflict)
				return
			}

			db.Exec("DELETE FROM roles WHERE id = ?", req.ID)
			json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Role deleted successfully"})
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
