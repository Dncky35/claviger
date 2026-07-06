package apps

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type CustomAppPayload struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Icon             string `json:"icon"`
	NeedsDynamicPort bool   `json:"needs_dynamic_port"`
	HasCustomSetup   bool   `json:"has_custom_setup"`
	SetupPort        int    `json:"setup_port"`
	ComposeYAML      string `json:"compose_yaml"`
}

// ValidationError represents a specific security rule violation
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// ValidateZeroTrustYAML enforces Claviger's network and port security rules
func ValidateZeroTrustYAML(payload CustomAppPayload) []ValidationError {
	var errors []ValidationError
	yamlStr := payload.ComposeYAML

	// 1. Basic YAML Syntax Check
	var dummy map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &dummy); err != nil {
		errors = append(errors, ValidationError{
			Field:   "compose_yaml",
			Code:    "INVALID_YAML",
			Message: "The provided YAML is invalid or malformed.",
		})
		return errors // Stop here if it's not even valid YAML
	}

	// 2. Network Isolation Check
	if !strings.Contains(yamlStr, "cloudrocean-net") {
		errors = append(errors, ValidationError{
			Field:   "compose_yaml",
			Code:    "MISSING_NETWORK",
			Message: "Security Alert: App must be attached to 'cloudrocean-net' to remain behind the VPN firewall.",
		})
	}

	// 3. UI Tracking Label Check
	if !strings.Contains(yamlStr, "claviger.app=") {
		errors = append(errors, ValidationError{
			Field:   "compose_yaml",
			Code:    "MISSING_LABEL",
			Message: "Missing Label: Add a 'claviger.app=your-app-name' label so the dashboard can track its status.",
		})
	}

	// 4. Fatal Port Conflicts (Master Gateway Protection)
	// We must prevent binding to 80, 443, or 22
	forbiddenPorts := []string{`"80:`, ` 80:`, `"443:`, ` 443:`, `"22:`, ` 22:`}
	for _, port := range forbiddenPorts {
		if strings.Contains(yamlStr, port) {
			errors = append(errors, ValidationError{
				Field:   "compose_yaml",
				Code:    "PORT_CONFLICT",
				Message: "Fatal: You cannot bind to ports 80, 443, or 22. These are reserved for the Master Gateway.",
			})
			break
		}
	}

	// 5. Dynamic Port Variable Check
	if payload.NeedsDynamicPort && !strings.Contains(yamlStr, "{{.DynamicPort}}") {
		errors = append(errors, ValidationError{
			Field:   "compose_yaml",
			Code:    "MISSING_PORT_VAR",
			Message: "You enabled 'Needs Dynamic Port' but forgot to add the '{{.DynamicPort}}' variable to your ports array.",
		})
	}

	return errors
}

func AutoCorrectZeroTrustYAML(payload CustomAppPayload) (string, error) {
	yamlStr := payload.ComposeYAML
	appName := payload.Name
	hasCustomSetup := payload.HasCustomSetup
	setupPort := payload.SetupPort
	needsDynamicPort := payload.NeedsDynamicPort

	var compose map[string]interface{}
	err := yaml.Unmarshal([]byte(yamlStr), &compose)
	if err != nil {
		return yamlStr, err
	}

	// Generate a safe label name from the App Name
	safeAppName := strings.ReplaceAll(strings.ToLower(appName), " ", "-")
	labelStr := fmt.Sprintf("claviger.app=%s", safeAppName)
	labelKey := "claviger.app"
	labelVal := safeAppName

	// 1. Ensure 'networks' exists at the root level safely
	if compose["networks"] == nil {
		compose["networks"] = map[string]interface{}{
			"cloudrocean-net": map[string]interface{}{"external": true},
		}
	} else {
		switch nets := compose["networks"].(type) {
		case map[string]interface{}:
			nets["cloudrocean-net"] = map[string]interface{}{"external": true}
		case map[interface{}]interface{}:
			nets["cloudrocean-net"] = map[string]interface{}{"external": true}
		}
	}

	// Track if we've successfully mapped our requested ports across any service
	dynamicPortInjected := false
	setupPortInjected := false

	// 2. Normalize and Iterate over all 'services' safely
	var servicesMap map[string]interface{}
	switch s := compose["services"].(type) {
	case map[string]interface{}:
		servicesMap = s
	case map[interface{}]interface{}:
		servicesMap = make(map[string]interface{})
		for k, v := range s {
			servicesMap[fmt.Sprintf("%v", k)] = v
		}
		compose["services"] = servicesMap
	}

	for svcName, svcRaw := range servicesMap {
		var svc map[string]interface{}

		switch s := svcRaw.(type) {
		case map[string]interface{}:
			svc = s
		case map[interface{}]interface{}:
			svc = make(map[string]interface{})
			for k, v := range s {
				svc[fmt.Sprintf("%v", k)] = v
			}
			servicesMap[svcName] = svc // Update the reference
		default:
			continue // Skip if the service block is malformed
		}

		// --- Inject Network ---
		if svc["networks"] == nil {
			svc["networks"] = []string{"cloudrocean-net"}
		} else {
			switch nets := svc["networks"].(type) {
			case []interface{}:
				hasNet := false
				for _, n := range nets {
					if str, ok := n.(string); ok && str == "cloudrocean-net" {
						hasNet = true
						break
					}
				}
				if !hasNet {
					svc["networks"] = append(nets, "cloudrocean-net")
				}
			case map[string]interface{}:
				if _, exists := nets["cloudrocean-net"]; !exists {
					nets["cloudrocean-net"] = nil
				}
			case map[interface{}]interface{}:
				if _, exists := nets["cloudrocean-net"]; !exists {
					nets["cloudrocean-net"] = nil
				}
			}
		}

		// --- Inject Label ---
		if svc["labels"] == nil {
			svc["labels"] = []string{labelStr}
		} else {
			switch labels := svc["labels"].(type) {
			case []interface{}:
				hasLabel := false
				for _, l := range labels {
					if str, ok := l.(string); ok && strings.HasPrefix(str, "claviger.app=") {
						hasLabel = true
						break
					}
				}
				if !hasLabel {
					svc["labels"] = append(labels, labelStr)
				}
			case map[string]interface{}:
				labels[labelKey] = labelVal
			case map[interface{}]interface{}:
				labels[labelKey] = labelVal
			}
		}

		// --- Inject / Correct Ports (Your Dynamic Logic) ---
		if svc["ports"] != nil {
			if ports, ok := svc["ports"].([]interface{}); ok {
				var correctedPorts []interface{}

				for _, p := range ports {
					pStr := fmt.Sprintf("%v", p)

					// Skip if they already manually typed the correct Zero Trust variables
					if strings.Contains(pStr, "{{.DynamicPort}}") {
						dynamicPortInjected = true
						correctedPorts = append(correctedPorts, pStr)
						continue
					}
					if setupPort > 0 && strings.HasPrefix(pStr, fmt.Sprintf("%d:", setupPort)) {
						setupPortInjected = true
						correctedPorts = append(correctedPorts, pStr)
						continue
					}

					// Auto-replace standard host ports with Claviger variables
					parts := strings.Split(pStr, ":")
					if len(parts) >= 2 {
						containerPart := parts[len(parts)-1]

						isCommonWebPort := strings.HasPrefix(pStr, "80:") ||
							strings.HasPrefix(pStr, "8080:") ||
							strings.HasPrefix(pStr, "443:") ||
							strings.HasPrefix(pStr, "9000:") ||
							strings.HasPrefix(pStr, "3000:")

						if needsDynamicPort && !dynamicPortInjected && isCommonWebPort {
							pStr = fmt.Sprintf("{{.DynamicPort}}:%s", containerPart)
							dynamicPortInjected = true
						} else if hasCustomSetup && setupPort > 0 && !setupPortInjected && !strings.Contains(pStr, "53") {
							pStr = fmt.Sprintf("%d:%s", setupPort, containerPart)
							setupPortInjected = true
						} else if needsDynamicPort && !dynamicPortInjected && !hasCustomSetup && !strings.Contains(pStr, "53") {
							pStr = fmt.Sprintf("{{.DynamicPort}}:%s", containerPart)
							dynamicPortInjected = true
						}
					}
					correctedPorts = append(correctedPorts, pStr)
				}

				// 🛡️ FALLBACK 1: They had a ports array, but our heuristic missed it.
				// Force inject it to pass validation.
				if needsDynamicPort && !dynamicPortInjected {
					correctedPorts = append(correctedPorts, "{{.DynamicPort}}:80")
					dynamicPortInjected = true
				}

				svc["ports"] = correctedPorts
			}
		} else {
			// 🛡️ FALLBACK 2: The YAML had NO ports block at all (e.g. Stirling PDF)
			// We dynamically generate the ports block from scratch!
			var newPorts []interface{}

			if needsDynamicPort && !dynamicPortInjected {
				// 8080 is the standard internal port for most port-less web containers
				newPorts = append(newPorts, "{{.DynamicPort}}:8080")
				dynamicPortInjected = true
			}

			if hasCustomSetup && setupPort > 0 && !setupPortInjected {
				newPorts = append(newPorts, fmt.Sprintf("%d:3000", setupPort))
				setupPortInjected = true
			}

			// Inject the newly created block into the service
			if len(newPorts) > 0 {
				svc["ports"] = newPorts
			}
		}
	}

	// 3. Marshal it back to YAML
	outBytes, err := yaml.Marshal(&compose)
	if err != nil {
		return yamlStr, err
	}

	return string(outBytes), nil
}
