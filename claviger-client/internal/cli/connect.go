package cli

import (
	"bufio"
	"claviger-client/internal/config"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

func HandleConnect(vault *config.ClientVault, args []string, ctx context.Context) {
	// 1. Parse Arguments (If provided, override ActiveProfileID)
	targetID := ""
	if len(args) > 0 {
		targetID = strings.TrimSpace(args[0])
	} else {
		targetID = vault.ActiveProfileID
	}

	// 2. Early Validation (Safe to read from vault memory)
	if targetID == "" || len(vault.Profiles) == 0 {
		log.Fatalf("❌ No active server profile found. Please run 'claviger-client generate'.")
	}

	activeProfile, exists := vault.Profiles[targetID]
	if !exists {
		log.Fatalf("❌ Server profile '%s' not found. Run 'claviger-client list'.", targetID)
	}

	if activeProfile.Status != "active" {
		log.Fatalf("❌ Selected server is pending approval. Run 'claviger-client approve' first.")
	}

	// 3. Resolve Routing Mode
	routeMode := "split"
	if vault.UseGlobalRouting {
		routeMode = "global"
	}

	fmt.Println("⏳ Requesting Daemon to establish secure tunnel...")

	// 4. DELEGATE TO ROOT DAEMON (Strict Requirement Now)
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		// 🛑 NO FALLBACK! If the daemon isn't running, we fail immediately.
		log.Fatalf("❌ Daemon is not running!\n" +
			"You must start the background service first before connecting.\n" +
			"Run: 'sudo systemctl start claviger-server'")
	}
	defer conn.Close()

	// Send the command with the \n delimiter
	payload := fmt.Sprintf("CONNECT|%s|%s\n", targetID, routeMode)
	if _, err := conn.Write([]byte(payload)); err != nil {
		log.Fatalf("❌ Failed to send command to Daemon: %v", err)
	}

	// 5. Read response cleanly using bufio
	// Handshakes and routing table updates can take a few seconds, so we give it 10 seconds.
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	resp, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil && err != io.EOF {
		log.Fatalf("❌ Failed to read response from Daemon: %v", err)
	}

	resp = strings.TrimSpace(resp)

	if resp == "OK" {
		fmt.Println("✅ Tunnel Secured! The background daemon is managing the connection.")
	} else {
		log.Fatalf("❌ Daemon rejected the connection request: %s\n(Check daemon logs for details)", resp)
	}
}
