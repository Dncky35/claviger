package apps

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

// VLLMAdapter handles the enterprise vLLM engine via OpenAI API specs and Docker orchestration
type VLLMAdapter struct {
	Port   string
	AppDir string // The directory containing the docker-compose.yml (e.g., /var/lib/claviger/llms/vllm)
}

// ListModels uses vLLM's OpenAI-compatible endpoint
func (v *VLLMAdapter) ListModels() ([]UnifiedModel, error) {
	if v.Port == "" || v.Port == "0" {
		return nil, fmt.Errorf("vLLM port is not configured or engine is offline")
	}

	url := fmt.Sprintf("http://127.0.0.1:%s/v1/models", v.Port)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to vLLM engine: %w", err)
	}
	defer resp.Body.Close()

	var aiResp OpenAIModelResponse // Reusing the struct we made for LocalAI
	if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil {
		return nil, fmt.Errorf("failed to decode vLLM response: %w", err)
	}

	var unified []UnifiedModel
	for _, m := range aiResp.Data {
		unified = append(unified, UnifiedModel{
			ID:           m.ID,
			Name:         m.ID,
			Engine:       "vllm",
			SizeGB:       0.0,
			Params:       "Unknown",
			Quantization: "FP16/AWQ", // vLLM typically runs unquantized or AWQ/GPTQ
		})
	}

	return unified, nil
}

// PullModel writes the target model to a .env file and restarts the vLLM container
func (v *VLLMAdapter) PullModel(modelID string) error {
	// 1. Write the target model to a local .env file
	envPath := filepath.Join(v.AppDir, ".env")
	envContent := fmt.Sprintf("VLLM_TARGET_MODEL=%s\n", modelID)

	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		return fmt.Errorf("failed to write vLLM environment configuration: %w", err)
	}

	// 2. Restart the container so it picks up the new environment variable
	// We use `up -d` instead of `restart` so Docker fully evaluates the new .env file
	cmd := exec.Command("docker", "compose", "up", "-d")
	cmd.Dir = v.AppDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restart vLLM container: %s\nError: %w", string(output), err)
	}

	return nil
}

// DeleteModel is intentionally restricted for vLLM
func (v *VLLMAdapter) DeleteModel(modelID string) error {
	// vLLM manages downloads internally via the HuggingFace Hub cache structure (blobs/snapshots).
	// Deleting a single model programmatically without corrupting the cache index is unsafe.
	return fmt.Errorf("vLLM models are managed via HuggingFace cache and cannot be deleted via the API. Please clear the vllm-data volume manually if space is needed")
}
