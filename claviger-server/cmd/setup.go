package cmd

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"claviger-server/api"
	"claviger-server/internal/auth"
	"claviger-server/internal/system"
	"claviger-server/network"
	"claviger-server/storage"

	"github.com/google/uuid"
)

// helper function to prompt user with a default fallback
func promptUser(reader *bufio.Reader, promptText string, defaultValue string) string {
	fmt.Printf("%s [%s]: ", promptText, defaultValue)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	return input
}

func RunSetup(args []string) {
	// 1. Root / Sudo Enforcement
	if os.Geteuid() != 0 {
		log.Fatal("❌ Permission Denied. Setup must be run with root privileges (e.g., 'sudo claviger-server setup')")
	}

	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" || sudoUser == "root" {
		fmt.Println("⚠️  SECURITY WARNING: You are running this directly as the 'root' user.")
		fmt.Println("   It is highly recommended to create a standard user account and run this via 'sudo'.")
		fmt.Println("   Read more: https://claviger.cloudrocean.com/docs/security/root-access")
		fmt.Println("--------------------------------------------------")
	}

	setupCmd := flag.NewFlagSet("setup", flag.ExitOnError)
	keyFlag := setupCmd.String("key", "", "Your Cloudrocean Setup Key")
	setupCmd.Parse(args)

	fmt.Println("=== Claviger Edge Node Setup ===")

	db := storage.InitDB()
	defer db.Close()

	existingToken := storage.GetConfig(db, "api_token")
	if existingToken != "" {
		log.Fatal("❌ This node is already configured.\nManage your devices at: https://claviger.cloudrocean.com/dashboard")
	}

	reader := bufio.NewReader(os.Stdin)

	// 3. Setup Key Prompt
	setupKey := *keyFlag
	if setupKey == "" {
		fmt.Print("\n🔑 Enter your Cloudrocean Setup Key: ")
		input, _ := reader.ReadString('\n')
		setupKey = strings.TrimSpace(input)
	}

	if setupKey == "" {
		log.Fatal("❌ Setup Key cannot be empty.")
	}

	// 4. System Identity
	nodeID := uuid.New().String()
	nodeOS := runtime.GOOS
	nodeArch := runtime.GOARCH
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Unknown-Device"
	}

	fmt.Printf("\n⚙️  Generating Node Identity: %s\n", nodeID)
	storage.SetConfig(db, "node_id", nodeID)

	// 5. Cloudrocean Handshake
	fmt.Println("🔄 Connecting to Cloudrocean API for Authorization...")
	apiToken := api.Authenticate(setupKey, nodeID, hostname, nodeOS, nodeArch)
	storage.SetConfig(db, "api_token", apiToken)

	// 2. Network Configuration Prompts
	fmt.Println("\n📡 Network Configuration")
	wgPort := promptUser(reader, "Enter WireGuard Listen Port", "51820")
	hubIP := promptUser(reader, "Enter Local Hub IP Address", "10.8.0.1")
	hubPort := promptUser(reader, "Enter Local Hub Web Port", "18080")

	storage.SetConfig(db, "wg_port", wgPort)
	storage.SetConfig(db, "hub_ip", hubIP)
	storage.SetConfig(db, "hub_port", hubPort)

	// 6. WireGuard Provisioning (Server ONLY)
	fmt.Println("\n🛡️  Provisioning WireGuard VPN Interface...")
	if err := network.InstallWireGuard(); err != nil {
		log.Fatalf("❌ Failed to install WireGuard: %v", err)
	}

	fmt.Print("⚙️  Detecting Public IP... ")
	serverIPRaw, err := network.GetPublicIP()
	if err != nil {
		log.Fatalf("❌ Failed. Ensure this server has internet access.")
	}
	serverIP := strings.TrimSpace(serverIPRaw)
	fmt.Printf("%s\n", serverIP)
	storage.SetConfig(db, "public_ip", serverIP)

	// SERVER GENERATES ONLY ITS OWN KEYS
	serverPriv, serverPub, err := network.GenerateKeys()
	if err != nil {
		log.Fatalf("❌ Failed to generate Server keys: %v", err)
	}

	storage.SetConfig(db, "wg_private_key", serverPriv)
	storage.SetConfig(db, "wg_public_key", serverPub)

	// Write the base server config (No peers yet!)
	if err = network.WriteBaseConfig(serverPriv, wgPort); err != nil {
		log.Printf("⚠️ Could not write /etc/wireguard/wg0.conf: %v\n", err)
	}

	// 7. Generate the Admin Bootstrap Token using the new Auth Engine
	inviteToken := auth.GenerateInviteToken()
	expiresAt := time.Now().Add(24 * time.Hour).Format(time.RFC3339)

	serverKey := storage.GetConfig(db, "wg_public_key")

	if serverKey == "" {
		log.Fatal("❌ Server public key not found in database. Aborting.")
	}

	db.Exec("INSERT INTO invitations (token, role_id, expires_at, is_used) VALUES (?, 'admin', ?, 0)", inviteToken, expiresAt)
	adminToken, _ := auth.GenerateSmartToken(inviteToken, serverIP, hubPort, serverKey)

	// --- NEW: Install the Systemd Auto-Start Service ---
	fmt.Println("\n🔄 Installing background services...")
	if err := system.InstallSystemdService(); err != nil {
		log.Printf("⚠️ Warning: Could not install auto-start service: %v\n", err)
	}

	// --- THE TERMINAL REVEAL ---
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🎉 NODE PROVISIONED SUCCESSFULLY!")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("1. Run 'sudo systemctl start claviger' to boot the daemon in the background.\n")
	fmt.Printf("2. Open your Claviger Client and enroll using the token below.\n")
	fmt.Printf("3. Once connected, open http://%s:%s to access the Hub.\n", hubIP, hubPort)
	fmt.Println("\n🔑 YOUR ADMIN BOOTSTRAP TOKEN:")
	fmt.Printf("\n%s\n\n", adminToken)
	fmt.Println(strings.Repeat("=", 60))
}
