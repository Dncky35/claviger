package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// EnrollPayload matches what the server's /api/enroll endpoint expects
type EnrollPayload struct {
	Token     string `json:"token"`
	PublicKey string `json:"public_key"`
	Name      string `json:"name"` // Changed from device_name to name
	Platform  string `json:"platform"`
	DeviceID  string `json:"device_id"` // Added to match server
}

func main() {
	fmt.Println("=== Claviger Zero Trust Client ===")

	// 1. Setup CLI Flags
	enrollCmd := flag.NewFlagSet("enroll", flag.ExitOnError)
	tokenFlag := enrollCmd.String("token", "", "The invitation token provided by your admin")
	serverFlag := enrollCmd.String("server", "", "The public IP and port of the Hub (e.g., http://203.0.113.5:18080)")

	if len(os.Args) < 2 {
		fmt.Println("Expected 'enroll' command.")
		fmt.Println("Usage: claviger-client enroll --token clav-1234 --server http://<PUBLIC_IP>:18080")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "enroll":
		enrollCmd.Parse(os.Args[2:])
		if *tokenFlag == "" || *serverFlag == "" {
			log.Fatal("❌ Both --token and --server are required for enrollment.")
		}
		handleEnrollment(*tokenFlag, *serverFlag)
	default:
		fmt.Println("Unknown command. Available commands: enroll")
		os.Exit(1)
	}
}

func handleEnrollment(token, serverURL string) {
	fmt.Println("\n🔑 Generating secure cryptographic identity...")

	// 2. Generate local WireGuard Keys
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		log.Fatalf("❌ Failed to generate keys: %v", err)
	}

	// privateKey := key.String()
	publicKey := key.PublicKey().String()

	// In a real app, you MUST save privateKey to SQLite/disk here!
	// For this MVP test, we will just print it.
	fmt.Printf("   Private Key: [HIDDEN/SAVED]\n")
	fmt.Printf("   Public Key:  %s\n", publicKey)

	// 3. Gather Device Info
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "Unknown-Device"
	}
	platform := runtime.GOOS

	fmt.Printf("🖥️  Registering device: %s (%s)\n", hostname, platform)

	// 4. Build the payload
	payload := EnrollPayload{
		Token:     token,
		PublicKey: publicKey,
		Name:      hostname, // Changed from DeviceName
		Platform:  platform,
		DeviceID:  "dummy-id-123", // In the future, we can grab real hardware UUIDs here
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("❌ Failed to encode payload: %v", err)
	}

	// 5. Send to Server
	fmt.Printf("📡 Connecting to Claviger Hub at %s...\n", serverURL)
	endpoint := fmt.Sprintf("%s/api/enroll", serverURL)

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("❌ Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("❌ Network error. Is the server reachable? Error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		fmt.Println("\n✅ ENROLLMENT SUCCESSFUL!")
		fmt.Println("⏳ Your device is now in the 'Waiting Room'.")
		fmt.Println("👉 Ask your network administrator to approve your device in the Hub.")

		// TO DO LATER: After approval, the client needs to fetch its assigned 10.8.0.x IP
		// and the server's Public Key to actually build its local wg0.conf file!
	} else {
		log.Fatalf("❌ Enrollment rejected by server. Status Code: %d", resp.StatusCode)
	}
}
