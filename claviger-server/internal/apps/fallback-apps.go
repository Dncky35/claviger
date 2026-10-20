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

// Catalog is our universal App Registry
var Catalog = map[string]types.AppManifest{
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
	"jenkins": {
		Name:             "Jenkins",
		Category:         "optional",
		Description:      "The leading open-source automation server for CI/CD pipelines.",
		Icon:             "🏗️",
		HasCustomSetup:   false,
		NeedsDynamicPort: true,
		SetupPort:        0,
		ComposeYAML: `
version: '3.3'
services:
  jenkins:
    image: jenkins/jenkins:lts
    container_name: jenkins
    restart: unless-stopped
    user: root  # Required so Jenkins can interact with the host's Docker socket
    environment:
      - JAVA_OPTS="-Djava.awt.headless=true"
    ports:
      - "{{.DynamicPort}}:8080/tcp"  # Web UI handled by Claviger Proxy
      - "50000:50000/tcp"            # JNLP port for connecting external build agents
    volumes:
      - ./jenkins-data:/var/jenkins_home
      - /var/run/docker.sock:/var/run/docker.sock  # Allows Jenkins to spin up Docker environments
      - /etc/timezone:/etc/timezone:ro
      - /etc/localtime:/etc/localtime:ro
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=jenkins"

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

// LLMCatalog is the dedicated App Registry for the AI Studio
var LLMCatalog = map[string]types.AppManifest{

	// --- AI ENGINES (The Backends) ---
	"ollama": {
		Name:             "Ollama",
		Category:         "llm_engine",
		Description:      "The standard engine for local LLMs. Fast, reliable, and highly compatible.",
		Icon:             "🦙",
		HasCustomSetup:   false,
		NeedsDynamicPort: false, // Standard port 11434. Proxy not needed for background engines.
		SetupPort:        0,
		ComposeYAML: `
version: '3.8'
services:
  ollama:
    image: ollama/ollama:{{.Version}}
    container_name: ollama
    restart: unless-stopped
    ports:
      - "11434:11434"
    volumes:
      - ./ollama-data:/root/.ollama
    {{if .HasAMD}}
    devices:
      - "/dev/kfd:/dev/kfd"
      - "/dev/dri:/dev/dri"
    {{end}}
    {{if .HasNvidia}}
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
    {{end}}
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=ollama"

networks:
  cloudrocean-net:
    external: true
`,
	},

	"vllm": {
		Name:             "vLLM",
		Category:         "llm_engine",
		Description:      "High-throughput engine for production workloads. (NVIDIA Required)",
		Icon:             "🚀",
		HasCustomSetup:   false,
		NeedsDynamicPort: true, // Exposes an OpenAI-compatible API that NPM can proxy
		SetupPort:        0,
		ComposeYAML: `
version: '3.8'
services:
  vllm:
    image: vllm/vllm-openai:latest
    container_name: vllm
    restart: unless-stopped
    env_file:
      - .env
    environment:
      - HUGGING_FACE_HUB_TOKEN={{.CustomToken}}
	command: --model ${VLLM_TARGET_MODEL:-facebook/opt-125m}
    ports:
      - "{{.DynamicPort}}:8000"
    volumes:
      - ./vllm-data:/root/.cache/huggingface
    {{if .HasNvidia}}
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
    {{end}}
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=vllm"

networks:
  cloudrocean-net:
    external: true
`,
	},

	"localai": {
		Name:             "LocalAI",
		Category:         "llm_engine",
		Description:      "Complete OpenAI-compatible API replacement (LLMs, Vision, Audio).",
		Icon:             "🧠",
		HasCustomSetup:   false,
		NeedsDynamicPort: true, // Has a built-in gallery and API that needs 1808X mapping
		SetupPort:        0,
		ComposeYAML: `
version: '3.8'
services:
  localai:
    image: localai/localai:{{.Version}}
    container_name: localai
    restart: unless-stopped
    environment:
      - MODELS_PATH=/models
      - CORS=true
    ports:
      - "{{.DynamicPort}}:8080"
    volumes:
      - ./localai-models:/models
    {{if .HasNvidia}}
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
    {{end}}
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=localai"

networks:
  cloudrocean-net:
    external: true
`,
	},

	// --- AI FRONTENDS (The Interfaces) ---
	"open-webui": {
		Name:             "Open WebUI",
		Category:         "llm_frontend",
		Description:      "A polished, ChatGPT-style interface for your local AI models.",
		Icon:             "💬",
		HasCustomSetup:   false, // Users need to create their first admin account on setup
		NeedsDynamicPort: true,  // Needs a dynamic port (1808X) for Nginx Proxy Manager
		SetupPort:        0,
		ComposeYAML: `
version: '3.8'
services:
  open-webui:
    image: ghcr.io/open-webui/open-webui:main
    container_name: open-webui
    restart: unless-stopped
    environment:
      # Automatically connects to the Ollama container over the internal Docker network
      - OLLAMA_BASE_URL=http://ollama:11434
      - WEBUI_SECRET_KEY={{.CustomToken}} 
    ports:
      - "{{.DynamicPort}}:8080"
    volumes:
      - ./open-webui-data:/app/backend/data
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=open-webui"

networks:
  cloudrocean-net:
    external: true
`,
	},

	"librechat": {
		Name:             "LibreChat",
		Category:         "llm_frontend",
		Description:      "Premium chat UI capable of blending local models with cloud APIs.",
		Icon:             "✨",
		HasCustomSetup:   false,
		NeedsDynamicPort: true,
		SetupPort:        0,
		ComposeYAML: `
version: '3.8'
services:
  librechat:
    image: ghcr.io/danny-avila/librechat:latest
    container_name: librechat
    restart: unless-stopped
    environment:
      - HOST=0.0.0.0
      - PORT=3080
      - ENDPOINTS=ollama
      - OLLAMA_BASE_URL=http://ollama:11434
    ports:
      - "{{.DynamicPort}}:3080"
    volumes:
      - ./librechat-data:/app/client/public/images
      - ./librechat-env:/app/.env
    networks:
      - cloudrocean-net
    labels:
      - "claviger.app=librechat"

networks:
  cloudrocean-net:
    external: true
`,
	},
}
