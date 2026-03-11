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

type HeartbeatPayload struct {
	NodeID           string `json:"node_id"`
	Status           string `json:"status"`
	WgStatus         string `json:"wg_status"`
	ConnectedClients int    `json:"connected_clients"`
	Version          string `json:"version"`
}

func StartHeartbeatLoop(db *sql.DB, nodeID string, apiToken string, version string) {
	apiURL := "http://localhost:8000/v1/claviger/auth/heartbeat"

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	sendHeartbeat(db, apiURL, nodeID, apiToken, version)

	for range ticker.C {
		sendHeartbeat(db, apiURL, nodeID, apiToken, version)
	}
}

func sendHeartbeat(db *sql.DB, apiURL, nodeID, apiToken, version string) {
	totalClients, activeClients := network.GetPeerCounts()

	payload := HeartbeatPayload{
		NodeID:           nodeID,
		Status:           "active",
		WgStatus:         "up",
		ConnectedClients: totalClients,
		Version:          version,
	}

	if activeClients == 0 {
		log.Printf("No client")
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

	if err != nil {
		CloudSyncStatus = "Disconnected (Network Error)"
		log.Printf("📡 Heartbeat warning: Could not reach Cloudrocean API (%v)", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		fmt.Println("\n" + strings.Repeat("!", 60))
		log.Println("🚨 CRITICAL: THIS NODE HAS BEEN REVOKED BY THE ADMIN 🚨")
		fmt.Println(strings.Repeat("!", 60))

		network.StopWireGuard()
		storage.ClearConfig(db)
		log.Println("✅ Local configuration wiped. Shutting down daemon safely.")
		os.Exit(0)
	}

	// Make absolutely sure we use `=` to update the global variables from status.go!
	if resp.StatusCode == http.StatusOK {
		CloudSyncStatus = "Healthy"
		LastCloudSync = time.Now()
	} else {
		CloudSyncStatus = fmt.Sprintf("Error: HTTP %d", resp.StatusCode)
		log.Printf("⚠️ Heartbeat warning: Cloud backend returned unexpected status %d", resp.StatusCode)
	}
}
