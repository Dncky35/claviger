package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// ==========================================
// 1. DATA MODELS
// ==========================================

// ServerState represents the JSON map returned by the server
type ServerState struct {
	Server_Endpoint string `json:"server_endpoint"`
	DNS             string `json:"dns"`
	MTU             string `json:"mtu"`
	Revision        string `json:"revision"`
}

// SyncClient holds the state for the background poller
// 🎯 THIS DEFINITION FIXES YOUR "UndeclaredName" ERROR!
type SyncClient struct {
	ServerURL     string
	LocalRevision int
	DeviceName    string
	httpClient    *http.Client
}

// ==========================================
// 2. CONSTRUCTOR
// ==========================================

// NewSyncClient initializes a new background poller instance
func NewSyncClient(serverURL string, deviceName string, currentRevision int) *SyncClient {
	return &SyncClient{
		ServerURL:     serverURL,
		DeviceName:    deviceName,
		LocalRevision: currentRevision,
		// A 10-second timeout ensures the app never freezes if the server goes offline
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ==========================================
// 3. FETCH LOGIC
// ==========================================

// FetchServerState reaches out to the server and parses the response
func (c *SyncClient) FetchServerState(deviceKey string) (*ServerState, error) {
	// Construct the URL
	targetURL := fmt.Sprintf("%s/api/status/sync-state", c.ServerURL)
	log.Printf("🔍 [SyncManager] Initiating fetch to %s", targetURL)

	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		log.Printf("❌ [SyncManager] Failed to build request: %v", err)
		return nil, fmt.Errorf("failed to build request: %v", err)
	}

	// Inject the authentication header using the Device ID from your ClientVault
	req.Header.Set("X-Device-Key", deviceKey)
	log.Printf("🛡️ [SyncManager] Injected Auth Header (X-Device-Key): %s", deviceKey)

	// Execute the request
	log.Printf("⏳ [SyncManager] Waiting for server response...")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("❌ [SyncManager] Network error during fetch: %v", err)
		return nil, fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()

	log.Printf("📡 [SyncManager] Received HTTP Status: %d", resp.StatusCode)

	// Handle Server Status Codes
	switch resp.StatusCode {
	case http.StatusOK:
		log.Printf("✅ [SyncManager] Status 200 OK. Proceeding to parse JSON...")
	case http.StatusUnauthorized, http.StatusForbidden:
		log.Printf("🚨 [SyncManager] FATAL: Device revoked or unauthorized by server!")
		return nil, fmt.Errorf("REVOKED")
	case http.StatusServiceUnavailable:
		log.Printf("⚠️ [SyncManager] Server VPN endpoint not yet configured (503).")
		return nil, fmt.Errorf("server VPN endpoint not yet configured")
	default:
		log.Printf("⚠️ [SyncManager] Unexpected server status: %d", resp.StatusCode)
		return nil, fmt.Errorf("unexpected server status: %d", resp.StatusCode)
	}

	// Parse the JSON into our ServerState struct
	var state ServerState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		log.Printf("❌ [SyncManager] Failed to decode JSON response: %v", err)
		return nil, fmt.Errorf("invalid json response: %v", err)
	}

	log.Printf("📦 [SyncManager] Successfully parsed state: %+v", state)
	return &state, nil
}
