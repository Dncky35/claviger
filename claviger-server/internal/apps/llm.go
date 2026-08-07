package apps

import (
	"claviger-server/storage"
	"database/sql"
	"fmt"
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

func GetAdapter(engineName string, db *sql.DB) (EngineAdapter, error) {
	switch engineName {
	case "ollama":
		// Ollama is static, no dynamic port needed
		return &OllamaAdapter{}, nil

	case "localai":
		// Fetch the assigned port from SQLite
		portStr := storage.GetConfig(db, "llm_localai_port")
		if portStr == "" {
			return nil, fmt.Errorf("localai is not installed or port is missing")
		}
		return &LocalAIAdapter{Port: portStr}, nil

	case "vllm":
		portStr := storage.GetConfig(db, "llm_vllm_port")
		if portStr == "" {
			return nil, fmt.Errorf("vllm is offline or missing port mapping")
		}
		return &VLLMAdapter{
			Port:   portStr,
			AppDir: "/var/lib/claviger/llms/vllm", // Injected for Compose restarts
		}, nil
	default:
		return nil, fmt.Errorf("unsupported AI engine: %s", engineName)
	}
}

// OpenAIModelResponse matches the JSON returned by GET /v1/models (LocalAI uses this standard)
