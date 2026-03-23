package api

import (
	"encoding/json"
	"net/http"

	"claviger-server/internal/access"
)

type AddKeyReq struct {
	RawKey string `json:"raw_key"`
}

type RevokeKeyReq struct {
	Comment string `json:"comment"`
}

func HandleSSHKeys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// GET: List all SSH keys
	if r.Method == http.MethodGet {
		keys, err := access.ListKeys()
		if err != nil {
			http.Error(w, `{"status":"error", "message":"Failed to read keys"}`, http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(keys)
		return
	}

	// POST: Add or Revoke a key
	if r.Method == http.MethodPost {
		action := r.URL.Query().Get("action")

		if action == "add" {
			var req AddKeyReq
			json.NewDecoder(r.Body).Decode(&req)

			if err := access.AddKey(req.RawKey); err != nil {
				json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "SSH Key added successfully"})
			return
		}

		if action == "revoke" {
			var req RevokeKeyReq
			json.NewDecoder(r.Body).Decode(&req)

			if err := access.RevokeKey(req.Comment); err != nil {
				json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "SSH Key revoked successfully"})
			return
		}
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
