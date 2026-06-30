package cli

import (
	"claviger-client/internal/config"
	"fmt"
	"net"
	"strings"
	"time"
)

func HandleStatus(vault *config.ClientVault) {
	fmt.Println("=======================================")
	fmt.Println("🛡️  CLAVIGER ZERO TRUST GATEWAY STATUS")
	fmt.Println("=======================================")

	// 1. Read static configuration from Vault
	autoStart := "🔴 Disabled"
	if vault.AutoConnect {
		autoStart = "🟢 Enabled (Boots on startup)"
	}
	fmt.Printf("🔄 Auto-Start:   %s\n", autoStart)

	routing := "🌗 Split Tunnel (Internal only)"
	if vault.UseGlobalRouting {
		routing = "🌐 Global Route (All traffic)"
	}
	fmt.Printf("🔀 Routing Mode: %s\n", routing)

	if vault.ActiveProfileID != "" {
		if profile, ok := vault.Profiles[vault.ActiveProfileID]; ok {
			// fmt.Printf("🎯 Target Hub:   %s (%s)\n", profile.Name, profile.ServerEndpoint)
			fmt.Printf("Target Server: %s\n", profile.ServerEndpoint)
		} else {
			fmt.Printf("🎯 Target Hub:   ⚠️ Unknown Profile (%s)\n", vault.ActiveProfileID)
		}
	} else {
		fmt.Printf("🎯 Target Hub:   ⚠️ None Selected\n")
	}

	fmt.Println("---------------------------------------")

	// 2. Ask the Daemon for Live Engine State
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		fmt.Println("🔌 Daemon:       🔴 OFFLINE (Background service not running)")
		fmt.Println("🛜  VPN State:    ⚪ DISCONNECTED")
	} else {
		defer conn.Close()
		fmt.Println("🔌 Daemon:       🟢 ONLINE (Background service running)")

		// Whisper the status request
		conn.Write([]byte("STATUS"))
		buf := make([]byte, 128)
		n, _ := conn.Read(buf)

		// 1. Clean the network text
		daemonState := strings.TrimSpace(string(buf[:n]))

		// 🎯 THE FIX: Convert to uppercase so "Secured" matches "SECURED"
		upperState := strings.ToUpper(daemonState)

		// 2. Format the visual output based on engine state
		stateStr := "⚪ " + daemonState
		switch upperState {
		case "CONNECTED", "ONLINE", "SECURED":
			stateStr = "🟢 CONNECTED & SECURED"
		case "CONNECTING":
			stateStr = "🟡 CONNECTING..."
		case "DISCONNECTED":
			stateStr = "⚪ DISCONNECTED"
		}

		fmt.Printf("🛜  VPN State:    %s\n", stateStr)
	}
	fmt.Println("=======================================")
}
