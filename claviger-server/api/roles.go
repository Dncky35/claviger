package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Role defines the structure of a network access level
type Role struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AllowedPorts string `json:"allowed_ports"`
	CreatedAt    string `json:"created_at"`
}

// RoleReq is used for Creating, Updating, and Deleting
type RoleReq struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AllowedPorts string `json:"allowed_ports"`
}

// generateRoleID creates a URL-safe ID from the name (e.g., "Web Developer" -> "web-developer")
func generateRoleID(name string) string {
	id := strings.ToLower(strings.TrimSpace(name))
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	id = reg.ReplaceAllString(id, "-")
	return strings.Trim(id, "-")
}

// HandleRoles manages the CRUD operations for Zero Trust Roles
func HandleRoles(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// --- GET: List all Roles ---
		if r.Method == http.MethodGet {
			rows, err := db.Query("SELECT id, name, allowed_ports, created_at FROM roles ORDER BY created_at ASC")
			if err != nil {
				http.Error(w, `{"status":"error", "message":"Database error"}`, http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var roles []Role
			for rows.Next() {
				var role Role
				rows.Scan(&role.ID, &role.Name, &role.AllowedPorts, &role.CreatedAt)
				roles = append(roles, role)
			}
			json.NewEncoder(w).Encode(roles)
			return
		}

		// --- POST: Create a new Role ---
		if r.Method == http.MethodPost {
			var req RoleReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.AllowedPorts == "" {
				http.Error(w, `{"status":"error", "message":"Invalid request. Name and Allowed Ports are required."}`, http.StatusBadRequest)
				return
			}

			roleID := generateRoleID(req.Name)

			// Attempt to insert. If the ID already exists, SQLite will throw an error.
			_, err := db.Exec("INSERT INTO roles (id, name, allowed_ports) VALUES (?, ?, ?)", roleID, req.Name, req.AllowedPorts)
			if err != nil {
				http.Error(w, `{"status":"error", "message":"A role with this name already exists."}`, http.StatusConflict)
				return
			}

			json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Role created successfully", "id": roleID})
			return
		}

		// --- PUT: Update an existing Role ---
		if r.Method == http.MethodPut {
			var req RoleReq
			json.NewDecoder(r.Body).Decode(&req)

			// Guard: Prevent locking out the admin
			if req.ID == "admin" && req.AllowedPorts != "ALL" {
				http.Error(w, `{"status":"error", "message":"The admin role must always have 'ALL' ports allowed."}`, http.StatusForbidden)
				return
			}

			_, err := db.Exec("UPDATE roles SET name = ?, allowed_ports = ? WHERE id = ?", req.Name, req.AllowedPorts, req.ID)
			if err != nil {
				http.Error(w, `{"status":"error", "message":"Failed to update role"}`, http.StatusInternalServerError)
				return
			}

			json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Role updated successfully"})
			return
		}

		// --- DELETE: Remove a Role ---
		if r.Method == http.MethodDelete {
			var req RoleReq
			json.NewDecoder(r.Body).Decode(&req)

			// Guard 1: Protect System Defaults
			if req.ID == "admin" || req.ID == "standard" {
				http.Error(w, `{"status":"error", "message":"System default roles cannot be deleted."}`, http.StatusForbidden)
				return
			}

			// Guard 2: Dependency Check (Are clients or invites using this role?)
			var activeUsers, activeInvites int
			db.QueryRow("SELECT count(*) FROM clients WHERE role_id = ?", req.ID).Scan(&activeUsers)
			db.QueryRow("SELECT count(*) FROM invitations WHERE role_id = ?", req.ID).Scan(&activeInvites)

			if activeUsers > 0 || activeInvites > 0 {
				errMsg := fmt.Sprintf("Cannot delete role. It is currently assigned to %d users and %d pending invites.", activeUsers, activeInvites)
				http.Error(w, fmt.Sprintf(`{"status":"error", "message":"%s"}`, errMsg), http.StatusConflict)
				return
			}

			_, err := db.Exec("DELETE FROM roles WHERE id = ?", req.ID)
			if err != nil {
				http.Error(w, `{"status":"error", "message":"Failed to delete role"}`, http.StatusInternalServerError)
				return
			}

			json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Role deleted successfully"})
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
