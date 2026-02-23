package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// --- JSON Data Structures ---

// What the Edge Node sends to Cloudrocean
type RegisterRequest struct {
	SetupKey string `json:"setup_key"`
	NodeID   string `json:"node_id"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

// What Cloudrocean replies with
type RegisterResponse struct {
	APIToken string `json:"api_token"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]

	switch command {
	case "setup":
		runSetup()
	case "start":
		fmt.Println("Starting Claviger daemon...")
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
	}
}

// --- Helper Functions ---

func printUsage() {
	fmt.Println("=== Claviger Edge Node ===")
	fmt.Println("Usage:")
	fmt.Println("  claviger setup  - Initialize the server, create DB, and authenticate SaaS")
	fmt.Println("  claviger start  - Run the VPN daemon and Local Admin Hub")
}

func runSetup() {
	fmt.Println("=== Claviger Edge Node Setup ===")

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter your Cloudrocean Setup Key: ")
	setupKey, _ := reader.ReadString('\n')
	setupKey = strings.TrimSpace(setupKey)

	if setupKey == "" {
		log.Fatal("❌ Setup Key cannot be empty.")
	}

	nodeID := uuid.New().String()
	nodeOS := runtime.GOOS
	nodeArch := runtime.GOARCH

	fmt.Printf("\n⚙️  Generating Node Identity: %s\n", nodeID)
	fmt.Printf("⚙️  Detecting Environment: %s/%s\n", nodeOS, nodeArch)

	db, err := sql.Open("sqlite", "claviger.db")
	if err != nil {
		log.Fatalf("❌ Failed to open database: %v", err)
	}
	defer db.Close()

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`
	if _, err = db.Exec(createTableSQL); err != nil {
		log.Fatalf("❌ Failed to create tables: %v", err)
	}

	insertSQL := `INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)`
	db.Exec(insertSQL, "node_id", nodeID)

	// The SaaS Handshake
	fmt.Println("\n🔄 Connecting to Cloudrocean API for Authorization...")

	// Execute the HTTP Request
	apiToken := authenticateApp(setupKey, nodeID, nodeOS, nodeArch)

	// Save the permanent API Token instead of the Setup Key
	db.Exec(insertSQL, "api_token", apiToken)
	fmt.Println("\n✅ Setup Complete! API Token saved securely.")
	fmt.Println("✅ You can now run: claviger start")

}

func authenticateApp(setupKey, nodeID, osName, arch string) string {
	// 1. Prepare the JSON data
	reqData := RegisterRequest{
		SetupKey: setupKey,
		NodeID:   nodeID,
		OS:       osName,
		Arch:     arch,
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		log.Fatalf("❌ Failed to encode JSON: %v", err)
	}

	// 2. Create an HTTP client with a strict 10-second timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 3. Build the POST Request (Change URL to your FastAPI endpoint)
	req, err := http.NewRequest("POST", "http://localhost:8000/v1/nodes/register", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("❌ Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 4. Fire the request
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("❌ Network error contacting Cloudrocean: %v", err)
	}
	defer resp.Body.Close()

	// 5. Handle Rejections (e.g., Invalid Setup Key)
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Fatalf("❌ API Rejected the Setup Key. Status: %d, Message: %s", resp.StatusCode, string(bodyBytes))
	}

	// 6. Parse the successful response
	var resData RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&resData); err != nil {
		log.Fatalf("❌ Failed to parse API response: %v", err)
	}

	if resData.APIToken == "" {
		log.Fatalf("❌ API returned an empty token.")
	}

	return resData.APIToken
}
