package apps

import "claviger-server/internal/types"

var FallBackCatalog = map[string]types.AppManifest{
	// --- SYSTEM CORE: THE MASTER GATEWAY ---
	"npm": {
		ID:               "npm",
		CatalogType:      "default",
		Name:             "Nginx Proxy Manager",
		Category:         "system_core",
		Description:      "The Master Gateway for SSL, subdomains, and routing.",
		Icon:             "🌐",
		HasCustomSetup:   false,
		NeedsDynamicPort: true, // NPM MUST own ports 80, 443, and 81
		SetupPort:        0,
		VersionTag:       "1.0.0",
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
		ID:               "cloudflared",
		CatalogType:      "default",
		Name:             "Cloudflare Tunnel",
		Category:         "system_core",
		Description:      "Securely route traffic from Cloudflare's global edge.",
		Icon:             "☁️",
		HasCustomSetup:   true,
		NeedsDynamicPort: false,
		SetupPort:        0,
		VersionTag:       "1.0.0",
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
		ID:               "adguard",
		CatalogType:      "default",
		Name:             "AdGuard Home",
		Category:         "system_core",
		Description:      "Network-wide ad blocking and local DNS.",
		Icon:             "🛡️",
		HasCustomSetup:   true,
		NeedsDynamicPort: true,
		SetupPort:        3030,
		VersionTag:       "1.0.0",
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
}
