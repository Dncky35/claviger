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
	"strings"
	"time"
)

// AppManifest defines everything the system and UI needs to know about an app
type AppManifest struct {
	ID               string `json:"id"`
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

	"cloudflared": {
		Name:             "Cloudflare Tunnel",
		Category:         "system_core",
		Description:      "Securely route traffic from Cloudflare's global edge.",
		Icon:             "☁️",
		HasCustomSetup:   true,
		NeedsDynamicPort: false,
		SetupPort:        0,
		ComposeYAML: `
version: '3.8'
services:
  cloudflared:
    image: cloudflare/cloudflared:latest
    container_name: claviger-cloudflared
    restart: unless-stopped
    environment:
      - TUNNEL_TOKEN={{.CustomToken}}
    command: tunnel --no-autoupdate run
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=cloudflared"

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
	"nextcloud": {
		Name:             "Nextcloud",
		Category:         "optional",
		Description:      "Self-hosted productivity platform, file sync, and secure collaboration.",
		Icon:             "☁️",
		HasCustomSetup:   false,
		NeedsDynamicPort: true, // NPM must handle routing to keep it behind the Zero Trust gateway
		SetupPort:        0,
		ComposeYAML: `
version: '3.3'
services:
  nextcloud:
    image: nextcloud:latest
    container_name: nextcloud
    restart: unless-stopped
    ports:
      - "{{.DynamicPort}}:80/tcp"  # Web UI handled by Claviger Proxy
    volumes:
      - ./nextcloud-data:/var/www/html
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=nextcloud"

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
	CustomToken string
}

// Install runs docker-compose for a specific app, assigning dynamic ports relative to hub_port.
func Install(db *sql.DB, appID string, isCustom bool) error {

	var manifest AppManifest

	if isCustom {
		// Route A: Fetch from local database
		row := db.QueryRow(`
		SELECT id, name, description, icon, needs_dynamic_port, compose_yaml 
		FROM custom_apps 
		WHERE id = ?
	`, appID)

		err := row.Scan(
			&manifest.ID,
			&manifest.Name,
			&manifest.Description,
			&manifest.Icon,
			&manifest.NeedsDynamicPort,
			&manifest.ComposeYAML,
		)

		if err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("custom app [%s] not found in database", appID)
			}
			return fmt.Errorf("database transaction failed: %w", err)
		}

	} else {
		// Route B: Fetch from the compiled, static Zero-Trust Catalog
		var exists bool
		manifest, exists = Catalog[appID]
		if !exists {
			return fmt.Errorf("official app [%s] is not in the system catalog", appID)
		}
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
	data := TemplateData{}
	needsTemplate := false // We use this flag to decide if we need to run the template parser

	// 3a. DYNAMIC PORT ALLOCATION
	if manifest.NeedsDynamicPort {
		assignedPort, err := GetNextAvailablePort(startRange, endRange)
		if err != nil {
			return fmt.Errorf("failed to allocate internal port in range %d-%d: %v", startRange, endRange, err)
		}

		data.DynamicPort = assignedPort // ✅ Add the port to the template data
		needsTemplate = true            // ✅ Flag that we need to run the parser

		// Save the specific app's port
		dbKey := fmt.Sprintf("app_%s_port", appID)
		if err := storage.SetConfig(db, dbKey, strconv.Itoa(assignedPort)); err != nil {
			return fmt.Errorf("failed to save assigned port to DB: %v", err)
		}
		fmt.Printf("🚀 App %s anchored to Port %d (Hub was at %d)\n", manifest.Name, assignedPort, hubPort)
	}

	// 3b. CUSTOM TOKEN FETCHING
	if appID == "cloudflared" {
		token := storage.GetConfig(db, "cloudflare_tunnel_token")
		if token == "" {
			return fmt.Errorf("cloudflared requires a tunnel token, but none was found in the database")
		}

		data.CustomToken = token // ✅ Add the token to the template data
		needsTemplate = true     // ✅ Flag that we need to run the parser
	}

	// 3c. EXECUTE TEMPLATE (If needed)
	if needsTemplate {
		tmpl, err := template.New("compose").Parse(manifest.ComposeYAML)
		if err != nil {
			return fmt.Errorf("failed to parse YAML template: %v", err)
		}

		var renderedYAML bytes.Buffer
		if err := tmpl.Execute(&renderedYAML, data); err != nil {
			return fmt.Errorf("failed to inject template data: %v", err)
		}

		finalYAML = renderedYAML.String() // This now has the REAL token injected!
	} else {
		finalYAML = manifest.ComposeYAML // Only apps with no dynamic ports and no tokens use the raw string
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

	// =========================================================
	// 5. POST-INSTALL HEALTH CHECK & ROLLBACK
	// =========================================================
	fmt.Printf("⏳ Waiting 3 seconds to verify '%s' stability...\n", manifest.Name)
	time.Sleep(3 * time.Second)

	// Ask Docker if any containers for this app are 'restarting' or 'exited'
	checkCmd := exec.Command("docker", "compose", "ps", "--status", "exited", "--status", "restarting", "-q")
	checkCmd.Dir = appDir
	checkOut, _ := checkCmd.Output()

	if len(strings.TrimSpace(string(checkOut))) > 0 {
		// 🚨 The container crashed! Grab the last 5 lines of logs to see why.
		logsCmd := exec.Command("docker", "compose", "logs", "--tail=5")
		logsCmd.Dir = appDir
		logsOut, _ := logsCmd.CombinedOutput()

		// Rollback: Destroy the broken container and network so the user can try again cleanly
		fmt.Printf("❌ App '%s' crashed! Rolling back...\n", manifest.Name)
		exec.Command("docker", "compose", "down").Run()

		return fmt.Errorf("Container crashed immediately. Reason:\n%s", string(logsOut))
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
