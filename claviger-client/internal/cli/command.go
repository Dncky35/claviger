package cli

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
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
  claviger-client update		   - Check and done the update
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

func HandleDisconnect(vault *config.ClientVault) {
	fmt.Println("🛑 Sending disconnect signal to Claviger Engine...")

	// 1. Dial the background process with a timeout
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		fmt.Println("⚪ Claviger is not currently running. Nothing to disconnect.")
		return
	}
	defer conn.Close()

	// 2. Set a Read Deadline so the CLI doesn't hang if the daemon stalls
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	// 3. Whisper the disconnect command
	if _, err := conn.Write([]byte("DISCON")); err != nil {
		fmt.Println("❌ Failed to send command to daemon.")
		return
	}

	// 4. Read the response
	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("❌ Connection lost while waiting for daemon: %v\n", err)
		return
	}

	response := strings.TrimSpace(string(buf[:n]))

	// 5. Handle the result
	if response == "OK" {
		fmt.Println("✅ Signal acknowledged by daemon.")

		// UX: Professional teardown sequence
		time.Sleep(400 * time.Millisecond)
		fmt.Println("🧹 Tearing down secure tunnels & resetting DNS...")

		time.Sleep(600 * time.Millisecond)
		fmt.Println("👋 Claviger disconnected gracefully. Normal network restored.")
	} else {
		fmt.Printf("⚠️ Daemon replied with error: %s\n", response)
	}
}

func HandleUpdate() {
	fmt.Println("🚀 Initializing secure Claviger update...")
	cmd := exec.Command("bash", "-c", "curl -sSL https://cloudrocean.com/installers/claviger-client.sh | sudo bash")

	// Bind the output so the user sees the bash script's progress in their terminal
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Printf("❌ Update failed: %v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
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
