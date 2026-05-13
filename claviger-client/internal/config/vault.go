package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ClientVault holds the state of the desktop app
type ClientVault struct {
	PrivateKey       string `json:"private_key"`
	PublicKey        string `json:"public_key"`
	AssignedIP       string `json:"assigned_ip"`
	ServerKey        string `json:"server_public_key"`
	DeviceID         string `json:"device_id"`
	ServerEndpoint   string `json:"server_endpoint"`
	Status           string `json:"status"`
	UseGlobalRouting bool   `json:"use_global_routing"`
	DNS              string `json:"dns"`         // 🎯 NEW: From server (e.g., "10.8.0.1")
	BaseSubnet       string `json:"base_subnet"` // 🎯 NEW: From server (e.g., "10.8.0.0/24")
}

// getVaultPath automatically finds the correct secure folder for Win/Mac/Linux
func getVaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	appDir := filepath.Join(configDir, "Claviger")
	// Ensure the directory exists
	if err := os.MkdirAll(appDir, 0700); err != nil {
		return "", err
	}

	return filepath.Join(appDir, "vault.json"), nil
}

// Load reads the configuration from disk
func Load() (*ClientVault, error) {
	path, err := getVaultPath()
	if err != nil {
		return nil, err
	}

	file, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return an empty vault if it's a brand new installation
			// Defaulting UseGlobalRouting to false (Split Tunnel) for safety
			return &ClientVault{Status: "unregistered", UseGlobalRouting: false}, nil
		}
		return nil, err
	}

	var vault ClientVault
	if err := json.Unmarshal(file, &vault); err != nil {
		return nil, err
	}
	return &vault, nil
}

// Save writes the configuration to disk
func Save(vault *ClientVault) error {
	path, err := getVaultPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(vault, "", "  ")
	if err != nil {
		return err
	}

	// 0600 means ONLY the current user can read/write this file (Security!)
	return os.WriteFile(path, data, 0600)
}
