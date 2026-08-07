package apps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type OpenAIModelResponse struct {
	Data []struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

// LocalAIPullRequest is the payload for POST /models/apply (LocalAI's gallery downloader)
type LocalAIPullRequest struct {
	ID string `json:"id"`
}

type LocalAIAdapter struct {
	Port string // Injected by the Factory (e.g., "18082")
}

// ListModels hits the OpenAI-compatible /v1/models endpoint
func (l *LocalAIAdapter) ListModels() ([]UnifiedModel, error) {
	if l.Port == "" || l.Port == "0" {
		return nil, fmt.Errorf("localai port is not configured or engine is offline")
	}

	url := fmt.Sprintf("http://127.0.0.1:%s/v1/models", l.Port)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to LocalAI engine: %w", err)
	}
	defer resp.Body.Close()

	var aiResp OpenAIModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil {
		return nil, fmt.Errorf("failed to decode LocalAI response: %w", err)
	}

	var unified []UnifiedModel
	for _, m := range aiResp.Data {
		// Note: The standard OpenAI /v1/models API does not return file sizes or
		// parameter counts (like "8B"). We pass safe defaults so the UI doesn't break.
		unified = append(unified, UnifiedModel{
			ID:           m.ID,
			Name:         m.ID,
			Engine:       "localai",
			SizeGB:       0.0,       // Unavailable via standard OpenAI API
			Params:       "Unknown", // Unavailable via standard OpenAI API
			Quantization: "N/A",
		})
	}

	return unified, nil
}

// PullModel instructs LocalAI to download a model from its community gallery
func (l *LocalAIAdapter) PullModel(modelID string) error {
	payload := LocalAIPullRequest{
		ID: modelID,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://127.0.0.1:%s/models/apply", l.Port)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to trigger model pull on LocalAI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("localai returned error status: %d", resp.StatusCode)
	}

	return nil
}

// DeleteModel removes a model's YAML and binary files from LocalAI's storage
func (l *LocalAIAdapter) DeleteModel(modelID string) error {
	url := fmt.Sprintf("http://127.0.0.1:%s/models/%s", l.Port, modelID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send delete request to LocalAI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete model, status code: %d", resp.StatusCode)
	}

	return nil
}
