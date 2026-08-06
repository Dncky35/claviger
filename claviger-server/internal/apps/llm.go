package apps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type UnifiedModel struct {
	ID           string  `json:"id"`           // The raw identifier (e.g., "llama3.1:latest", "meta-llama/Llama-3-8b")
	Name         string  `json:"name"`         // Clean UI name (e.g., "Llama 3.1")
	Engine       string  `json:"engine"`       // "ollama", "localai", or "vllm"
	SizeGB       float64 `json:"size_gb"`      // Normalized to Gigabytes
	Params       string  `json:"params"`       // e.g., "8B" (If the engine provides it)
	Quantization string  `json:"quantization"` // e.g., "Q4_0" (If the engine provides it)
}

// PullRequest handles the incoming request from the UI modal
type PullRequest struct {
	Engine  string `json:"engine"`
	ModelID string `json:"model_id"` // What the user typed in the input box
}

// EngineAdapter defines the contract for any AI backend managed by Claviger
type EngineAdapter interface {
	// ListModels queries the engine and returns the unified format
	ListModels() ([]UnifiedModel, error)

	// PullModel triggers a download.
	// Note: For vLLM, this might modify the docker-compose.yml and restart the container.
	PullModel(modelID string) error

	// DeleteModel removes the model from disk
	DeleteModel(modelID string) error
}

func GetAdapter(engineName string) (EngineAdapter, error) {
	switch engineName {
	case "ollama":
		return &OllamaAdapter{}, nil // We will build this struct next!
	// case "localai":
	// 	return &LocalAIAdapter{}, nil
	// case "vllm":
	// 	return &VLLMAdapter{}, nil
	default:
		return nil, fmt.Errorf("unsupported AI engine: %s", engineName)
	}
}

type OllamaModelResponse struct {
	Models []struct {
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		Details struct {
			ParameterSize     string `json:"parameter_size"`
			QuantizationLevel string `json:"quantization_level"`
		} `json:"details"`
	} `json:"models"`
}

// OllamaPullRequest is the payload Ollama expects for POST http://localhost:11434/api/pull
type OllamaPullRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"` // Set to false so we wait for completion (or true later for streaming)
}

type OllamaAdapter struct{}

func (o *OllamaAdapter) ListModels() ([]UnifiedModel, error) {
	resp, err := http.Get("http://127.0.0.1:11434/api/tags")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ollama engine: %w", err)
	}
	defer resp.Body.Close()

	var ollamaResp OllamaModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode Ollama response: %w", err)
	}

	var unified []UnifiedModel
	for _, m := range ollamaResp.Models {
		// Convert bytes to GB with clean float formatting
		sizeGB := float64(m.Size) / (1024 * 1024 * 1024)

		unified = append(unified, UnifiedModel{
			ID:           m.Name,
			Name:         m.Name, // Ollama names are already clean (e.g. "llama3.1:latest")
			Engine:       "ollama",
			SizeGB:       sizeGB,
			Params:       m.Details.ParameterSize,
			Quantization: m.Details.QuantizationLevel,
		})
	}

	return unified, nil
}

// PullModel instructs Ollama to download a new model from the library
func (o *OllamaAdapter) PullModel(modelID string) error {
	payload := OllamaPullRequest{
		Model:  modelID,
		Stream: false, // Keep it simple: wait until download finishes
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post("http://127.0.0.1:11434/api/pull", "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to trigger model pull on Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned error status: %d", resp.StatusCode)
	}

	return nil
}

// DeleteModel removes a model from Ollama's local storage
func (o *OllamaAdapter) DeleteModel(modelID string) error {
	// Ollama uses a custom DELETE request with a JSON body
	payload := map[string]string{"model": modelID}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodDelete, "http://127.0.0.1:11434/api/delete", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send delete request to Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete model, status code: %d", resp.StatusCode)
	}

	return nil
}
