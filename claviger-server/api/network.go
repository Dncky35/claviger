package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"claviger-server/internal/firewall"
	"claviger-server/internal/security"
	"claviger-server/storage"
)

// InternetReq is the payload expected from the UI toggle
type InternetReq struct {
	Enable bool `json:"enable"`
}

// ProxyConfig represents the initial setup choices
type ProxyConfig struct {
	UseReverseProxy bool   `json:"use_reverse_proxy"`
	ProxyProvider   string `json:"proxy_provider"` // "cloudflare", "standard", or "none"
}

// HandleNetworkSettings manages global routing configurations
func HandleNetworkSettings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// --- GET: Check current status ---
		if r.Method == http.MethodGet {
			// Read from database. Default is "false" (isolated mode) if not set.
			statusStr := storage.GetConfig(db, "allow_global_internet")
			isEnabled := statusStr == "true"

			json.NewEncoder(w).Encode(map[string]bool{"internet_enabled": isEnabled})
			return
		}

		// --- POST: Toggle Internet Routing ---
		if r.Method == http.MethodPost {
			var req InternetReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"status":"error", "message":"Invalid request payload"}`, http.StatusBadRequest)
				return
			}

			// 1. Update the actual Linux Firewall
			var err error
			if req.Enable {
				err = firewall.EnableInternet()
			} else {
				err = firewall.DisableInternet()
			}

			if err != nil {
				http.Error(w, `{"status":"error", "message":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}

			// 2. Save state to SQLite so it survives reboots
			stateStr := "false"
			if req.Enable {
				stateStr = "true"
			}
			storage.SetConfig(db, "allow_global_internet", stateStr)

			json.NewEncoder(w).Encode(map[string]string{
				"status":  "success",
				"message": "Global routing updated successfully.",
			})
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func HandleGetProxyConfig(w http.ResponseWriter, r *http.Request) {

	db := storage.InitDB()
	defer db.Close()

	useReverseProxy := storage.GetConfig(db, "use_reverse_proxy")
	proxyProvider := storage.GetConfig(db, "proxy_provider")

	if useReverseProxy == "" {
		useReverseProxy = "false"
	}
	if proxyProvider == "" {
		proxyProvider = "none"
	}

	config := ProxyConfig{
		UseReverseProxy: useReverseProxy == "true",
		ProxyProvider:   proxyProvider,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

func HandleUpdateProxyConfig(w http.ResponseWriter, r *http.Request) {
	var req ProxyConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	db := storage.InitDB()
	defer db.Close()

	// Convert boolean back to string for your SQLite config table
	useProxyStr := "false"
	if req.UseReverseProxy {
		useProxyStr = "true"
	}

	if req.ProxyProvider != "cloudflare" && req.ProxyProvider != "standard" && req.ProxyProvider != "none" {
		http.Error(w, "Invalid proxy provider", http.StatusBadRequest)
		return
	}

	// 🎯 THE NEW LOGIC: Manage Nginx Real-IP Config based on selection
	confPath := "/opt/claviger/proxy/cloudflare_ips.conf"

	if useProxyStr == "false" {
		// Close all ports of 80/443

		// 1. Read the exact IPs that were applied from the database
		ipString := storage.GetConfig(db, "cloudflare_active_ips")
		var activeIPs []string
		if ipString != "" {
			activeIPs = strings.Split(ipString, ",")
		}

		// 2. Revert the firewall using ONLY those IPs
		if err := security.DisableHosting(activeIPs); err != nil {
			http.Error(w, "Failed to revert lockdown", http.StatusInternalServerError)
			return
		}

		// 3. Clear the database record now that they are removed
		storage.SetConfig(db, "cloudflare_active_ips", "")

	} else if useProxyStr == "true" && req.ProxyProvider == "cloudflare" {
		// Fetch the IPs and write the Nginx Real-IP config
		ips, err := security.FetchCloudflareIPs()
		if err != nil {
			log.Printf("❌ ERROR: FetchCloudflareIPs failed: %v\n", err)
			http.Error(w, "Failed to fetch Cloudflare IPs from network", http.StatusBadGateway)
			return
		}
		if err := security.GenerateNginxRealIPConfig(ips, confPath); err != nil {
			log.Printf("❌ ERROR: GenerateNginxRealIPConfig failed: %v\n", err)
			http.Error(w, "Failed to generate Nginx config", http.StatusInternalServerError)
			return
		}
		// (Optional: If NPM is already running, you could trigger a docker exec nginx reload here)
	} else {
		// If they switch to Standard or None, wipe the Real-IP config for safety
		os.MkdirAll("/opt/claviger/proxy", 0755)
		os.WriteFile(confPath, []byte("# Proxy is not set to Cloudflare. Real-IP unmasking disabled."), 0644)
	}

	storage.SetConfig(db, "use_reverse_proxy", useProxyStr)
	storage.SetConfig(db, "proxy_provider", req.ProxyProvider)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
