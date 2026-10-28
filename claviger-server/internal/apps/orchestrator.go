package apps

import (
	"bytes"
	"claviger-server/internal/types"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func GetActionFromSchema(appID string, fieldID string) (types.Action, error) {
	// Adjust this base path to match your Claviger apps directory
	baseAppsDir := "/var/lib/claviger/apps"
	schemaPath := filepath.Join(baseAppsDir, appID, "schema.json")

	// 1. Read the schema file from disk
	fileData, err := os.ReadFile(schemaPath)
	if err != nil {
		return types.Action{}, fmt.Errorf("could not read schema file at %s: %w", schemaPath, err)
	}

	// 2. Parse the JSON
	var schema types.AppSchema
	if err := json.Unmarshal(fileData, &schema); err != nil {
		return types.Action{}, fmt.Errorf("failed to parse schema JSON for app %s: %w", appID, err)
	}

	// 3. Find the field ID the frontend requested
	for _, category := range schema.Categories {
		for _, field := range category.Fields {
			if field.ID == fieldID {
				return field.Action, nil
			}
		}
	}

	return types.Action{}, fmt.Errorf("field ID [%s] not found in schema for app [%s]", fieldID, appID)
}

// ExecuteAppAction routes the incoming change to the correct handler based on the schema.
func ExecuteAppAction(appDir string, action types.Action, rawValue string) error {

	// 1. Map the value if a value_mapping exists (e.g., boolean "true" -> string "true")
	mappedValue := rawValue
	if val, exists := action.ValueMapping[rawValue]; exists {
		mappedValue = val
	}

	// 2. Route to the correct execution handler
	var err error
	switch action.Type {
	case "env":
		err = HandleEnvUpdate(appDir, action.TargetKey, mappedValue)
	case "exec":
		err = HandleDockerExec(action, mappedValue)
	case "api":
		// TODO: Implement NPM/AdGuard API bridge
		return fmt.Errorf("api execution not yet implemented")
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}

	if err != nil {
		return err
	}

	// 3. Restart the container if the schema demands it
	if action.RequiresRestart {
		return RestartApp(appDir)
	}

	return nil
}

func HandleEnvUpdate(appDir string, targetKey string, newValue string) error {
	envPath := filepath.Join(appDir, ".env")

	// Read existing .env file safely
	content, err := os.ReadFile(envPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read .env file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	updated := false
	var newLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// If we find the key, replace the entire line
		if strings.HasPrefix(trimmed, targetKey+"=") {
			newLines = append(newLines, fmt.Sprintf("%s=%s", targetKey, newValue))
			updated = true
		} else {
			newLines = append(newLines, line)
		}
	}

	// If the key didn't exist, append it
	if !updated {
		newLines = append(newLines, fmt.Sprintf("%s=%s", targetKey, newValue))
	}

	// Write back to the .env file
	finalContent := strings.Join(newLines, "\n")
	return os.WriteFile(envPath, []byte(finalContent), 0644)
}

func RestartApp(appDir string) error {
	// Execute 'docker compose up -d' in the app's directory to apply .env changes
	cmd := exec.Command("docker", "compose", "up", "-d")
	cmd.Dir = appDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose restart failed: %s", stderr.String())
	}
	return nil
}

func HandleDockerExec(action types.Action, newValue string) error {
	if action.ContainerName == "" || action.CommandTemplate == "" {
		return fmt.Errorf("exec action requires container_name and command_template")
	}

	// Replace the placeholder with the actual user value
	finalCommand := strings.ReplaceAll(action.CommandTemplate, "{{value}}", newValue)

	// Construct the docker exec command
	args := []string{"exec", "-i"}
	if action.RunAsUser != "" {
		args = append(args, "-u", action.RunAsUser)
	}
	args = append(args, action.ContainerName)

	// Split the command string into arguments (basic split, consider a shell parser for complex commands)
	cmdArgs := strings.Fields(finalCommand)
	args = append(args, cmdArgs...)

	cmd := exec.Command("docker", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errStr := stderr.String()

		// Catch the specific Nextcloud initialization error
		if strings.Contains(errStr, "Nextcloud is not installed") {
			return fmt.Errorf("app is still initializing. Please open the app in your browser to complete the initial setup first")
		}

		return fmt.Errorf("command failed: %s", errStr)
	}

	return nil
}

func GetHydratedSchema(appID string) (types.AppSchema, error) {
	baseAppsDir := "/var/lib/claviger/apps"
	schemaPath := filepath.Join(baseAppsDir, appID, "schema.json")
	envPath := filepath.Join(baseAppsDir, appID, ".env")

	// 1. Read and parse the schema.json
	fileData, err := os.ReadFile(schemaPath)
	if err != nil {
		return types.AppSchema{}, fmt.Errorf("could not read schema file: %w", err)
	}

	var schema types.AppSchema
	if err := json.Unmarshal(fileData, &schema); err != nil {
		return types.AppSchema{}, fmt.Errorf("failed to parse schema JSON: %w", err)
	}

	// 2. Parse the active .env file into a map for quick lookups
	envMap := parseEnvFile(envPath)

	// 3. Hydrate the CurrentValue for each field
	for i, category := range schema.Categories {
		for j, field := range category.Fields {
			if field.Action.Type == "env" {
				if val, exists := envMap[field.Action.TargetKey]; exists {

					// Reverse the value_mapping (e.g., if .env has "true", map it back to UI boolean)
					actualVal := val
					for uiVal, envVal := range field.Action.ValueMapping {
						if envVal == val {
							actualVal = uiVal
							break
						}
					}

					// Safely assign boolean vs string for the frontend UI renderer
					if field.DataType == "boolean" {
						schema.Categories[i].Fields[j].CurrentValue = (actualVal == "true")
					} else {
						schema.Categories[i].Fields[j].CurrentValue = actualVal
					}
				}
			}
		}
	}

	return schema, nil
}

// parseEnvFile is a simple helper to read a .env file into a Go map
func parseEnvFile(path string) map[string]string {
	envs := make(map[string]string)
	content, err := os.ReadFile(path)
	if err != nil {
		return envs // Return empty map if .env doesn't exist yet
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			envs[parts[0]] = parts[1]
		}
	}
	return envs
}
