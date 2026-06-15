package cli

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"claviger-client/internal/auth"
	"claviger-client/internal/config"
	"claviger-client/internal/vpn"

	"github.com/google/uuid"
)

func PrintHelp() {
	fmt.Print(`
🛡️  Claviger Zero Trust Engine

Usage:
  claviger-client generate         - Generate a new Passport token to join a network
  claviger-client approve <token>  - Apply a Visa token provided by your Administrator
  claviger-client autoconnect      - Set enable/disable auto-connect feature
  claviger-client global           - Set enable/disable global routing feature
  claviger-client list             - Show all enrolled server profiles
  
  claviger-client remove <id>      - Delete a server profile from this device 
  claviger-client connect [id] 

  claviger-client disconnect       - Gracefully shut down the active VPN connection
  claviger-client status           - Provide current status and config of the client 
  claviger-client daemon           - Start the background VPN engine (Used by systemd)
`)
}

func HandleGenerate(vault *config.ClientVault) {
	fmt.Println("⏳ Generating cryptographic keys...")
	privKey, pubKey, err := vpn.GenerateKeys()
	if err != nil {
		log.Fatalf("❌ Failed to generate keys: %v", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Unknown-Desktop"
	}

	if vault.DeviceID == "" {
		vault.DeviceID = uuid.New().String()
	}

	newProfileID := uuid.New().String()
	newProfile := &config.ServerProfile{
		ID:         newProfileID,
		Name:       "Pending Server...",
		PrivateKey: privKey,
		PublicKey:  pubKey,
		Status:     "pending_approval",
	}

	if vault.Profiles == nil {
		vault.Profiles = make(map[string]*config.ServerProfile)
	}

	vault.Profiles[newProfileID] = newProfile
	vault.ActiveProfileID = newProfileID

	if err := config.Save(vault); err != nil {
		log.Fatalf("❌ Failed to save vault: %v", err)
	}

	token, err := auth.GenerateRequestToken(pubKey, hostname, runtime.GOOS, vault.DeviceID)
	if err != nil {
		log.Fatalf("❌ Failed to build request token: %v", err)
	}

	fmt.Println("\n✅ PASSPORT GENERATED SUCCESSFULLY")
	fmt.Println("Send this token to your Network Administrator:")
	fmt.Println("---------------------------------------------------")
	fmt.Println(token)
	fmt.Println("---------------------------------------------------")
}

func HandleApprove(vault *config.ClientVault, tokenString string) {
	fmt.Println("⏳ Decoding Server Visa...")
	approval, err := auth.DecodeApprovalToken(tokenString)
	if err != nil {
		log.Fatalf("❌ Invalid Visa token: %v", err)
	}

	profile, exists := vault.Profiles[vault.ActiveProfileID]
	if !exists || profile.Status != "pending_approval" {
		log.Fatalf("❌ No pending server request found. Run 'claviger generate' first.")
	}

	profile.AssignedIP = approval.AssignedIP
	profile.ServerKey = approval.ServerPubKey
	profile.ServerEndpoint = approval.ServerEndpoint
	profile.DNS = approval.DNS
	profile.BaseSubnet = approval.BaseSubnet
	profile.Status = "active"
	profile.HubPort = approval.HubPort

	serverIP := strings.Split(approval.ServerEndpoint, ":")[0]
	profile.Name = fmt.Sprintf("Claviger Hub (%s)", serverIP)

	if err := config.Save(vault); err != nil {
		log.Fatalf("❌ Failed to save vault: %v", err)
	}

	fmt.Printf("✅ Visa Accepted! You are now enrolled in: %s\n", profile.Name)
	fmt.Println("Run 'claviger connect' to establish the tunnel.")
}

// HandleAutostart toggles the background daemon's boot-time auto-connect feature
func HandleAutostart(vault *config.ClientVault, action string) {
	switch action {
	case "enable":
		vault.AutoConnect = true
		fmt.Println("🔄 Auto-Connect ENABLED.")
		fmt.Println("The background daemon will now automatically connect your active profile on system boot.")
	case "disable":
		vault.AutoConnect = false
		fmt.Println("🛑 Auto-Connect DISABLED.")
		fmt.Println("Claviger will wait for a manual connection command after reboot.")
	default:
		log.Fatalf("❌ Invalid action: '%s'. Usage: claviger-client autostart <enable|disable>", action)
	}

	// Save the preference to the system-wide vault
	if err := config.Save(vault); err != nil {
		// If they didn't run with sudo, it will fail here with a permission denied error
		log.Fatalf("❌ Failed to save preference (Did you forget 'sudo'?): %v", err)
	}
}

func HandleGlobalRouting(vault *config.ClientVault, action string) {
	switch action {
	case "enable":
		vault.UseGlobalRouting = true
		fmt.Println("🌐 Global Routing ENABLED.")

	case "disable":
		vault.UseGlobalRouting = false
		fmt.Println("🌗 Global Routing DISABLED.")
	default:
		log.Fatalf("❌ Invalid action: '%s'. Usage: claviger-client global <enable|disable>", action)
	}

	if err := config.Save(vault); err != nil {
		log.Fatalf("❌ Failed to save preference (Did you forget 'sudo'?): %v", err)
	}
}

func HandleList(vault *config.ClientVault) {
	fmt.Println("\n🌐 ENROLLED CLAVIGER SERVERS:")
	fmt.Println("---------------------------------------------------")
	if len(vault.Profiles) == 0 {
		fmt.Println("  No servers enrolled.")
		return
	}

	for id, profile := range vault.Profiles {
		activeMarker := "  "
		if id == vault.ActiveProfileID {
			activeMarker = "👉"
		}
		statusIcon := "🟢"
		if profile.Status != "active" {
			statusIcon = "🟡"
		}

		shortID := id
		if len(id) > 8 {
			shortID = id[:8]
		}

		fmt.Printf("%s [%s] %s %s | IP: %s\n", activeMarker, shortID, statusIcon, profile.Name, profile.AssignedIP)
		fmt.Printf("      Full ID: %s\n", id)
		fmt.Println("---------------------------------------------------")
	}
}

func HandleRemove(vault *config.ClientVault, profileID string) {
	if _, exists := vault.Profiles[profileID]; !exists {
		log.Fatalf("❌ Server profile '%s' not found.", profileID)
	}

	delete(vault.Profiles, profileID)
	if vault.ActiveProfileID == profileID {
		vault.ActiveProfileID = ""
	}

	if err := config.Save(vault); err != nil {
		log.Fatalf("❌ Failed to update vault: %v", err)
	}
	fmt.Println("✅ Server deleted successfully.")
}

// 🎯 NEW: Accepts arguments to parse Target ID and Routing Flags, then delegates to Daemon!
func HandleConnect(vault *config.ClientVault, args []string, disconnectChan chan bool) {
	targetID := ""
	routingMode := "split"
	if vault.UseGlobalRouting {
		routingMode = "global"
	}

	// 1. Scan the arguments for routing flags, autostart, or a specific server ID
	// for _, arg := range args {
	// 	switch arg {
	// 	case "--global":
	// 		vault.UseGlobalRouting = true
	// 		routingMode = "global"
	// 		fmt.Println("🌐 Mode: GLOBAL ROUTING (All traffic routed through VPN)")
	// 	case "--split":
	// 		vault.UseGlobalRouting = false
	// 		routingMode = "split"
	// 		fmt.Println("🌗 Mode: SPLIT TUNNEL (Only internal traffic routed through VPN)")
	// 	default:
	// 		targetID = arg // If it's not a flag, assume it's a Server ID
	// 	}
	// }

	// 2. Resolve Target ID
	if targetID == "" {
		targetID = vault.ActiveProfileID
	}

	if targetID == "" || len(vault.Profiles) == 0 {
		log.Fatalf("❌ No active server profile found. Please run 'claviger generate'.")
	}

	if _, exists := vault.Profiles[targetID]; !exists {
		log.Fatalf("❌ Server profile '%s' not found. Run 'claviger list'.", targetID)
	}

	activeProfile := vault.Profiles[targetID]
	if activeProfile.Status != "active" {
		log.Fatalf("❌ Selected server is pending approval. Run 'claviger approve' first.")
	}

	// 3. Save state preferences
	vault.ActiveProfileID = targetID
	if err := config.Save(vault); err != nil {
		log.Printf("⚠️ Could not save preferences: %v", err)
	}

	// 🎯 4. ATTEMPT TO DELEGATE TO ROOT DAEMON (Linux/Mac)
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err == nil {
		log.Println("📡 Whispering CONNECT command to root daemon...")
		payload := fmt.Sprintf("CONNECT|%s|%s", targetID, routingMode)
		conn.Write([]byte(payload))

		// Wait for acknowledgment
		ack := make([]byte, 2)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		conn.Read(ack)
		conn.Close()

		if string(ack) == "OK" {
			log.Println("✅ Tunnel Secured! The background daemon is managing the connection.")
			// We exit immediately here! The terminal doesn't need to stay open
			// because the background daemon is keeping the VPN alive.
			return
		} else {
			log.Fatalf("❌ Daemon rejected the connection request.")
		}
	}

	// 🎯 5. FALLBACK (Windows / Standalone Admin Mode)
	log.Println("⚠️ Daemon not found. Running engine directly in current terminal...")
	engine := vpn.NewEngine()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	connectErr := engine.Connect(vault, activeProfile, vault.UseGlobalRouting)
	if connectErr != nil {
		log.Fatalf("❌ Failed to connect: %v", connectErr)
	}

	log.Println("✅ Tunnel Secured! Traffic is flowing. Press Ctrl+C to disconnect safely.")

	// Block until EITHER an OS Signal OR a TCP Disconnect command arrives
	select {
	case <-sigChan:
		log.Println("⚠️ OS Shutdown Signal received!")
	case <-disconnectChan:
		log.Println("⚠️ Remote CLI Disconnect command received!")
	}

	fmt.Println()
	log.Println("Executing clean disconnect...")
	engine.Disconnect()
	log.Println("👋 Claviger Engine shut down gracefully. Network restored.")
	os.Exit(0)
}

func HandleDisconnect(vault *config.ClientVault) {
	fmt.Println("🛑 Sending disconnect signal to Claviger Engine...")

	// Dial the background process
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		fmt.Println("❌ Claviger is not currently running.")
		return
	}
	defer conn.Close()

	// Whisper the disconnect command
	conn.Write([]byte("DISCON"))

	// Read the response
	buf := make([]byte, 128)
	n, _ := conn.Read(buf)
	fmt.Printf("✅ %s\n", string(buf[:n]))

}

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
			fmt.Printf("🎯 Target Hub:   %s (%s)\n", profile.Name, profile.ServerEndpoint)
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
		daemonState := string(buf[:n])

		// Format the visual output based on engine state
		stateStr := "⚪ " + daemonState
		switch daemonState {
		case "Connected", "ONLINE":
			stateStr = "🟢 CONNECTED & SECURED"
		case "Connecting":
			stateStr = "🟡 CONNECTING..."
		case "Disconnected":
			stateStr = "⚪ DISCONNECTED"
		}

		fmt.Printf("🛜  VPN State:    %s\n", stateStr)
	}
	fmt.Println("=======================================")
}

// HandleUninstall safely shuts down the daemon, wipes the vault, and deletes the app.
func HandleUninstall() {
	// 1. MUST BE ROOT
	if os.Geteuid() != 0 {
		log.Fatalf("❌ Uninstallation requires Admin rights. Please run: sudo claviger-client uninstall")
	}

	// 2. CONFIRMATION PROMPT
	fmt.Printf("⚠️  WARNING: This will completely erase Claviger, including all VPN profiles, keys, and settings.\n")
	fmt.Printf("Are you absolutely sure? (type 'yes' to confirm): ")
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "yes" {
		fmt.Println("🛑 Uninstallation aborted.")
		os.Exit(0)
	}

	fmt.Println("\n🔥 Initiating Scorched Earth uninstallation...")

	// 3. STOP AND DISABLE SYSTEMD SERVICE
	fmt.Println("🛑 Stopping background daemon...")
	exec.Command("systemctl", "stop", "claviger-client.service").Run()
	exec.Command("systemctl", "disable", "claviger-client.service").Run()

	// 4. WIPE CONFIGURATIONS & VAULT
	fmt.Println("🗑️  Wiping Vault and configuration files (/etc/claviger)...")
	os.RemoveAll("/etc/claviger")

	// 5. REMOVE START MENU SHORTCUT
	fmt.Println("🧹 Removing Start Menu and Desktop integrations...")
	os.Remove("/usr/share/applications/claviger.desktop")
	os.Remove("/usr/share/applications/claviger-client.desktop") // Just in case of different naming

	// 6. REMOVE SYSTEMD SERVICE FILE
	fmt.Println("⚙️  Removing Systemd service...")
	os.Remove("/etc/systemd/system/claviger-client.service")
	exec.Command("systemctl", "daemon-reload").Run() // Tell Linux to forget the service

	// 7. SELF-DELETE (Remove the binary)
	// In Linux, an executable can actually delete its own file from the disk while running!
	fmt.Println("💀 Erasing Claviger executable...")
	binaryPath, err := os.Executable()
	if err == nil {
		os.Remove(binaryPath)
		// Also wipe standard paths just to be safe
		os.Remove("/usr/bin/claviger-client")
		os.Remove("/usr/local/bin/claviger-client")
	}

	fmt.Println("\n✅ Claviger has been completely removed from this system. Goodbye!")
	os.Exit(0)
}
