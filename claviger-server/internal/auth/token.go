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
type SmartTokenPayload struct {
	Token     string `json:"token"`
	ServerIP  string `json:"server_ip"`
	HubPort   string `json:"hub_port"`
	ServerKey string `json:"server_key"`
}

// GenerateSmartToken packs the raw invite, server details, and public key into one base64 string
func GenerateSmartToken(inviteToken, serverIP, hubPort, serverKey string) (string, error) {
	payload := SmartTokenPayload{
		Token:     inviteToken,
		ServerIP:  serverIP,
		HubPort:   hubPort,
		ServerKey: serverKey, // NEW
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// Encode to Base64 so it's a single, easy-to-copy string
	return base64.StdEncoding.EncodeToString(jsonData), nil
}
