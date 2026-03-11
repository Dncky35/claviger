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
	"time"

	"claviger-server/api"
	"claviger-server/network"
	"claviger-server/storage"
	"claviger-server/web"
	"text/template"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// syncWireGuardPeers reads all active clients from SQLite and hot-injects them into wg0.
// It also wipes any "ghost" peers from the kernel that shouldn't be there.
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

	// Apply the synchronized list directly to the Linux Kernel
	wg, err := wgctrl.New()
	if err != nil {
		log.Printf("⚠️ Failed to open WireGuard control: %v", err)
		return
	}
	defer wg.Close()

	err = wg.ConfigureDevice("wg0", wgtypes.Config{
		ReplacePeers: true, // Wipes the interface and replaces with our exact list
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

	if apiToken == "" || nodeID == "" {
		log.Fatal("❌ Error: Node is not registered. Please run 'sudo ./claviger-server setup' first.")
	}

	// If we pass the check, we are good to go!
	log.Println("✅ Node Identity loaded.")

	// 2. Synchronize the Kernel!
	syncWireGuardPeers(db)

	// ---------------------------------------------------------
	// 2. NETWORK BOOT (MUST HAPPEN BEFORE HEARTBEAT)
	// ---------------------------------------------------------
	if err := network.CheckAndOpenFirewall("51820"); err != nil {
		log.Printf("⚠️ Firewall check warning: %v\n", err)
	}

	if err := network.StartWireGuard(); err != nil {
		log.Fatalf("❌ Failed to start WireGuard: %v\n(If it is already running, run 'sudo wg-quick down wg0' to reset it)", err)
	}

	// ---------------------------------------------------------
	// 3. START HEARTBEAT ENGINE
	// ---------------------------------------------------------
	log.Println("💓 Starting Cloudrocean Heartbeat Engine...")
	go api.StartHeartbeatLoop(db, nodeID, apiToken, daemonVersion)

	// ---------------------------------------------------------
	// 4. SETUP THE WEB HUB
	// ---------------------------------------------------------
	mux := http.NewServeMux()

	// Wire up our clean API endpoints
	mux.HandleFunc("/api/status", api.HandleStatus(nodeID, apiToken != ""))
	mux.HandleFunc("/api/system", api.HandleSystemStats) // <-- The new hardware stats endpoint!
	mux.HandleFunc("/api/security", api.HandleSecurityStats)
	mux.HandleFunc("/api/security/action", api.HandleSecurityAction)
	mux.HandleFunc("/api/clients", api.HandleClients(db))
	mux.HandleFunc("/api/invites", api.HandleInvites(db))
	mux.HandleFunc("/api/enroll", api.HandleEnroll(db))
	mux.HandleFunc("/api/approve", api.HandleApprove(db))
	mux.HandleFunc("/api/revoke", api.HandleRevoke(db))

	// Parse the embedded HTML templates on boot
	tmpl, err := template.ParseFS(web.TemplatesFS, "index.html", "components/*.html")
	if err != nil {
		log.Fatalf("❌ Failed to parse HTML templates: %v", err)
	}

	// Serve the stitched UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		// Execute the template (this replaces w.Write(web.IndexHTML))
		err := tmpl.ExecuteTemplate(w, "index.html", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	port := "18080"
	server := &http.Server{
		Addr:    "0.0.0.0:" + port,
		Handler: mux,
	}

	go func() {
		fmt.Printf("\n🌐 Local Hub running securely.\n")
		fmt.Printf("👉 Access via VPN: http://10.8.0.1:%s\n", port)
		fmt.Printf("👉 Access via SSH: http://127.0.0.1:%s\n", port)
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

	// The code completely pauses here, waiting for the user to press Ctrl+C
	<-stopChan

	// --- SHUTDOWN SEQUENCE ---
	fmt.Println("\n\n🛑 Shutting down Claviger Edge Daemon...")

	// Safely bring down the VPN interface
	if err := network.StopWireGuard(); err != nil {
		log.Printf("⚠️ Error stopping WireGuard: %v\n", err)
	}

	// Safely shut down the local web server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("⚠️ Web server shutdown error: %v\n", err)
	}

	fmt.Println("✅ Daemon stopped cleanly. Network routes cleared. Goodbye!")
}
