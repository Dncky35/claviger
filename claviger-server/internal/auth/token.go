package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// GenerateRandomHex creates a cryptographically secure random hex string
func GenerateRandomHex(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to read random bytes: %v", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateInviteToken creates the standard 'clav-xyz' connection string
func GenerateInviteToken() string {
	hexStr, err := GenerateRandomHex(8) // 8 bytes = 16 hex characters
	if err != nil {
		return "clav-err-fallback" // Extremely rare fallback
	}
	return "clav-" + hexStr
}

// AdminPayload defines what gets packed into the Setup Command's Base64 token
type AdminPayload struct {
	Token    string `json:"token"`
	ServerIP string `json:"server_ip"`
	HubPort  string `json:"hub_port"`
}

// GenerateAdminSetupToken packs the initial invite and server details into one copy-paste string
func GenerateAdminSetupToken(inviteToken, serverIP, hubPort string) (string, error) {
	payload := AdminPayload{
		Token:    inviteToken,
		ServerIP: serverIP,
		HubPort:  hubPort,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// Encode to Base64 so it's easy for the user to double-click and copy in the terminal
	return base64.StdEncoding.EncodeToString(jsonData), nil
}
