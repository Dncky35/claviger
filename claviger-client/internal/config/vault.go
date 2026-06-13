package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
	AutoConnect      bool                      `json:"auto_connect"`       // User preference

	// --- LEGACY FIELDS (For Auto-Migration Only) ---
	LegacyPrivateKey string `json:"private_key,omitempty"`
	LegacyPublicKey  string `json:"public_key,omitempty"`
	LegacyAssignedIP string `json:"assigned_ip,omitempty"`
	LegacyServerKey  string `json:"server_public_key,omitempty"`
	LegacyEndpoint   string `json:"server_endpoint,omitempty"`
	LegacyStatus     string `json:"status,omitempty"`
	LegacyDNS        string `json:"dns,omitempty"`
	LegacySubnet     string `json:"base_subnet,omitempty"`
}

// 🎯 UPGRADED: getVaultPath automatically finds the correct SYSTEM-WIDE secure folder
func getVaultPath() (string, error) {
	var appDir string

	switch runtime.GOOS {
	case "windows":
		// Windows System-Wide Path: C:\ProgramData\Claviger
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		appDir = filepath.Join(programData, "Claviger")
	case "darwin":
		// macOS System-Wide Path
		appDir = "/Library/Application Support/Claviger"
	default:
		// Linux System-Wide Path
		appDir = "/etc/claviger"
	}

	// Ensure the directory exists.
	// 0755 allows Root to write, and Standard Users to read.
	if err := os.MkdirAll(appDir, 0755); err != nil {
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

	// 🛡️ AUTO-MIGRATION
	if vault.LegacyPrivateKey != "" {
		migratedID := "default-profile"

		vault.Profiles[migratedID] = &ServerProfile{
			ID:             migratedID,
			Name:           "Default Server",
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

		vault.LegacyPrivateKey = ""
		vault.LegacyPublicKey = ""
		vault.LegacyAssignedIP = ""
		vault.LegacyServerKey = ""
		vault.LegacyEndpoint = ""
		vault.LegacyStatus = ""
		vault.LegacyDNS = ""
		vault.LegacySubnet = ""

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

	// 🎯 CRITICAL FIX: 0644 permissions.
	// This means the Root Daemon can save data, and your Standard User GUI
	// can read the file to populate the interface visually!
	return os.WriteFile(path, data, 0644)
}
