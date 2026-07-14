package cmd

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"claviger-server/internal/auth"
	"claviger-server/internal/crypto"
	"claviger-server/internal/firewall"
	"claviger-server/internal/security"
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

// promptValidatedPort checks if the port is a valid number and actually free on the OS
func promptValidatedPort(reader *bufio.Reader, promptText string, defaultPort string, protocol string) string {
	for {
		input := promptUser(reader, promptText, defaultPort)

		// 1. Check if it's a valid number
		portNum, err := strconv.Atoi(input)
		if err != nil || portNum < 1 || portNum > 65535 {
			fmt.Println("⚠️  Invalid port. Please enter a number between 1 and 65535.")
			continue
		}

		// 2. Ask the kernel if the port is currently available
		if protocol == "udp" {
			addr, _ := net.ResolveUDPAddr("udp", ":"+input)
			conn, err := net.ListenUDP("udp", addr)
			if err != nil {
				fmt.Printf("⚠️  Port %s (UDP) is currently in use! Please choose another.\n", input)
				continue
			}
			conn.Close()
		} else if protocol == "tcp" {
			ln, err := net.Listen("tcp", ":"+input)
			if err != nil {
				fmt.Printf("⚠️  Port %s (TCP) is currently in use! Please choose another.\n", input)
				continue
			}
			ln.Close()
		}

		return input // It passed all tests!
	}
}

// promptValidatedIP ensures the user types a real IPv4 address
func promptValidatedIP(reader *bufio.Reader, promptText string, defaultIP string) string {
	for {
		input := promptUser(reader, promptText, defaultIP)
		if net.ParseIP(input) == nil {
			fmt.Println("⚠️  Invalid IP format. Please enter a valid IPv4 address (e.g., 10.8.0.1).")
			continue
		}
		return input
	}
}

// promptYesNo handles [Y/n] questions safely
func promptYesNo(reader *bufio.Reader, promptText string, defaultYes bool) bool {
	defaultStr := "Y/n"
	if !defaultYes {
		defaultStr = "y/N"
	}

	for {
		fmt.Printf("%s [%s]: ", promptText, defaultStr)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input == "" {
			return defaultYes
		}
		if input == "y" || input == "yes" {
			return true
		}
		if input == "n" || input == "no" {
			return false
		}
		fmt.Println("⚠️  Please enter 'y' or 'n'.")
	}
}

// promptChoice handles numbered menus
func promptChoice(reader *bufio.Reader, promptText string, options map[string]string, defaultChoice string) string {
	fmt.Printf("\n%s\n", promptText)
	for key, desc := range options {
		fmt.Printf("  [%s] %s\n", key, desc)
	}

	for {
		fmt.Printf("Select an option [%s]: ", defaultChoice)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" {
			return defaultChoice
		}
		if _, exists := options[input]; exists {
			return input
		}
		fmt.Println("⚠️  Invalid selection. Please choose a valid number from the list.")
	}
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

	// --- THE GUARDRAIL: Block if already setup ---
	if storage.GetConfig(db, "node_id") != "" {
		log.Fatal("❌ Node is already configured! Please run 'sudo claviger-server reset' if you want to start over.")
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

	// Check UDP availability for WireGuard
	wgPort := promptValidatedPort(reader, "Enter WireGuard Listen Port", "51820", "udp")

	// Ensure valid IP structure
	hubIP := promptValidatedIP(reader, "Enter Local Hub IP Address", "10.8.0.1")

	// Check TCP availability for the Web Server
	hubPort := promptValidatedPort(reader, "Enter Local Hub Web Port", "18080", "tcp")

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

	defaultIP := ""
	if err != nil {
		fmt.Println("⚠️ Could not detect Public IP.")
	} else {
		defaultIP = strings.TrimSpace(serverIPRaw)
		fmt.Printf("%s\n", defaultIP)
	}

	// ==========================================
	// ENDPOINT VERIFICATION (IP or Domain)
	// ==========================================
	var finalIP string
	fmt.Printf("\n🌍 Enter the IP address or Domain Name clients will use to connect.\n")
	if defaultIP != "" {
		fmt.Printf("   (Press ENTER to use detected public IP: %s)\n", defaultIP)
	} else {
		fmt.Printf("   (e.g., 198.51.100.5, vpn.mydomain.com, or 192.168.1.50 for LAN)\n")
	}

	for {
		fmt.Print("👉 Endpoint IP/Domain: ")
		reader = bufio.NewReader(os.Stdin)
		userInput, _ := reader.ReadString('\n')
		userInput = strings.TrimSpace(userInput)

		finalIP = defaultIP
		if userInput != "" {
			finalIP = userInput
		}

		if finalIP == "" {
			log.Fatalf("❌ Setup aborted: You must provide a valid IP or domain.")
		}

		// Run the DNS/IP Verification Engine
		status, message := network.VerifyEndpoint(finalIP, defaultIP)

		// Success Cases (Valid IP or matching Domain)
		if status == network.StatusExactMatch || status == network.StatusValidIP {
			fmt.Printf("✅ %s\n", message)
			// Save it to tempConfig under the correct key!
			tempConfig["vpn_endpoint"] = finalIP
			fmt.Printf("✅ Server Endpoint successfully set to: %s\n", finalIP)
			break
		}

		// Failure/Mismatch Case
		fmt.Printf("\n%s\n", message)
		fmt.Print("❓ Do you want to FORCE USE this endpoint anyway? (y/N): ")
		overrideInput, _ := reader.ReadString('\n')
		overrideInput = strings.TrimSpace(strings.ToLower(overrideInput))

		if overrideInput == "y" || overrideInput == "yes" {
			// User forced it. Save and exit!
			fmt.Printf("✅ Server Endpoint forcefully set to: %s\n", finalIP)
			tempConfig["vpn_endpoint"] = finalIP
			break
		}

		fmt.Println("🔄 Let's try typing the endpoint again...")
		// Loop automatically restarts here
	}

	// serverPriv := keys.WireGuardPrivateKey
	// serverPub := keys.WireGuardPublicKey

	// // Generate Server Keys
	serverPriv, serverPub, err := network.GenerateKeys()
	if err != nil {
		log.Fatalf("❌ Failed to generate Server keys: %v", err)
	}

	tempConfig["wg_private_key"] = serverPriv
	tempConfig["wg_public_key"] = serverPub

	// ---------------------------------------------------------
	// 4.5. THE INITIALIZATION INTERVIEW (App Architecture)
	// ---------------------------------------------------------

	// Question 1: Reverse Proxy
	wantsProxy := promptYesNo(reader, "Will you be hosting Web Apps on public domains?", true)

	if wantsProxy {
		tempConfig["use_reverse_proxy"] = "true"

		// Question 2: Cloudflare Integration
		proxyOptions := map[string]string{
			"1": "Cloudflare (Recommended: Free SSL, DDoS Protection, CDN)",
			"2": "Standard/Direct (You will manage your own DNS records)",
		}

		providerChoice := promptChoice(reader, "Which DNS/Proxy provider will you use?", proxyOptions, "1")

		if providerChoice == "1" {
			tempConfig["proxy_provider"] = "cloudflare"
			fmt.Println("✅ Cloudflare integration enabled.")

			// Fetch the IPs and write the Nginx Real-IP config right away since they chose it during setup
			ips, err := security.FetchCloudflareIPs()
			if err != nil {
				log.Fatalf("❌ Failed to fetch Cloudflare IPs from network: %v", err)
			}
			if err := security.GenerateNginxRealIPConfig(ips, "/opt/claviger/proxy/cloudflare_ips.conf"); err != nil {
				log.Fatalf("❌ Failed to generate Nginx config: %v", err)
			}
			fmt.Println("✅ Nginx Real-IP configuration generated with current Cloudflare IPs.")

		} else {
			tempConfig["proxy_provider"] = "standard"
			fmt.Println("✅ Standard routing selected.")
		}
	} else {
		// If they say no, they are just using Claviger for pure VPN routing
		tempConfig["use_reverse_proxy"] = "false"
		tempConfig["proxy_provider"] = "none"
		fmt.Println("✅ Internal VPN routing only. (You can change this in the UI later).")
	}

	fmt.Println(strings.Repeat("-", 60))

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

	// serverPriv = hex.EncodeToString(keys.WireGuardPrivateKey)

	// 1. Generate the Master Identity (12-word Seed)
	mnemonic, err := crypto.GenerateNewMnemonic()
	if err != nil {
		log.Fatalf("❌ Failed to generate identity seed: %v", err)
	}

	if err := os.WriteFile("/var/lib/claviger/seed.txt", []byte(mnemonic), 0600); err != nil {
		log.Fatalf("❌ Failed to save recovery seed: %v", err)
	}

	// // --- NEW: Generate the AES-256 Disaster Recovery Key ---
	// keyBytes := make([]byte, 32)
	// if _, err := io.ReadFull(rand.Reader, keyBytes); err != nil {
	// 	log.Fatalf("❌ Failed to generate secure backup key: %v", err)
	// }

	// recoveryKeyHex := hex.EncodeToString(keyBytes)
	// tempConfig["backup_recovery_key"] = recoveryKeyHex

	// A. Wipe old config to ensure a totally clean slate
	storage.ClearConfig(db)

	// B. Save all configs to SQLite
	for key, value := range tempConfig {
		storage.SetConfig(db, key, value)
	}

	// C. Build the base wg0.conf file so wg-quick doesn't crash later!
	if err := os.MkdirAll("/etc/wireguard", 0700); err != nil {
		log.Fatalf("❌ Failed to create WireGuard directory: %v", err)
	}

	// Make sure SaveConfig is false so wg-quick doesn't overwrite your manual changes
	wgConfContent := fmt.Sprintf(`[Interface]
PrivateKey = %s
ListenPort = %s
Address = %s/24
SaveConfig = false
`, serverPriv, wgPort, hubIP)

	if err := os.WriteFile("/etc/wireguard/wg0.conf", []byte(wgConfContent), 0600); err != nil {
		log.Fatalf("❌ Failed to write wg0.conf file: %v", err)
	}

	// D. START WIREGUARD BEFORE ENROLLING ADMIN!
	if err := network.StartWireGuard(); err != nil {
		log.Printf("⚠️ Warning: Could not start WireGuard automatically: %v", err)
	}

	// E. Enroll the Admin in the Database and hot-inject into the kernel
	approvalData, err := auth.EnrollFirstAdmin(db, connReq, finalIP)
	if err != nil {
		log.Fatalf("❌ Failed to enroll Admin in database: %v", err)
	}

	// F. Encode the final Visa token
	finalToken, err := auth.EncodeConnectionApproval(approvalData)
	if err != nil {
		log.Fatalf("❌ Failed to generate Approval token: %v", err)
	}

	// --- Install the Systemd Auto-Start Service ---
	fmt.Println("🔄 Installing background services...")
	if err := system.InstallSystemdService(); err != nil {
		log.Printf("⚠️ Warning: Could not install auto-start service: %v\n", err)
	}

	// --- Enable WireGuard Tunnel Firewall Rules ---
	fmt.Println("🛡️  Configuring firewall rules for VPN interface...")
	// if err := exec.Command("ufw", "allow", "in", "on", "wg0").Run(); err != nil {
	// 	log.Printf("⚠️ Warning: Could not automatically configure UFW (is UFW installed and active?): %v\n", err)
	// }

	firewall.SetupFirewall(wgPort, false)

	// --- THE TERMINAL REVEAL ---
	fmt.Println("\n" + strings.Repeat("=", 65))
	fmt.Println("🎉 NODE PROVISIONED SUCCESSFULLY!")
	fmt.Println(strings.Repeat("=", 65))
	fmt.Printf("1. Run 'sudo systemctl start claviger' to boot the daemon.\n")
	fmt.Printf("2. Paste the Server Approval Token below into your Claviger Client.\n")
	fmt.Printf("3. Once connected, open http://%s:%s to access the Hub.\n", hubIP, hubPort)

	fmt.Println("\n⚠️  CRITICAL: BACKUP YOUR RECOVERY SEED!")
	fmt.Println("   If this server is destroyed, you MUST have these 12 words")
	fmt.Println("   to restore your VPN identity and decrypt your database.")
	fmt.Println("\n   👉 " + mnemonic)

	// fmt.Println("\n🛡️  DISASTER RECOVERY KEY (SAVE THIS NOW!):")
	// fmt.Println("   If this server is destroyed, you will need this key to decrypt")
	// fmt.Println("   your automated backups. It will NEVER be shown again.")
	// fmt.Printf("   👉 %s\n", recoveryKeyHex)

	fmt.Println("\n🔑 YOUR SERVER APPROVAL TOKEN:")
	fmt.Printf("\n%s\n\n", finalToken)
	fmt.Println(strings.Repeat("=", 65))
}
