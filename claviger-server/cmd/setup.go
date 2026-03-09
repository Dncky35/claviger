package cmd

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	"claviger-server/api"
	"claviger-server/network"
	"claviger-server/storage"

	"github.com/google/uuid"
)

func RunSetup(args []string) {
	// 1. Define the flag set for the 'setup' subcommand
	setupCmd := flag.NewFlagSet("setup", flag.ExitOnError)
	keyFlag := setupCmd.String("key", "", "Your Cloudrocean Setup Key")

	// Parse the flags passed from main.go
	setupCmd.Parse(args)

	fmt.Println("=== Claviger Edge Node Setup ===")

	// 2. Initialize DB and check for existing token
	db := storage.InitDB()
	defer db.Close()

	existingToken := storage.GetConfig(db, "api_token")
	if existingToken != "" {
		log.Fatal("❌ This node is already configured.\n\nManage your devices at: https://claviger.cloudrocean.com/dashboard\nTo link this server to a new license, run: claviger reset")
	}

	// 3. Determine the Setup Key
	setupKey := *keyFlag

	// If the flag wasn't provided, ask the user interactively
	if setupKey == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter your Cloudrocean Setup Key: ")
		input, _ := reader.ReadString('\n')
		setupKey = strings.TrimSpace(input)
	}

	if setupKey == "" {
		log.Fatal("❌ Setup Key cannot be empty.")
	}

	// Fetch System Info
	nodeID := uuid.New().String()
	nodeOS := runtime.GOOS
	nodeArch := runtime.GOARCH
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Unknown-Device"
	}

	fmt.Printf("\n⚙️  Generating Node Identity: %s\n", nodeID)
	fmt.Printf("⚙️  Detecting Environment: %s (%s/%s)\n", hostname, nodeOS, nodeArch)

	storage.SetConfig(db, "node_id", nodeID)

	// The SaaS Handshake
	fmt.Println("\n🔄 Connecting to Cloudrocean API for Authorization...")
	apiToken := api.Authenticate(setupKey, nodeID, hostname, nodeOS, nodeArch)

	// Save Secure Token
	storage.SetConfig(db, "api_token", apiToken)

	// --- NEW: WIREGUARD PROVISIONING ---
	fmt.Println("\n🛡️  Provisioning WireGuard VPN Interface...")

	// 1. Install it
	err := network.InstallWireGuard()
	if err != nil {
		log.Fatalf("❌ Failed to install WireGuard: %v", err)
	}

	// 2. Auto-detect the Public IP and sanitize it
	fmt.Print("⚙️  Detecting Public IP... ")
	serverIPRaw, err := network.GetPublicIP()
	if err != nil {
		log.Fatalf("❌ Failed. Ensure this server has internet access.")
	}
	serverIP := strings.TrimSpace(serverIPRaw) // FIX: Prevents trailing newline corruption
	fmt.Printf("%s\n", serverIP)

	// 3. Generate Keys for the Server AND the Admin with strict error checking
	serverPriv, serverPub, err := network.GenerateKeys()
	if err != nil {
		log.Fatalf("❌ Failed to generate Server crypto keys: %v", err)
	}

	adminPriv, adminPub, err := network.GenerateKeys()
	if err != nil {
		log.Fatalf("❌ Failed to generate Admin crypto keys: %v", err)
	}

	// 4. Save Server keys to SQLite
	storage.SetConfig(db, "wg_private_key", serverPriv)
	storage.SetConfig(db, "wg_public_key", serverPub)

	// 5. Write the server config (Adding the Admin as Peer 1)
	if err = network.WriteConfigWithAdmin(serverPriv, adminPub); err != nil {
		log.Printf("⚠️ Could not write /etc/wireguard/wg0.conf (Are you running as root?): %v\n", err)
	}

	// 6. Generate the Base64 Copy-Paste Token for the Client
	adminToken := network.GenerateAdminToken(adminPriv, serverPub, serverIP)

	// --- THE TERMINAL REVEAL ---
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🎉 NODE PROVISIONED SUCCESSFULLY!")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("1. Run 'sudo claviger-server start' to boot the daemon.")
	fmt.Println("2. Copy the token below and paste it into your Claviger Client.")
	fmt.Println("3. Once connected, open http://10.8.0.1:18080 to access the Hub.")
	fmt.Println("\n🔑 YOUR ADMIN ACCESS TOKEN:")
	fmt.Printf("\n%s\n\n", adminToken)
	fmt.Println(strings.Repeat("=", 60))
}
