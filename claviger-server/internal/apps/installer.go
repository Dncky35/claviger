package apps

import (
	"bytes"
	"claviger-server/internal/hardware"
	"claviger-server/internal/types"
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

// Install runs docker-compose for a specific app
// TemplateData holds the variables we will inject into the YAML
type TemplateData struct {
	DynamicPort int
	CustomToken string
}

// Install runs docker-compose for a specific app, assigning dynamic ports relative to hub_port.
func Install(db *sql.DB, appID string, isCustom bool) error {

	var manifest types.AppManifest

	if isCustom {
		// Route A: User's Custom Apps
		app, found, err := storage.GetCustomAppByID(db, appID)
		if err != nil {
			return fmt.Errorf("database error checking custom app: %w", err)
		}
		if !found {
			return fmt.Errorf("custom app [%s] not found in database", appID)
		}
		manifest = app

	} else {
		// Route B: Official Apps (Remote sync -> Local fallback)
		app, found, err := storage.GetRemoteAppByID(db, appID)
		if err != nil {
			return fmt.Errorf("database error checking remote apps: %w", err)
		}

		// Ensure we only install from the 'default' catalog
		if found && app.CatalogType == "default" {
			manifest = app
		} else {
			// Route C: Offline / Static Fallback
			staticApp, staticFound := FallBackCatalog[appID] // Make sure apps.Catalog is imported
			if !staticFound {
				return fmt.Errorf("official app [%s] not found in remote or static catalog", appID)
			}
			manifest = staticApp
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

// LLMTemplateData holds the variables injected into the docker-compose.yml
type LLMTemplateData struct {
	DynamicPort int
	CustomToken string
	Version     string
	HasAMD      bool
	HasNvidia   bool
}

// InstallLLM deploys an AI engine or frontend, handling GPU passthrough and dynamic ports.
func InstallLLM(db *sql.DB, appID string, targetVersion string) error {

	var manifest types.AppManifest
	app, found, err := storage.GetRemoteAppByID(db, appID)
	if err != nil {
		return fmt.Errorf("database error checking remote apps: %w", err)
	}

	// Ensure we only install from the 'default' catalog
	if found && app.CatalogType == "llm" {
		manifest = app
	} else {
		// Route C: Offline / Static Fallback
		staticApp, staticFound := FallBackCatalog[appID] // Make sure apps.Catalog is imported
		if !staticFound {
			return fmt.Errorf("official app [%s] not found in remote or static catalog", appID)
		}
		manifest = staticApp
	}

	// 1. Fetch the Anchor Port from DB (Fallback to 18080)
	hubPortStr := storage.GetConfig(db, "hub_port")
	hubPort, err := strconv.Atoi(hubPortStr)
	if err != nil || hubPort == 0 {
		hubPort = 18080
	}

	// Use a slightly different port range for LLM Apps to avoid colliding with regular apps
	startRange := hubPort + 101
	endRange := hubPort + 200

	// Keep LLM apps in a separate directory from standard web apps
	appDir := filepath.Join("/var/lib/claviger/llms", appID)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return fmt.Errorf("failed to create llm directory: %v", err)
	}

	// Default version to latest if none specified
	if targetVersion == "" {
		targetVersion = "latest"
	}

	// 2. Prepare the Template Data
	data := LLMTemplateData{
		Version:   targetVersion,
		HasAMD:    false,
		HasNvidia: false,
	}

	// 3. HARDWARE PROFILER INTEGRATION (The Magic Step)
	// We scan the system *right now* to see what GPU to mount
	profile, err := hardware.RunProfiler()
	if err == nil && profile.GPU.HasGPU {
		switch profile.GPU.Vendor {
		case "amd":
			data.HasAMD = true
			fmt.Println("🚀 AMD GPU Detected: Injecting /dev/kfd and /dev/dri into container.")
		case "nvidia":
			data.HasNvidia = true
			fmt.Println("🚀 NVIDIA GPU Detected: Injecting CUDA capabilities into container.")
		}
	}

	// 4. DYNAMIC PORT ALLOCATION
	if manifest.NeedsDynamicPort {
		assignedPort, err := GetNextAvailablePort(startRange, endRange)
		if err != nil {
			return fmt.Errorf("failed to allocate internal port in range %d-%d: %v", startRange, endRange, err)
		}

		data.DynamicPort = assignedPort

		// Save the specific app's port so NPM or the UI knows where to route
		dbKey := fmt.Sprintf("llm_%s_port", appID)
		if err := storage.SetConfig(db, dbKey, strconv.Itoa(assignedPort)); err != nil {
			return fmt.Errorf("failed to save assigned port to DB: %v", err)
		}
		fmt.Printf("🔌 LLM App %s anchored to Port %d\n", manifest.Name, assignedPort)
	}

	// 5. SECRETS & TOKENS
	if appID == "vllm" {
		data.CustomToken = storage.GetConfig(db, "hf_hub_token") // HuggingFace Token
	}
	if appID == "open-webui" {
		token := storage.GetConfig(db, "webui_secret_key")
		if token == "" {
			// Generate a basic random token if the user hasn't set one, so the container doesn't crash
			token = fmt.Sprintf("claviger-secure-%d", time.Now().UnixNano())
			storage.SetConfig(db, "webui_secret_key", token)
		}
		data.CustomToken = token
	}

	// 6. COMPILE THE TEMPLATE
	tmpl, err := template.New("compose").Parse(manifest.ComposeYAML)
	if err != nil {
		return fmt.Errorf("failed to parse YAML template: %v", err)
	}

	var renderedYAML bytes.Buffer
	if err := tmpl.Execute(&renderedYAML, data); err != nil {
		return fmt.Errorf("failed to inject template data: %v", err)
	}

	finalYAML := renderedYAML.String()

	// 7. WRITE AND DEPLOY
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

	// 8. POST-INSTALL HEALTH CHECK & ROLLBACK
	fmt.Printf("⏳ Waiting 4 seconds to verify '%s' stability...\n", manifest.Name)
	time.Sleep(4 * time.Second)

	checkCmd := exec.Command("docker", "compose", "ps", "--status", "exited", "--status", "restarting", "-q")
	checkCmd.Dir = appDir
	checkOut, _ := checkCmd.Output()

	if len(strings.TrimSpace(string(checkOut))) > 0 {
		logsCmd := exec.Command("docker", "compose", "logs", "--tail=10")
		logsCmd.Dir = appDir
		logsOut, _ := logsCmd.CombinedOutput()

		fmt.Printf("❌ LLM App '%s' crashed! Rolling back...\n", manifest.Name)
		exec.Command("docker", "compose", "down").Run()

		return fmt.Errorf("Container crashed immediately. Reason:\n%s", string(logsOut))
	}

	fmt.Printf("✅ LLM App '%s' successfully deployed!\n", manifest.Name)
	return nil
}

// UninstallLLM tears down an AI app and network, optionally wiping all model data and volumes.
func UninstallLLM(db *sql.DB, appID string, isWiped bool) error {
	appDir := filepath.Join("/var/lib/claviger/llms", appID)

	// Check if directory actually exists
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		return fmt.Errorf("app directory does not exist: %s", appID)
	}

	fmt.Printf("🛑 Tearing down LLM App: %s (Wipe Data: %v)\n", appID, isWiped)

	// Stop and remove the Docker containers safely
	cmd := exec.Command("docker", "compose", "down")
	cmd.Dir = appDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Even if docker compose encounters an issue (e.g. broken compose file),
		// we can still proceed with file cleanup if the user requested a hard wipe.
		fmt.Printf("⚠️ Warning during compose down: %s\n", string(output))
	}

	// Remove port mapping from database so it can be safely reused later
	// storage.DeleteConfig(db, fmt.Sprintf("llm_%s_port", appID))

	if isWiped {
		// 🚨 DANGER: Completely delete the app directory, including all subfolders
		// like ./ollama-data or ./open-webui-data containing models and chat logs.
		if err := os.RemoveAll(appDir); err != nil {
			return fmt.Errorf("failed to completely wipe app directory: %w", err)
		}
		fmt.Printf("🗑️ App '%s' and ALL ITS MODEL DATA successfully wiped from disk.\n", appID)
	} else {
		// Safe Mode: Remove the compose file, but keep the data directory intact
		os.Remove(filepath.Join(appDir, "docker-compose.yml"))
		fmt.Printf("🗑️ App '%s' containers removed. Model data and volumes preserved.\n", appID)
	}

	return nil
}
