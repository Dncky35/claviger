package cmd // or your relevant cmd package

import (
	"fmt"
	"log"
	"os"
)

func ShowRecoveryKey() {
	// 1. Mandatory Security Check
	if os.Geteuid() != 0 {
		log.Fatal("❌ Access Denied: You must be running as root to view the master recovery key.")
	}

	seedPath := "/var/lib/claviger/seed.txt"

	// 2. Read the seed
	mnemonicBytes, err := os.ReadFile(seedPath)
	if err != nil {
		log.Fatalf("❌ Failed to read recovery seed: %v (Is the server initialized?)", err)
	}

	// 3. Clear, professional output
	fmt.Println("==========================================================")
	fmt.Println("🛡️  CLAVIGER MASTER IDENTITY RECOVERY KEY")
	fmt.Println("==========================================================")
	fmt.Printf("\n%s\n\n", string(mnemonicBytes))
	fmt.Println("----------------------------------------------------------")
	fmt.Println("⚠️  CRITICAL SECURITY WARNING:")
	fmt.Println("   This 12-word seed is the root identity of your server.")
	fmt.Println("   1. If this server is lost, you NEED this to recover.")
	fmt.Println("   2. Store this OFFLINE (on paper or a physical vault).")
	fmt.Println("   3. Never share this with anyone, including Cloudrocean.")
	fmt.Println("==========================================================")
}
