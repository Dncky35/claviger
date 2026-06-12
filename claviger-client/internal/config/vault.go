package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// 🎯 NEW: ServerProfile represents a single VPN server connection
type ServerProfile struct {
	ID             string `json:"id"`
	Name           string `json:"name"` // e.g., "Cloudrocean Hub"
	PrivateKey     string `json:"private_key"`
	PublicKey      string `json:"public_key"`
	AssignedIP     string `json:"assigned_ip"`
	ServerKey      string `json:"server_public_key"`
	ServerEndpoint string `json:"server_endpoint"`
	Status         string `json:"status"`
	DNS            string `json:"dns"`
	BaseSubnet     string `json:"base_subnet"`
	ConfigRevision string `json:"config_revision"`
	HubPort        string `json:"hub_port"` // 🎯 NEW: e.g., "10880"
}

// ClientVault holds the state of the desktop app
type ClientVault struct {
	DeviceID         string                    `json:"device_id"`          // Global unique ID for this computer
	UseGlobalRouting bool                      `json:"use_global_routing"` // User preference
	Profiles         map[string]*ServerProfile `json:"profiles"`           // Map of ID -> Profile
	ActiveProfileID  string                    `json:"active_profile_id"`  // The currently selected server

	// --- LEGACY FIELDS (For Auto-Migration Only) ---
	// omitempty ensures these disappear from the JSON file once they are empty
	LegacyPrivateKey string `json:"private_key,omitempty"`
	LegacyPublicKey  string `json:"public_key,omitempty"`
	LegacyAssignedIP string `json:"assigned_ip,omitempty"`
	LegacyServerKey  string `json:"server_public_key,omitempty"`
	LegacyEndpoint   string `json:"server_endpoint,omitempty"`
	LegacyStatus     string `json:"status,omitempty"`
	LegacyDNS        string `json:"dns,omitempty"`
	LegacySubnet     string `json:"base_subnet,omitempty"`
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
			// Brand new installation: initialize the map
			return &ClientVault{
				UseGlobalRouting: false,
				Profiles:         make(map[string]*ServerProfile),
			}, nil
		}
		return nil, err
	}

	var vault ClientVault
	if err := json.Unmarshal(file, &vault); err != nil {
		return nil, err
	}

	// Ensure the map is initialized even if the JSON didn't have it
	if vault.Profiles == nil {
		vault.Profiles = make(map[string]*ServerProfile)
	}

	// 🛡️ AUTO-MIGRATION: If we find legacy flat data, convert it to a Profile!
	if vault.LegacyPrivateKey != "" {
		migratedID := "default-profile"

		vault.Profiles[migratedID] = &ServerProfile{
			ID:             migratedID,
			Name:           "Default Server", // We give it a generic name
			PrivateKey:     vault.LegacyPrivateKey,
			PublicKey:      vault.LegacyPublicKey,
			AssignedIP:     vault.LegacyAssignedIP,
			ServerKey:      vault.LegacyServerKey,
			ServerEndpoint: vault.LegacyEndpoint,
			Status:         vault.LegacyStatus,
			DNS:            vault.LegacyDNS,
			BaseSubnet:     vault.LegacySubnet,
		}
		vault.ActiveProfileID = migratedID

		// Wipe the legacy fields so they delete themselves on the next save
		vault.LegacyPrivateKey = ""
		vault.LegacyPublicKey = ""
		vault.LegacyAssignedIP = ""
		vault.LegacyServerKey = ""
		vault.LegacyEndpoint = ""
		vault.LegacyStatus = ""
		vault.LegacyDNS = ""
		vault.LegacySubnet = ""

		// Save the newly migrated structure immediately
		_ = Save(&vault)
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
