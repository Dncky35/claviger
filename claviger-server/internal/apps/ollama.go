package apps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

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
