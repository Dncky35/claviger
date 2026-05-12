package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"claviger-server/internal/apps"
	"claviger-server/internal/gateway"
	"claviger-server/internal/security"
	"claviger-server/storage"
)

type InstallReq struct {
	AppID string `json:"app_id"`
}

// --- STATE MANAGER ---
// activeInstalls tracks which apps are currently downloading/installing
var (
	installMutex   sync.Mutex
	activeInstalls = make(map[string]bool)
)

// HandleAppInstall processes requests to install apps safely
func HandleAppInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"status": "error", "message": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req InstallReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"status": "error", "message": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	// ---------------------------------------------------------
	// 1. CHECK THE LOCK
	// ---------------------------------------------------------
	installMutex.Lock()
	if activeInstalls[req.AppID] {
		installMutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict) // 409 Conflict
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("%s is already installing. Please wait.", req.AppID),
		})
		return
	}

	activeInstalls[req.AppID] = true
	installMutex.Unlock()

	defer func() {
		installMutex.Lock()
		delete(activeInstalls, req.AppID)
		installMutex.Unlock()
	}()

	// ---------------------------------------------------------
	// 2. ROUTE THE INSTALLATION
	// ---------------------------------------------------------
	var err error

	db := storage.InitDB()
	defer db.Close()

	switch req.AppID {
	case "fail2ban":
		err = security.InstallFail2Ban()
	case "npm":
		// 🎯 THE GATEWAY GUARDRAIL
		// Before we let the catalog install NPM, make sure Ports 80/443 are free!
		if portErr := gateway.CheckPorts(); portErr != nil {
			installMutex.Lock()
			delete(activeInstalls, req.AppID)
			installMutex.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict) // 409 Conflict
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": portErr.Error()})
			return
		}
		err = apps.Install(db, req.AppID)
	default:
		err = apps.Install(db, req.AppID)
	}

	// ---------------------------------------------------------
	// 3. HANDLE RESULTS
	// ---------------------------------------------------------
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// HandleAppUninstall processes requests to safely destroy apps
func HandleAppUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"status": "error", "message": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		AppID string `json:"app_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"status": "error", "message": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	// 1. Mutex Lock (Reusing the installMutex to prevent Install/Uninstall conflicts)
	installMutex.Lock()
	if activeInstalls[req.AppID] {
		installMutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("%s is currently busy. Please wait.", req.AppID),
		})
		return
	}
	activeInstalls[req.AppID] = true
	installMutex.Unlock()

	defer func() {
		installMutex.Lock()
		delete(activeInstalls, req.AppID)
		installMutex.Unlock()
	}()

	// 2. Route the Uninstall
	var err error
	if req.AppID == "fail2ban" {
		// err = security.UninstallFail2Ban()
	} else {
		err = apps.Uninstall(req.AppID)
	}

	// 3. Handle Results
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
