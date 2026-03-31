package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EnrollPayload is what we send to the server
type EnrollPayload struct {
	Token     string `json:"token"`
	PublicKey string `json:"public_key"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	DeviceID  string `json:"device_id"`
}

// EnrollResponse is what the server replies with
type EnrollResponse struct {
	Status       string `json:"status"`
	Message      string `json:"message"`
	ClientID     string `json:"client_id"`
	AssignedIP   string `json:"assigned_ip"`       // Only present if Admin bypass
	ServerPubKey string `json:"server_public_key"` // Only present if Admin bypass
}

// StatusResponse is what the server sends when we poll the waiting room
type StatusResponse struct {
	Status       string `json:"status"`
	AssignedIP   string `json:"assigned_ip"`
	ServerPubKey string `json:"server_public_key"`
	HubIP        string `json:"hub_ip"`
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// RequestEnrollment sends the initial join request to the Claviger Hub
func RequestEnrollment(serverURL string, payload EnrollPayload) (*EnrollResponse, error) {
	endpoint := fmt.Sprintf("%s/api/enroll", serverURL)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: could not reach server")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf(errResp["message"])
	}

	var enrollResp EnrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&enrollResp); err != nil {
		return nil, err
	}

	return &enrollResp, nil
}

// CheckStatus pings the Hub to see if the Admin has approved us yet
func CheckStatus(serverURL, clientID string) (*StatusResponse, error) {
	endpoint := fmt.Sprintf("%s/api/client/status?id=%s", serverURL, clientID)

	resp, err := httpClient.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// Read the actual error body from the server!
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Server Error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var statusResp StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return nil, err
	}

	return &statusResp, nil
}
