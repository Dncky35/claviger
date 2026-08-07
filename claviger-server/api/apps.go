package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	"claviger-server/internal/apps"
	"claviger-server/internal/gateway"
	"claviger-server/internal/security"
	"claviger-server/storage"
)

type InstallReq struct {
	AppID       string `json:"app_id"`
	IsCustom    bool   `json:"is_custom"` // New field to indicate if the app is custom
	CustomToken string `json:"custom_token"`
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
		// 1. Ensure directory and file exist
		os.MkdirAll("/opt/claviger/proxy", 0755)
		confPath := "/opt/claviger/proxy/cloudflare_ips.conf"

		// Create the file if it doesn't exist
		if _, err := os.Stat(confPath); os.IsNotExist(err) {
			os.WriteFile(confPath, []byte("# Empty initially. Run Lockdown to sync."), 0644)
		}

		// 2. 🎯 AUTOMATE OWNERSHIP:
		// NPM expects UID 1000. Set this so you never see 'Read-only file system' errors.
		// Note: This requires your Go app to run with sufficient privileges (sudo/root).
		err = os.Chown(confPath, 1000, 1000)
		if err != nil {
			fmt.Printf("Warning: Could not set ownership on %s: %v\n", confPath, err)
		}

		// 3. Port conflict check
		if portErr := gateway.CheckPorts(); portErr != nil {
			installMutex.Lock()
			delete(activeInstalls, req.AppID)
			installMutex.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": portErr.Error()})
			return
		}

		// 4. Safe to install
		err = apps.Install(db, req.AppID, req.IsCustom)

	case "cloudflared":
		// 1. Validate that a token was actually provided by the UI
		if req.CustomToken == "" {
			installMutex.Lock()
			delete(activeInstalls, req.AppID)
			installMutex.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "Cloudflare Tunnel token is required."})
			return
		}

		// 2. Persist the token to the database so it survives container restarts/reboots
		if dbErr := storage.SetConfig(db, "cloudflare_tunnel_token", req.CustomToken); dbErr != nil {
			fmt.Printf("⚠️ Warning: Failed to save cloudflare token to database: %v\n", dbErr)
		}

		// 3. Trigger installation (passing the custom token into your apps package)
		err = apps.Install(db, req.AppID, req.IsCustom)

	default:
		err = apps.Install(db, req.AppID, req.IsCustom)
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

	// =========================================================
	// 🎯 4. INCREMENT REVISION ON SUCCESS
	// =========================================================
	// We increment the revision because installing apps (like AdGuard)
	// changes the routing/DNS topology that connected clients need to know about!

	if req.AppID == "adguard" {
		newRevStr, revErr := storage.IncrementConfigRevision(db)
		if revErr != nil {
			// We don't fail the install, but we log the error
			fmt.Printf("⚠️ [App Install] %s installed, but failed to increment revision: %v\n", req.AppID, revErr)
		} else {
			fmt.Printf("🔄 [App Install] %s installed successfully. Revision bumped to %s\n", req.AppID, newRevStr)
		}
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
		err = security.UninstallFail2Ban()
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

	// =========================================================
	// 🎯 4. INCREMENT REVISION ON SUCCESS
	// =========================================================

	if req.AppID == "adguard" {
		db := storage.InitDB()
		defer db.Close()

		newRevStr, revErr := storage.IncrementConfigRevision(db)
		if revErr != nil {
			// We don't fail the install, but we log the error
			fmt.Printf("⚠️ [App Install] %s installed, but failed to increment revision: %v\n", req.AppID, revErr)
		} else {
			fmt.Printf("🔄 [App Install] %s installed successfully. Revision bumped to %s\n", req.AppID, newRevStr)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// LLMInstallReq maps to the JSON payload sent by our JS frontend
type LLMInstallReq struct {
	AppID   string `json:"app_id"`
	Version string `json:"version"`
}

// HandleLLMInstall processes requests to safely pull and deploy AI Engines and Frontends
func HandleLLMInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"status": "error", "message": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req LLMInstallReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"status": "error", "message": "Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	// 1. MUTEX LOCK (Prevent concurrent deployments of the same massive AI image)
	installMutex.Lock()
	if activeInstalls[req.AppID] {
		installMutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict) // 409 Conflict
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("%s is already deploying. Please wait, as AI images can be very large.", req.AppID),
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

	// 2. INITIALIZE DB
	db := storage.InitDB()
	defer db.Close()

	// 3. EXECUTE INSTALLATION
	// This calls the InstallLLM function we built earlier which includes the Hardware Profiler
	err := apps.InstallLLM(db, req.AppID, req.Version)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}

	// 4. SUCCESS RESPONSE
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// HandleLLMUninstall safely tears down an AI container with an optional data wipe
func HandleLLMUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"status": "error", "message": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		AppID   string `json:"app_id"`
		IsWiped bool   `json:"is_wiped"` // 🎯 New field mapping to JSON
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"status": "error", "message": "Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	// 1. MUTEX LOCK
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

	// 2. INITIALIZE DB
	db := storage.InitDB()
	defer db.Close()

	// 3. EXECUTE TEARDOWN (Passing the isWiped flag)
	err := apps.UninstallLLM(db, req.AppID, req.IsWiped)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
