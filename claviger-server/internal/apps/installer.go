package apps

import (
	"bytes"
	"claviger-server/storage"
	"database/sql"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// AppManifest defines everything the system and UI needs to know about an app
type AppManifest struct {
	Name             string // Clean name for the UI
	Category         string // "system_core" or "optional"
	Description      string // Subtitle for the UI
	Icon             string // Emoji icon for the UI
	HasCustomSetup   bool   // Does it have a first-time setup wizard?
	NeedsDynamicPort bool   // Should Claviger assign an 1808X port?
	SetupPort        int    // The port for the setup wizard (0 if none)
	ComposeYAML      string // The docker-compose.yml template
}

// Catalog is our universal App Registry
var Catalog = map[string]AppManifest{
	// --- SYSTEM CORE: THE MASTER GATEWAY ---
	"npm": {
		Name:             "Nginx Proxy Manager",
		Category:         "system_core",
		Description:      "The Master Gateway for SSL, subdomains, and routing.",
		Icon:             "🌐",
		HasCustomSetup:   false,
		NeedsDynamicPort: true, // NPM MUST own ports 80, 443, and 81
		SetupPort:        0,
		ComposeYAML: `
version: '3.8'
services:
  app:
    image: 'jc21/nginx-proxy-manager:latest'
    container_name: npm
    restart: unless-stopped
    ports:
      - '80:80'   # Public HTTP (Must stay static)
      - '443:443' # Public HTTPS (Must stay static)
      - '{{.DynamicPort}}:81' # 🎯 Admin UI joins the 1808X secure block!
    volumes:
      - ./data:/data
      - ./letsencrypt:/etc/letsencrypt
      - /opt/claviger/proxy/cloudflare_ips.conf:/data/nginx/custom/http_top.conf
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=npm"

networks:
  cloudrocean-net:
    external: true
`,
	},

	// --- SYSTEM CORE: NETWORK SERVICES ---
	"adguard": {
		Name:             "AdGuard Home",
		Category:         "system_core",
		Description:      "Network-wide ad blocking and local DNS.",
		Icon:             "🛡️",
		HasCustomSetup:   true,
		NeedsDynamicPort: true,
		SetupPort:        3030,
		ComposeYAML: `
services:
  adguardhome:
    image: adguard/adguardhome
    container_name: adguard
    restart: unless-stopped
    ports:
      - "53:53/tcp"
      - "53:53/udp"
      - "3030:3000/tcp" # Setup Dashboard (mapped to 3030)
      - "{{.DynamicPort}}:80/tcp" # Main UI (Assigned from 1808X block)
    volumes:
      - ./work:/opt/adguardhome/work
      - ./conf:/opt/adguardhome/conf
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=adguard"

networks:
  cloudrocean-net:
    external: true
`,
	},

	// --- OPTIONAL EXTENSIONS ---
	"vaultwarden": {
		Name:             "Vaultwarden",
		Category:         "optional",
		Description:      "Private password manager (Bitwarden compatible).",
		Icon:             "🔑",
		HasCustomSetup:   false,
		NeedsDynamicPort: true,
		SetupPort:        0,
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
      - "{{.DynamicPort}}:80/tcp" # Assigned from 1808X block
    volumes:
      - ./vw-data:/data
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=vaultwarden"

networks:
  cloudrocean-net:
    external: true
`,
	},
	"gitea": {
		Name:             "Gitea",
		Category:         "optional",
		Description:      "Painless self-hosted Git repository and CI/CD.",
		Icon:             "🍵",
		HasCustomSetup:   false,
		NeedsDynamicPort: true,
		SetupPort:        0,
		ComposeYAML: `
version: '3.3'
services:
  gitea:
    image: gitea/gitea:latest
    container_name: gitea
    restart: unless-stopped
    environment:
      - USER_UID=1000
      - USER_GID=1000
    ports:
      - "{{.DynamicPort}}:3000/tcp"  # Web UI handled by Claviger Proxy
      - "2222:22/tcp"                # SSH port (avoids conflict with host OS port 22)
    volumes:
      - ./gitea-data:/data
      - /etc/timezone:/etc/timezone:ro
      - /etc/localtime:/etc/localtime:ro
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=gitea"

networks:
  cloudrocean-net:
    external: true
`,
	},
	"rustdesk": {
		Name:             "RustDesk Server",
		Category:         "optional",
		Description:      "Self-hosted remote desktop infrastructure (ID & Relay).",
		Icon:             "🖥️",
		HasCustomSetup:   false,
		NeedsDynamicPort: false, // Uses standard fixed ports (TCP/UDP) instead of the Web Proxy
		SetupPort:        0,
		ComposeYAML: `
version: '3.3'
services:
  hbbs:
    image: rustdesk/rustdesk-server:latest
    container_name: rustdesk-hbbs
    restart: unless-stopped
    command: hbbs -r hbbr
    ports:
      - "21115:21115/tcp"
      - "21116:21116/tcp"
      - "21116:21116/udp"
      - "21118:21118/tcp"
    volumes:
      - ./rustdesk-data:/root
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=rustdesk-hbbs"
    depends_on:
      - hbbr

  hbbr:
    image: rustdesk/rustdesk-server:latest
    container_name: rustdesk-hbbr
    restart: unless-stopped
    command: hbbr
    ports:
      - "21117:21117/tcp"
      - "21119:21119/tcp"
    volumes:
      - ./rustdesk-data:/root
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=rustdesk-hbbr"

networks:
  cloudrocean-net:
    external: true
`,
	},
}

// Install runs docker-compose for a specific app
// TemplateData holds the variables we will inject into the YAML
type TemplateData struct {
	DynamicPort int
}

// Install runs docker-compose for a specific app, assigning dynamic ports relative to hub_port.
func Install(db *sql.DB, appID string) error {
	manifest, exists := Catalog[appID]
	if !exists {
		return fmt.Errorf("app %s is not in the catalog", appID)
	}

	// 1. Fetch the Anchor Port from DB
	// We retrieve what the user entered during the 'setup' phase
	hubPortStr := storage.GetConfig(db, "hub_port")
	hubPort, err := strconv.Atoi(hubPortStr)
	if err != nil || hubPort == 0 {
		// Fallback to a sensible default if the DB entry is missing/corrupt
		hubPort = 18080
	}

	// 2. Define the search range starting right after the Hub
	startRange := hubPort + 1
	endRange := hubPort + 100 // Allow up to 100 Claviger apps in this block

	appDir := filepath.Join("/var/lib/claviger/apps", appID)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return fmt.Errorf("failed to create app directory: %v", err)
	}

	var finalYAML string

	// 3. DYNAMIC PORT ALLOCATION
	if manifest.NeedsDynamicPort {
		// 🎯 Use the dynamic range based on the user's hub_port choice
		assignedPort, err := GetNextAvailablePort(startRange, endRange)
		if err != nil {
			return fmt.Errorf("failed to allocate internal port in range %d-%d: %v", startRange, endRange, err)
		}

		// Inject into the YAML template
		tmpl, err := template.New("compose").Parse(manifest.ComposeYAML)
		if err != nil {
			return fmt.Errorf("failed to parse YAML template: %v", err)
		}

		var renderedYAML bytes.Buffer
		data := TemplateData{DynamicPort: assignedPort}
		if err := tmpl.Execute(&renderedYAML, data); err != nil {
			return fmt.Errorf("failed to inject dynamic port: %v", err)
		}

		finalYAML = renderedYAML.String()

		// Save the specific app's port so we don't have to scan for it again later
		dbKey := fmt.Sprintf("app_%s_port", appID)
		if err := storage.SetConfig(db, dbKey, strconv.Itoa(assignedPort)); err != nil {
			return fmt.Errorf("failed to save assigned port to DB: %v", err)
		}

		fmt.Printf("🚀 App %s anchored to Port %d (Hub was at %d)\n", manifest.Name, assignedPort, hubPort)

	} else {
		finalYAML = manifest.ComposeYAML
	}

	// 4. Write and Deploy
	composePath := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(finalYAML), 0644); err != nil {
		return fmt.Errorf("failed to write compose file: %v", err)
	}

	cmd := exec.Command("docker", "compose", "up", "-d")
	cmd.Dir = appDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose failed: %s\nError: %v", string(output), err)
	}

	fmt.Printf("✅ App '%s' successfully deployed and started!\n", manifest.Name)

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
