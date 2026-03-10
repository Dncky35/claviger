package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"claviger-server/api"
	"claviger-server/network"
	"claviger-server/storage"
	"claviger-server/web"
)

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

	// Serve the static HTML UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(web.IndexHTML)
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
