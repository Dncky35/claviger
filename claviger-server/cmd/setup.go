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

	storage.SaveConfig(db, "node_id", nodeID)

	// The SaaS Handshake
	fmt.Println("\n🔄 Connecting to Cloudrocean API for Authorization...")
	apiToken := api.Authenticate(setupKey, nodeID, hostname, nodeOS, nodeArch)

	// Save Secure Token
	storage.SaveConfig(db, "api_token", apiToken)
	fmt.Println("\n✅ Setup Complete! API Token saved securely.")
	fmt.Println("✅ You can now run: claviger start")
}
