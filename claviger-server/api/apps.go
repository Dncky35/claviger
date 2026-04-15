package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"claviger-server/internal/apps"
	"claviger-server/internal/security"
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
		// App is already installing! Unlock and reject the request immediately.
		installMutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict) // 409 Conflict
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("%s is already installing. Please wait.", req.AppID),
		})
		return
	}

	// Lock it!
	activeInstalls[req.AppID] = true
	installMutex.Unlock()

	// Ensure the lock is ALWAYS removed when this function finishes, even if it crashes
	defer func() {
		installMutex.Lock()
		delete(activeInstalls, req.AppID)
		installMutex.Unlock()
	}()
	// ---------------------------------------------------------

	// ---------------------------------------------------------
	// 2. ROUTE THE INSTALLATION
	// ---------------------------------------------------------
	var err error
	if req.AppID == "fail2ban" {
		err = security.InstallFail2Ban()
	} else {
		err = apps.Install(req.AppID)
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
