package apps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// AppManifest holds the raw docker-compose YAML for our supported apps
var Catalog = map[string]string{
	"adguard": `
version: '3.3'
services:
  adguardhome:
    image: adguard/adguardhome
    container_name: adguard
    restart: unless-stopped
    ports:
      - "53:53/tcp"
      - "53:53/udp"
      - "3030:3000/tcp" # Setup Dashboard (Moved away from Next.js 3000)
      - "8083:80/tcp"   # Main Web Interface (After setup is done)
    volumes:
      - ./work:/opt/adguardhome/work
      - ./conf:/opt/adguardhome/conf
    labels:
      - "claviger.app=adguard"
`,
}

// Install runs docker-compose for a specific app
func Install(appID string) error {
	yamlContent, exists := Catalog[appID]
	if !exists {
		return fmt.Errorf("app %s is not in the catalog", appID)
	}

	// 1. Create a dedicated folder for the app data
	// e.g., /var/lib/claviger/apps/adguard/
	appDir := filepath.Join("/var/lib/claviger/apps", appID)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return fmt.Errorf("failed to create app directory: %v", err)
	}

	// 2. Write the docker-compose.yml file
	composePath := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(yamlContent), 0644); err != nil {
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
