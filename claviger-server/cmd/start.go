package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"claviger-server/api"
	"claviger-server/internal/docker"
	"claviger-server/internal/firewall"
	"claviger-server/internal/scheduler"
	"claviger-server/network"
	"claviger-server/storage"
	"claviger-server/web"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// syncWireGuardPeers reads all active clients from SQLite and hot-injects them into wg0.
func syncWireGuardPeers(db *sql.DB) {
	log.Println("🔄 Synchronizing active peers to WireGuard interface...")

	rows, err := db.Query("SELECT public_key, ip_address FROM clients WHERE status = 'active'")
	if err != nil {
		log.Printf("⚠️ Failed to query active clients: %v", err)
		return
	}
	defer rows.Close()

	var peers []wgtypes.PeerConfig

	for rows.Next() {
		var pubKeyStr, ipStr string
		if err := rows.Scan(&pubKeyStr, &ipStr); err != nil {
			continue
		}

		pubKey, err := wgtypes.ParseKey(pubKeyStr)
		if err != nil {
			log.Printf("⚠️ Invalid public key for IP %s: %v", ipStr, err)
			continue
		}

		_, ipNet, err := net.ParseCIDR(ipStr + "/32")
		if err != nil {
			continue
		}

		peers = append(peers, wgtypes.PeerConfig{
			PublicKey:         pubKey,
			ReplaceAllowedIPs: true,
			AllowedIPs:        []net.IPNet{*ipNet},
		})
	}

	wg, err := wgctrl.New()
	if err != nil {
		log.Printf("⚠️ Failed to open WireGuard control: %v", err)
		return
	}
	defer wg.Close()

	err = wg.ConfigureDevice("wg0", wgtypes.Config{
		ReplacePeers: true,
		Peers:        peers,
	})

	if err != nil {
		log.Printf("❌ Failed to sync peers to kernel: %v", err)
	} else {
		log.Printf("✅ Successfully synced %d active peers to wg0", len(peers))
	}
}

func RunStart() {
	fmt.Println("=== Starting Claviger Edge Daemon ===")

	// 1. Initialize DB to read local config
	db := storage.InitDB()
	defer db.Close()

	// --- THE FIX: Ensure Setup Was Completed ---
	if storage.GetConfig(db, "node_id") == "" {
		log.Fatal("❌ Node is not configured! Please run 'sudo claviger-server setup' first.")
	}

	// --- Read Dynamic Ports from DB ---
	wgPort := storage.GetConfig(db, "wg_port")
	hubPort := storage.GetConfig(db, "hub_port")
	hubIP := storage.GetConfig(db, "hub_ip")

	if wgPort == "" {
		wgPort = "51820"
	}
	if hubPort == "" {
		hubPort = "18080"
	}
	if hubIP == "" {
		hubIP = "10.8.0.1"
	}

	// ---------------------------------------------------------
	// 2. NETWORK BOOT (DYNAMIC PORTS)
	// ---------------------------------------------------------
	if err := network.CheckAndOpenFirewall(wgPort); err != nil {
		log.Printf("⚠️ Firewall check warning: %v\n", err)
	}

	if err := network.StartWireGuard(); err != nil {

		// try to down wg0 and start again, in case it was a leftover state from an unclean shutdown
		log.Printf("⚠️ Initial WireGuard start failed: %v\nAttempting to recover by bringing down wg0 and retrying...", err)
		if err := network.StopWireGuard(); err != nil {
			log.Printf("⚠️ Failed to bring down wg0: %v", err)
		} else {
			log.Println("✅ Successfully brought down wg0. Retrying WireGuard start...")
			if err := network.StartWireGuard(); err != nil {
				log.Fatalf("❌ Recovery attempt failed. Please check the WireGuard configuration and logs: %v", err)
			} else {
				log.Println("✅ WireGuard started successfully on retry!")
			}
		}
	} else {
		log.Println("✅ WireGuard started successfully!")
	}

	// Restore Global Internet State
	if storage.GetConfig(db, "allow_global_internet") == "true" {
		log.Println("🔄 Restoring Global Internet Routing...")
		if err := firewall.EnableInternet(); err != nil {
			log.Printf("⚠️ Failed to restore internet routing: %v", err)
		}
	} else {
		log.Println("🔒 Server is in Isolated LAN mode (No Public Routing).")
	}

	syncWireGuardPeers(db)

	// ---------------------------------------------------------
	// 3. START HEARTBEAT ENGINE
	// ---------------------------------------------------------
	// log.Println("Starting Cloudrocean Heartbeat Engine...")
	// go api.StartHeartbeatLoop(db, nodeID, apiToken, daemonVersion)

	// ---------------------------------------------------------
	// 3.5 START DOCKER ORCHESTRATION ENGINE
	// ---------------------------------------------------------
	dockerEngine, err := docker.NewEngine()
	if err != nil {
		log.Printf("⚠️ Docker Engine Warning: %v\n(Containers tab will be disabled until Docker is installed)", err)
	} else {
		defer dockerEngine.Close()
	}

	// ---------------------------------------------------------
	// 4. SETUP THE WEB HUB & ERROR CHANNEL
	// ---------------------------------------------------------
	mux := http.NewServeMux()

	// --- UNPROTECTED ROUTES ---
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	})

	// --- PROTECTED ROUTES (Requires allow_hub = 1) ---
	mux.HandleFunc("/api/status", api.HubAccessMiddleware(db, api.HandleStatus()))
	mux.HandleFunc("/api/system", api.HubAccessMiddleware(db, api.HandleSystemStats))
	mux.HandleFunc("/api/security", api.HubAccessMiddleware(db, api.HandleSecurityStats))
	mux.HandleFunc("/api/security/action", api.HubAccessMiddleware(db, api.HandleSecurityAction))
	mux.HandleFunc("/api/clients", api.HubAccessMiddleware(db, api.HandleClients(db)))
	mux.HandleFunc("/api/enrollment/mobile", api.HubAccessMiddleware(db, api.HandleMobileEnrollment(db)))
	mux.HandleFunc("/api/revoke", api.HubAccessMiddleware(db, api.HandleRevoke(db)))
	mux.HandleFunc("/api/access/ssh", api.HubAccessMiddleware(db, api.HandleSSHKeys))
	mux.HandleFunc("/api/roles", api.HubAccessMiddleware(db, api.HandleRoles(db)))
	mux.HandleFunc("/api/network/internet", api.HubAccessMiddleware(db, api.HandleNetworkSettings(db)))
	mux.HandleFunc("/api/security/fail2ban/config", api.HubAccessMiddleware(db, api.HandleFail2BanConfig))

	// --- PROTECTED INSTALL/UNINSTALL ROUTES ---
	mux.HandleFunc("/api/apps/uninstall", api.HubAccessMiddleware(db, api.HandleAppUninstall))
	mux.HandleFunc("/api/apps/install", api.HubAccessMiddleware(db, api.HandleAppInstall))

	// --- PROTECTED GATEWAY ROUTES ---
	http.HandleFunc("/api/gateway/status", api.HubAccessMiddleware(db, api.HandleGatewayStatus()))

	// --- PROTECTED SETTINGS ROUTES ---
	mux.HandleFunc("/api/settings/endpoint", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			api.HubAccessMiddleware(db, api.HandleGetEndpoint(db))(w, r)
		} else {
			api.HubAccessMiddleware(db, api.HandleSaveEndpoint(db))(w, r)
		}
	})

	mux.HandleFunc("/api/system/tasks", api.HubAccessMiddleware(db, api.HandleGetTasks))

	// 2. The POST routes to run/toggle them
	mux.HandleFunc("/api/system/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/run") {
			api.HubAccessMiddleware(db, api.HandleRunTask)(w, r)
		} else if strings.HasSuffix(r.URL.Path, "/toggle") {
			api.HubAccessMiddleware(db, api.HandleToggleTask)(w, r)
		}
	})

	// --- PROTECTED DOCKER API ROUTES ---
	mux.HandleFunc("/api/containers", api.HubAccessMiddleware(db, api.HandleContainers(dockerEngine)))
	mux.HandleFunc("/api/containers/action", api.HubAccessMiddleware(db, api.HandleContainerAction(dockerEngine)))
	mux.HandleFunc("/api/containers/logs", api.HubAccessMiddleware(db, api.HandleContainerLogs(dockerEngine)))
	mux.HandleFunc("/api/containers/stats", api.HubAccessMiddleware(db, api.HandleContainerStats(dockerEngine)))

	// --- SERVE THE MAIN UI DASHBOARD ---
	tmpl, err := template.ParseFS(web.TemplatesFS, "index.html", "components/*.html")
	if err != nil {
		log.Fatalf("❌ Failed to parse HTML templates: %v", err)
	}

	// Wrap the UI in the same middleware so only authorized VPN IPs can see it
	mux.HandleFunc("/", api.HubAccessMiddleware(db, func(w http.ResponseWriter, r *http.Request) {
		// Strictly enforce the root path. Ignore /favicon.ico or random browser requests
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "index.html", nil); err != nil {
			http.Error(w, "Template Error: "+err.Error(), http.StatusInternalServerError)
		}
	}))

	db = storage.InitDB() // Or whatever you named your DB connection variable
	defer db.Close()

	scheduler.Start(db)

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", hubIP, hubPort), // Locked to VPN only
		Handler: mux,
	}

	// THE FIX: Create a channel to catch web server crashes
	errChan := make(chan error, 1)

	go func() {
		fmt.Printf("\n🌐 Local Hub running securely.\n")
		fmt.Printf("👉 Access strictly isolated to VPN: http://%s:%s\n", hubIP, hubPort)
		fmt.Println("\nPress Ctrl+C to safely shut down the daemon.")

		// THE FIX: Instead of log.Fatalf, send the error to the channel
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("Web Hub crashed: %v", err)
		}
	}()

	// ---------------------------------------------------------
	// 5. THE GRACEFUL SHUTDOWN TRAP
	// ---------------------------------------------------------
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	// THE FIX: Wait for EITHER a Ctrl+C OR a Web Server Crash
	select {
	case <-stopChan:
		fmt.Println("\n\n🛑 User requested shutdown. Stopping Claviger Edge Daemon...")
	case err := <-errChan:
		fmt.Printf("\n\n❌ FATAL ERROR: %v\n", err)
		fmt.Println("🛑 Initiating emergency cleanup...")
	}

	// --- CLEAN UP IPTABLES NAT ROUTING ---
	if storage.GetConfig(db, "allow_global_internet") == "true" {
		firewall.DisableInternet()
	}

	// --- CLEAN UP WIREGUARD ---
	if err := network.StopWireGuard(); err != nil {
		log.Printf("⚠️ Error stopping WireGuard: %v\n", err)
	}

	// --- CLEAN UP HTTP SERVER ---
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("⚠️ Web server shutdown error: %v\n", err)
	}

	fmt.Println("✅ Daemon stopped cleanly. Network routes cleared. Goodbye!")
}
