package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"claviger-server/network"
	"claviger-server/storage"
	"database/sql"
)

// HeartbeatPayload matches the Pydantic schema in FastAPI exactly.
type HeartbeatPayload struct {
	NodeID           string `json:"node_id"`
	Status           string `json:"status"`
	WgStatus         string `json:"wg_status"`
	ConnectedClients int    `json:"connected_clients"`
	Version          string `json:"version"`
}

// StartHeartbeatLoop runs continuously in the background.
func StartHeartbeatLoop(db *sql.DB, nodeID string, apiToken string, version string) {
	// The URL of your FastAPI backend (Update this to your actual production URL later)
	apiURL := "http://localhost:8000/v1/claviger/auth/heartbeat"

	// Create a ticker that fires every 60 seconds
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Run the first heartbeat immediately before entering the loop
	sendHeartbeat(db, apiURL, nodeID, apiToken, version)

	// Infinite loop: waits for the ticker to "tick" every 60s
	for range ticker.C {
		sendHeartbeat(db, apiURL, nodeID, apiToken, version)
	}
}

func sendHeartbeat(db *sql.DB, apiURL, nodeID, apiToken, version string) {
	// 1. Gather Telemetry
	// For now, we hardcode wg_status to "up" and clients to 0.
	// We will write the Linux commands to fetch real stats in the next step!
	payload := HeartbeatPayload{
		NodeID:           nodeID,
		Status:           "active",
		WgStatus:         "up",
		ConnectedClients: 0,
		Version:          version,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("⚠️ Heartbeat Error: Failed to encode JSON: %v", err)
		return
	}

	// 2. Prepare the HTTP Request
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("⚠️ Heartbeat Error: Failed to create request: %v", err)
		return
	}

	// Add the Zero Trust Headers
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	// 3. Send the Request with a strict 10-second timeout
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)

	// If the internet goes down, just log a warning and try again in 60s
	if err != nil {
		log.Printf("📡 Heartbeat warning: Could not reach Cloudrocean API (%v)", err)
		return
	}
	defer resp.Body.Close()

	// 4. --- THE POISON PILL CHECK ---
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		fmt.Println("\n" + strings.Repeat("!", 60))
		log.Println("🚨 CRITICAL: THIS NODE HAS BEEN REVOKED BY THE ADMIN 🚨")
		fmt.Println(strings.Repeat("!", 60))

		// Step A: Kill the VPN so it stops routing traffic immediately
		network.StopWireGuard()

		// Step B: Wipe the local database so it can never reconnect
		storage.ClearConfig(db)

		log.Println("✅ Local configuration wiped. Shutting down daemon safely.")

		// Step C: Forcefully exit the Go application
		os.Exit(0)
	}

	// 5. --- SUCCESS & ERROR LOGGING ---
	if resp.StatusCode == http.StatusOK {
		log.Printf("💖 Heartbeat sent successfully (Status: %d)", resp.StatusCode)
	} else {
		// Catch 422s, 500s, or any other weird errors so we can debug them!
		log.Printf("⚠️ Heartbeat warning: Cloud backend returned unexpected status %d", resp.StatusCode)
	}
}
