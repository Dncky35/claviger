package apps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// AppManifest defines everything the system and UI needs to know about an app
type AppManifest struct {
	Name           string // Clean name for the UI (e.g., "AdGuard Home")
	Category       string // "system_core" or "optional"
	Description    string // Subtitle for the UI
	Icon           string // Emoji icon for the UI
	HasCustomSetup bool   // Does it have a first-time setup wizard?
	SetupPort      int    // The port for the setup wizard (0 if none)
	DashPort       int    // The port for the main dashboard
	ComposeYAML    string // The raw docker-compose.yml file
}

// Catalog is our new universal App Registry
var Catalog = map[string]AppManifest{
	// --- SYSTEM CORE APPS ---
	"adguard": {
		Name:           "AdGuard Home",
		Category:       "system_core",
		Description:    "Network-wide ad blocking and local DNS.",
		Icon:           "🛡️",
		HasCustomSetup: true,
		SetupPort:      3030,
		DashPort:       8083,
		ComposeYAML: `
version: '3.3'
services:
  adguardhome:
    image: adguard/adguardhome
    container_name: adguard
    restart: unless-stopped
    ports:
      - "53:53/tcp"
      - "53:53/udp"
      - "3030:3000/tcp" # Setup Dashboard
      - "8083:80/tcp"   # Main Web Interface
    volumes:
      - ./work:/opt/adguardhome/work
      - ./conf:/opt/adguardhome/conf
    labels:
      - "claviger.app=adguard"
`,
	},

	// --- OPTIONAL EXTENSIONS ---
	"vaultwarden": {
		Name:           "Vaultwarden",
		Category:       "optional",
		Description:    "Private, self-hosted password manager (Bitwarden compatible).",
		Icon:           "🔑",
		HasCustomSetup: false, // No setup required, jumps straight to dashboard
		SetupPort:      0,
		DashPort:       8222, // Custom port to avoid conflicts with Claviger/Nginx
		ComposeYAML: `
version: '3.3'
services:
  vaultwarden:
    image: vaultwarden/server:latest
    container_name: vaultwarden
    restart: unless-stopped
    environment:
      - WEBSOCKET_ENABLED=true
    ports:
      - "8222:80/tcp"
    volumes:
      - ./vw-data:/data
    labels:
      - "claviger.app=vaultwarden"
`,
	},
}

// Install runs docker-compose for a specific app
func Install(appID string) error {
	manifest, exists := Catalog[appID] // NEW: We now extract the manifest struct
	if !exists {
		return fmt.Errorf("app %s is not in the catalog", appID)
	}

	// 1. Create a dedicated folder for the app data
	appDir := filepath.Join("/var/lib/claviger/apps", appID)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return fmt.Errorf("failed to create app directory: %v", err)
	}

	// 2. Write the docker-compose.yml file (pulling from the manifest)
	composePath := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(manifest.ComposeYAML), 0644); err != nil {
		return fmt.Errorf("failed to write compose file: %v", err)
	}

	// 3. Execute 'docker compose up -d'
	cmd := exec.Command("docker", "compose", "up", "-d")
	cmd.Dir = appDir // Run the command INSIDE the new folder

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose failed: %s\nError: %v", string(output), err)
	}

	return nil
}

// Uninstall cleanly removes an app's containers, networks, and persistent data
func Uninstall(appID string) error {
	appDir := filepath.Join("/var/lib/claviger/apps", appID)

	// 1. Check if the app directory actually exists
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		return fmt.Errorf("app %s is not installed (directory missing)", appID)
	}

	// 2. Execute 'docker compose down -v'
	cmd := exec.Command("docker", "compose", "down", "-v")
	cmd.Dir = appDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down failed: %s\nError: %v", string(output), err)
	}

	// 3. The Data Wipe (Scorched Earth for this app)
	if err := os.RemoveAll(appDir); err != nil {
		return fmt.Errorf("failed to wipe app data directory: %v", err)
	}

	return nil
}
