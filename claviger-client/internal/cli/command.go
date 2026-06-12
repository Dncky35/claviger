package cli

import (
	"fmt"
	"log"
	"net"
	"os"
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
  claviger-client list             - Show all enrolled server profiles
  claviger-client remove <id>      - Delete a server profile from this device
  
  claviger-client connect [id] [flags] - Connect to a server
      Flags:
      --global   Route ALL internet traffic through the VPN
      --split    Route ONLY internal traffic through the VPN (Default)

  claviger-client disconnect       - Gracefully shut down the active VPN connection
  claviger-client status           - Check if the VPN engine is currently running
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

// 🎯 NEW: Accepts arguments to parse Target ID and Routing Flags!
func HandleConnect(vault *config.ClientVault, args []string, disconnectChan chan bool) {
	targetID := ""

	// Scan the arguments for routing flags or a specific server ID
	for _, arg := range args {
		switch arg {
		case "--global":
			vault.UseGlobalRouting = true
			fmt.Println("🌐 Mode: GLOBAL ROUTING (All traffic routed through VPN)")
			config.Save(vault) // Save their preference for next time
		case "--split":
			vault.UseGlobalRouting = false
			fmt.Println("🌗 Mode: SPLIT TUNNEL (Only internal traffic routed through VPN)")
			config.Save(vault)
		default:
			targetID = arg // If it's not a flag, assume it's a Server ID
		}
	}

	if targetID != "" {
		if _, exists := vault.Profiles[targetID]; !exists {
			log.Fatalf("❌ Server profile '%s' not found. Run 'claviger list'.", targetID)
		}
		vault.ActiveProfileID = targetID
		_ = config.Save(vault)
	}

	if vault.ActiveProfileID == "" || len(vault.Profiles) == 0 {
		log.Fatalf("❌ No active server profile found. Please run 'claviger generate'.")
	}

	activeProfile := vault.Profiles[vault.ActiveProfileID]
	if activeProfile.Status != "active" {
		log.Fatalf("❌ Selected server is pending approval. Run 'claviger approve' first.")
	}

	log.Println("🛡️ Claviger Network Engine starting...")
	engine := vpn.NewEngine()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	err := engine.Connect(vault, activeProfile, vault.UseGlobalRouting)
	if err != nil {
		log.Fatalf("❌ Failed to connect: %v", err)
	}

	log.Println("✅ Tunnel Secured! Traffic is flowing. Press Ctrl+C to disconnect safely.")

	// 🎯 Upgrade: Block until EITHER an OS Signal OR a TCP Disconnect command arrives!
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
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		fmt.Println("⚪ Status: OFFLINE (Engine is not running)")
		return
	}
	defer conn.Close()

	// Whisper the status request
	conn.Write([]byte("STATUS"))

	buf := make([]byte, 128)
	n, _ := conn.Read(buf)
	fmt.Printf("🔵 Status: %s\n", string(buf[:n]))
}
