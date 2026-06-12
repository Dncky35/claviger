package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// ConnectionRequest is the payload sent to the Admin (The Passport)
type ConnectionRequest struct {
	Type      string `json:"type"`
	PublicKey string `json:"public_key"`
	Hostname  string `json:"hostname"`
	Platform  string `json:"platform"`
	DeviceID  string `json:"device_id"`
}

// ConnectionApproval is the map to the Hub received from the Admin (The Visa)
type ConnectionApproval struct {
	Type           string `json:"type"`
	Role           string `json:"role"`
	AssignedIP     string `json:"assigned_ip"`
	ServerPubKey   string `json:"server_public_key"`
	ServerEndpoint string `json:"server_endpoint"`
	DNS            string `json:"dns"`         // 🎯 NEW: From server (e.g., "10.8.0.1")
	BaseSubnet     string `json:"base_subnet"` // 🎯 NEW: From server (e.g., "10.8.0.0/24")
	HubPort        string `json:"hub_port"`    // 🎯 NEW: From server
}

// GenerateRequestToken packs the client's identity into a secure Base64 string
func GenerateRequestToken(pubKey, hostname, platform, deviceID string) (string, error) {
	req := ConnectionRequest{
		Type:      "request",
		PublicKey: pubKey,
		Hostname:  hostname,
		Platform:  platform,
		DeviceID:  deviceID,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	return base64.StdEncoding.EncodeToString(jsonData), nil
}

// DecodeApprovalToken safely unpacks the Admin's Base64 Visa string
func DecodeApprovalToken(tokenString string) (*ConnectionApproval, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(tokenString)
	if err != nil {
		return nil, fmt.Errorf("invalid token format: not valid base64")
	}

	var approval ConnectionApproval
	if err := json.Unmarshal(decodedBytes, &approval); err != nil {
		return nil, fmt.Errorf("corrupted token payload: invalid json")
	}

	if approval.Type != "approval" {
		return nil, fmt.Errorf("wrong token type: expected server approval")
	}

	return &approval, nil
}
