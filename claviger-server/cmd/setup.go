package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"claviger-server/internal/auth"
	"claviger-server/internal/system"
	"claviger-server/network"
	"claviger-server/storage"

	"github.com/google/uuid"
)

// promptUser is a helper function to read console input with a default fallback
func promptUser(reader *bufio.Reader, prompt string, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}
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
		fmt.Println("--------------------------------------------------")
	}

	fmt.Println("=== Claviger Edge Node Setup (Open-Source Edition) ===")

	db := storage.InitDB()
	defer db.Close()

	// Since we removed the API token, we check for a node_id to see if setup was already run
	if storage.GetConfig(db, "node_id") != "" {
		log.Fatal("❌ This node is already configured.")
	}

	reader := bufio.NewReader(os.Stdin)

	// =====================================================================
	// THE TRANSACTIONAL VAULT
	// We store everything in memory. Nothing hits the DB until the very end!
	// =====================================================================
	tempConfig := make(map[string]string)

	// 2. System Identity
	nodeID := uuid.New().String()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Unknown-Device"
	}

	fmt.Printf("\n⚙️  Generating Node Identity: %s\n", nodeID)
	tempConfig["node_id"] = nodeID

	// 3. Network Configuration Prompts
	fmt.Println("\n📡 Network Configuration")
	wgPort := promptUser(reader, "Enter WireGuard Listen Port", "51820")
	hubIP := promptUser(reader, "Enter Local Hub IP Address", "10.8.0.1")
	hubPort := promptUser(reader, "Enter Local Hub Web Port", "18080")

	tempConfig["wg_port"] = wgPort
	tempConfig["hub_ip"] = hubIP
	tempConfig["hub_port"] = hubPort

	// 4. WireGuard Provisioning
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
	tempConfig["public_ip"] = serverIP

	// Generate Server Keys
	serverPriv, serverPub, err := network.GenerateKeys()
	if err != nil {
		log.Fatalf("❌ Failed to generate Server keys: %v", err)
	}
	tempConfig["wg_private_key"] = serverPriv
	tempConfig["wg_public_key"] = serverPub

	// ---------------------------------------------------------
	// 5. THE ZERO-TRUST ADMIN ENROLLMENT
	// ---------------------------------------------------------
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🛡️  ZERO-TRUST PROVISIONING")
	fmt.Println("To complete setup, open your Claviger Desktop App, click 'Generate Connection Request',")
	fmt.Println("and paste the token here.")

	requestToken := promptUser(reader, "\nPaste Connection Request", "")
	if requestToken == "" {
		log.Fatal("❌ Setup aborted. Database remains clean.")
	}

	// Decode the string into our Request Struct
	connReq, err := auth.DecodeConnectionRequest(requestToken)
	if err != nil {
		log.Fatalf("❌ Invalid Token. Setup aborted. Database remains clean: %v", err)
	}

	// =====================================================================
	// POINT OF NO RETURN (COMMIT TO SYSTEM)
	// The token is valid! Now we write to the Hard Drive and the Database.
	// =====================================================================

	fmt.Println("\n💾 Committing configurations to disk...")

	// A. Save all configs to SQLite
	for key, value := range tempConfig {
		storage.SetConfig(db, key, value)
	}

	// B. Build the base wg0.conf file so wg-quick doesn't crash later!
	if err := os.MkdirAll("/etc/wireguard", 0700); err != nil {
		log.Fatalf("❌ Failed to create WireGuard directory: %v", err)
	}

	wgConfContent := fmt.Sprintf(`[Interface]
PrivateKey = %s
ListenPort = %s
Address = %s/24
`, serverPriv, wgPort, hubIP)

	if err := os.WriteFile("/etc/wireguard/wg0.conf", []byte(wgConfContent), 0600); err != nil {
		log.Fatalf("❌ Failed to write wg0.conf file: %v", err)
	}

	// C. Enroll the Admin in the Database
	approvalData, err := auth.EnrollFirstAdmin(db, connReq, serverIP)
	if err != nil {
		log.Fatalf("❌ Failed to enroll Admin in database: %v", err)
	}

	// D. Encode the final Visa token
	finalToken, err := auth.EncodeConnectionApproval(approvalData)
	if err != nil {
		log.Fatalf("❌ Failed to generate Approval token: %v", err)
	}

	// --- Install the Systemd Auto-Start Service ---
	fmt.Println("🔄 Installing background services...")
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
