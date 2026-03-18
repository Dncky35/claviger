package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"claviger-server/network"
	"claviger-server/storage"
	"database/sql"
)

// The maximum time the node can be offline before it auto-suspends the VPN
const MaxOfflineDuration = 15 * time.Minute

// Tracks if the daemon intentionally took WG down due to network loss
var isSuspendedLocally bool = false

type HeartbeatPayload struct {
	NodeID           string `json:"node_id"`
	Status           string `json:"status"`
	WgStatus         string `json:"wg_status"`
	ConnectedClients int    `json:"connected_clients"`
	Version          string `json:"version"`
}

func StartHeartbeatLoop(db *sql.DB, nodeID string, apiToken string, version string) {
	apiURL := "http://localhost:8000/v1/claviger/auth/heartbeat"

	// Initialize the clock on boot so we don't instantly trigger the Dead Man's Switch
	// if the server boots up without an internet connection.
	LastCloudSync = time.Now()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Fire the first heartbeat immediately
	sendHeartbeat(db, apiURL, nodeID, apiToken, version)

	for range ticker.C {
		sendHeartbeat(db, apiURL, nodeID, apiToken, version)
	}
}

func sendHeartbeat(db *sql.DB, apiURL, nodeID, apiToken, version string) {
	totalClients, activeClients := network.GetPeerCounts()

	// If we suspended it locally, report the status accurately to the Hub
	wgStatus := "up"
	if isSuspendedLocally {
		wgStatus = "down"
	}

	payload := HeartbeatPayload{
		NodeID:           nodeID,
		Status:           "active",
		WgStatus:         wgStatus,
		ConnectedClients: totalClients,
		Version:          version,
	}

	if activeClients == 0 {
		log.Printf("No active clients connected.")
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("⚠️ Heartbeat Error: Failed to encode JSON: %v", err)
		return
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("⚠️ Heartbeat Error: Failed to create request: %v", err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)

	// =========================================================================
	// 1. NETWORK ERROR (Passive Kill / Dead Man's Switch)
	// =========================================================================
	if err != nil {
		CloudSyncStatus = "Disconnected (Network Error)"
		log.Printf("📡 Heartbeat warning: Could not reach Cloudrocean API (%v)", err)

		// Check the Dead Man's Switch
		if time.Since(LastCloudSync) > MaxOfflineDuration {
			if !isSuspendedLocally {
				log.Println("🚨 DEAD MAN'S SWITCH TRIGGERED: Hub unreachable for 15 minutes.")
				log.Println("Suspending WireGuard interface to prevent unauthorized access...")
				network.StopWireGuard()
				isSuspendedLocally = true
			}
		}
		return
	}
	defer resp.Body.Close()

	// =========================================================================
	// 2. THE POISON PILL (Active Kill by Admin)
	// =========================================================================
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		fmt.Println("\n" + strings.Repeat("!", 60))
		log.Println("🚨 CRITICAL: THIS NODE HAS BEEN REVOKED BY THE ADMIN 🚨")
		fmt.Println(strings.Repeat("!", 60))

		network.StopWireGuard()
		storage.ClearConfig(db)

		// --- ADD THIS BLOCK ---
		log.Println("🔓 Emergency Override: Re-opening SSH (Port 22) to prevent server lockout...")
		// Try UFW (Ubuntu/Debian standard)
		exec.Command("ufw", "allow", "22").Run()
		// Try generic iptables just in case UFW isn't installed
		exec.Command("iptables", "-I", "INPUT", "-p", "tcp", "--dport", "22", "-j", "ACCEPT").Run()
		// ----------------------

		log.Println("✅ Local configuration wiped. Shutting down daemon safely.")
		os.Exit(0)
	}

	// =========================================================================
	// 3. HEALTHY CONNECTION (Recovery)
	// =========================================================================
	if resp.StatusCode == http.StatusOK {
		CloudSyncStatus = "Healthy"
		LastCloudSync = time.Now()

		// If the node was suspended due to an outage, the internet is back!
		if isSuspendedLocally {
			log.Println("✅ Hub connection restored! Reactivating WireGuard interface...")

			// Note: Ensure you have a StartWireGuard() function in your network package!
			network.StartWireGuard()
			isSuspendedLocally = false
		}
	} else {
		CloudSyncStatus = fmt.Sprintf("Error: HTTP %d", resp.StatusCode)
		log.Printf("⚠️ Heartbeat warning: Cloud backend returned unexpected status %d", resp.StatusCode)
	}
}
