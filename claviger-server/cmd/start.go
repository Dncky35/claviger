package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"text/template"
	"time"

	"claviger-server/api"
	"claviger-server/internal/firewall"
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

	nodeID := storage.GetConfig(db, "node_id")
	apiToken := storage.GetConfig(db, "api_token")
	daemonVersion := "1.0.0"

	// --- NEW: Read Dynamic Ports from DB ---
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

	if apiToken == "" || nodeID == "" {
		log.Fatal("❌ Error: Node is not registered. Please run 'sudo claviger-server setup' first.")
	}

	log.Println("✅ Node Identity loaded.")

	// ---------------------------------------------------------
	// 2. NETWORK BOOT (DYNAMIC PORTS)
	// ---------------------------------------------------------
	if err := network.CheckAndOpenFirewall(wgPort); err != nil {
		log.Printf("⚠️ Firewall check warning: %v\n", err)
	}

	if err := network.StartWireGuard(); err != nil {
		log.Fatalf("❌ Failed to start WireGuard: %v\n(If it is already running, run 'sudo wg-quick down wg0')", err)
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
	log.Println("Starting Cloudrocean Heartbeat Engine...")
	go api.StartHeartbeatLoop(db, nodeID, apiToken, daemonVersion)

	// ---------------------------------------------------------
	// 4. SETUP THE WEB HUB
	// ---------------------------------------------------------
	mux := http.NewServeMux()

	mux.HandleFunc("/api/status", api.HandleStatus(nodeID, apiToken != ""))
	mux.HandleFunc("/api/system", api.HandleSystemStats)
	mux.HandleFunc("/api/security", api.HandleSecurityStats)
	mux.HandleFunc("/api/security/action", api.HandleSecurityAction)
	mux.HandleFunc("/api/clients", api.HandleClients(db))
	mux.HandleFunc("/api/invites", api.HandleInvites(db))
	mux.HandleFunc("/api/enroll", api.HandleEnroll(db))
	mux.HandleFunc("/api/approve", api.HandleApprove(db))
	mux.HandleFunc("/api/revoke", api.HandleRevoke(db))
	mux.HandleFunc("/api/access/ssh", api.HandleSSHKeys)
	mux.HandleFunc("/api/roles", api.HandleRoles(db))
	mux.HandleFunc("/api/network/internet", api.HandleNetworkSettings(db))

	tmpl, err := template.ParseFS(web.TemplatesFS, "index.html", "components/*.html")
	if err != nil {
		log.Fatalf("❌ Failed to parse HTML templates: %v", err)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		err := tmpl.ExecuteTemplate(w, "index.html", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	server := &http.Server{
		Addr:    "0.0.0.0:" + hubPort,
		Handler: mux,
	}

	go func() {
		fmt.Printf("\n🌐 Local Hub running securely.\n")
		fmt.Printf("👉 Access via VPN: http://%s:%s\n", hubIP, hubPort)
		fmt.Println("\nPress Ctrl+C to safely shut down the daemon.")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Web Hub crashed: %v", err)
		}
	}()

	// ---------------------------------------------------------
	// 5. THE GRACEFUL SHUTDOWN TRAP
	// ---------------------------------------------------------
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
	<-stopChan

	fmt.Println("\n\n🛑 Shutting down Claviger Edge Daemon...")

	// --- NEW: CLEAN UP IPTABLES NAT ROUTING ---
	if storage.GetConfig(db, "allow_global_internet") == "true" {
		firewall.DisableInternet()
	}

	if err := network.StopWireGuard(); err != nil {
		log.Printf("⚠️ Error stopping WireGuard: %v\n", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("⚠️ Web server shutdown error: %v\n", err)
	}

	fmt.Println("✅ Daemon stopped cleanly. Network routes cleared. Goodbye!")
}
