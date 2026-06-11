package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ==========================================
// 1. DATA MODELS
// ==========================================

// ServerState represents the JSON map returned by the server
type ServerState struct {
	Endpoint string `json:"endpoint"`
	DNS      string `json:"dns"`
	MTU      string `json:"mtu"`
	Revision string `json:"revision"`
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
	targetURL := fmt.Sprintf("%s/api/v1/sync/state", c.ServerURL)

	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %v", err)
	}

	// Inject the authentication header using the Device ID from your ClientVault
	req.Header.Set("X-Device-Key", deviceKey)

	// Execute the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()

	// Handle Server Status Codes
	switch resp.StatusCode {
	case http.StatusOK:
		// Success, continue to parse below
	case http.StatusUnauthorized, http.StatusForbidden:
		// 🚨 FATAL: The device was revoked or deleted from the server DB!
		return nil, fmt.Errorf("REVOKED")
	case http.StatusServiceUnavailable:
		return nil, fmt.Errorf("server VPN endpoint not yet configured")
	default:
		return nil, fmt.Errorf("unexpected server status: %d", resp.StatusCode)
	}

	// Parse the JSON into our ServerState struct
	var state ServerState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, fmt.Errorf("invalid json response: %v", err)
	}

	return &state, nil
}
