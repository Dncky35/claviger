package cli

import (
	"claviger-client/internal/config"
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

func HandleConnect(vault *config.ClientVault, args []string, ctx context.Context) {
	// 1. Parse Arguments (If provided, override ActiveProfileID)
	targetID := ""
	if len(args) > 0 {
		targetID = args[0]
	} else {
		targetID = vault.ActiveProfileID
	}

	// 2. Resolve Routing Mode
	routeMode := "split"
	if vault.UseGlobalRouting {
		routeMode = "global"
	}

	// 3. Validation
	if targetID == "" || len(vault.Profiles) == 0 {
		log.Fatalf("❌ No active server profile found. Please run 'claviger generate'.")
	}

	activeProfile, exists := vault.Profiles[targetID]
	if !exists {
		log.Fatalf("❌ Server profile '%s' not found. Run 'claviger list'.", targetID)
	}

	if activeProfile.Status != "active" {
		log.Fatalf("❌ Selected server is pending approval. Run 'claviger approve' first.")
	}

	// 4. Update Preferences
	vault.ActiveProfileID = targetID
	if err := config.Save(vault); err != nil {
		log.Printf("⚠️ Could not save preferences: %v", err)
	}

	// 5. DELEGATE TO ROOT DAEMON (Strict Requirement Now)
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		// 🛑 NO FALLBACK! If the daemon isn't running, we fail immediately.
		log.Fatalf("❌ Daemon is not running!\n" +
			"You must start the background service first before connecting.\n" +
			"Run: 'sudo systemctl start claviger' OR 'sudo claviger-client daemon'")
	}
	defer conn.Close()

	log.Println("📡 Whispering CONNECT command to root daemon...")
	payload := fmt.Sprintf("CONNECT|%s|%s", targetID, routeMode)
	conn.Write([]byte(payload))

	// Read response with buffer
	buf := make([]byte, 16)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := conn.Read(buf)

	response := strings.TrimSpace(string(buf[:n]))

	if response == "OK" {
		log.Println("✅ Tunnel Secured! The background daemon is managing the connection.")
		return
	}

	log.Fatalf("❌ Daemon rejected the connection request: %s", response)
}
