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

	// ---------------------------------------------------------
	// 7. THE ZERO-TRUST ADMIN ENROLLMENT
	// ---------------------------------------------------------
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🛡️  ZERO-TRUST PROVISIONING")
	fmt.Println("To complete setup, open your Claviger Client app, click 'Generate Connection Request',")
	fmt.Println("and paste the token here.")

	requestToken := promptUser(reader, "\nPaste Connection Request", "")
	if requestToken == "" {
		log.Fatal("❌ Setup aborted. A Connection Request is required to secure the server.")
	}

	// 1. Decode the string into our Request Struct
	connReq, err := auth.DecodeConnectionRequest(requestToken)
	if err != nil {
		log.Fatalf("❌ Failed to read request: %v", err)
	}

	// 2. Pass it to our new Enrollment Engine!
	approvalData, err := auth.EnrollFirstAdmin(db, connReq, serverIP)
	if err != nil {
		log.Fatalf("❌ Failed to enroll Admin: %v", err)
	}

	// 3. Encode the Approval Data into the final Visa token!
	finalToken, err := auth.EncodeConnectionApproval(approvalData)
	if err != nil {
		log.Fatalf("❌ Failed to generate Approval token: %v", err)
	}

	// --- Install the Systemd Auto-Start Service ---
	fmt.Println("\n🔄 Installing background services...")
	if err := system.InstallSystemdService(); err != nil {
		log.Printf("⚠️ Warning: Could not install auto-start service: %v\n", err)
	}

	// --- THE TERMINAL REVEAL ---
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🎉 NODE PROVISIONED SUCCESSFULLY!")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("1. Run 'sudo systemctl start claviger' to boot the daemon.\n")
	fmt.Printf("2. Paste the Server Approval Token below into your Claviger Client.\n")
	fmt.Printf("3. Once connected, open http://%s:%s to access the Hub.\n", hubIP, hubPort)
	fmt.Println("\n🔑 YOUR SERVER APPROVAL TOKEN:")
	fmt.Printf("\n%s\n\n", finalToken)
	fmt.Println(strings.Repeat("=", 60))
}
