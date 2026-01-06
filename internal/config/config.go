package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	// Shared API keys (used by both generation and embedding if same provider)
	GeminiAPIKeys []string
	OpenAIAPIKeys []string

	// Generation model configurations
	GenerationModelType string
	GenerationModelName string

	// Embedding model configurations
	EmbeddingModelType string
	EmbeddingModelName string
	EmbeddingDim       int
	EmbeddingBatchSize int
	UseBatchEmbedding  bool

	PostgresURL string
	TargetHour  int
	// PortNumber  int
}

// GetAPIKeys returns the API keys for the given provider type
func (c *Config) GetAPIKeys(providerType string) []string {
	switch providerType {
	case "gemini":
		return c.GeminiAPIKeys
	case "openai":
		return c.OpenAIAPIKeys
	default:
		return []string{}
	}
}

func LoadConfig() (*Config, error) {

	// load env variable
	_ = godotenv.Load()

	// Load generation model settings
	generationModelType := os.Getenv("GENERATION_MODEL_TYPE")
	generationModelName := os.Getenv("GENERATION_MODEL_NAME")

	// Load embedding model settings
	embeddingModelType := os.Getenv("EMBEDDING_MODEL_TYPE")
	embeddingModelName := os.Getenv("EMBEDDING_MODEL_NAME")

	// Auto-detect embedding dimension based on embedding provider type
	var embeddingDim int
	switch embeddingModelType {
	case "minilm":
		embeddingDim = 384
	case "gemini":
		embeddingDim = 3072
	case "openai":
		embeddingDim = 1536
	default:
		embeddingDim = 384
	}

	log.Printf("Generation model: %s (%s)", generationModelName, generationModelType)
	log.Printf("Embedding model: %s (%s), dimension: %d", embeddingModelName, embeddingModelType, embeddingDim)

	embeddingBatchSize, err := strconv.Atoi(os.Getenv("EMBEDDING_BATCH_SIZE"))
	if err != nil || embeddingBatchSize <= 0 {
		embeddingBatchSize = 100
		log.Println("Invalid or missing EMBEDDING_BATCH_SIZE, using 100 as default")
	}
	useBatchEmbedding := true
	if val := os.Getenv("USE_BATCH_EMBEDDING"); val == "false" || val == "0" {
		useBatchEmbedding = false
	}
	postgresURL := os.Getenv("POSTGRES_URL")
	targetHour, err := strconv.Atoi(os.Getenv("TARGET_HOUR"))
	if err != nil {
		targetHour = 9
		log.Println("Invalid target hour, using 9 as default")
	}

	// portNumber, err := strconv.Atoi(os.Getenv("PORT_NUMBER"))
	// if err != nil {
	// 	portNumber = 8000
	// 	log.Println("Invalid port number, using 8000 as default")
	// }

	// Load API keys for each provider
	geminiKeys := splitKeys(os.Getenv("GEMINI_API_KEYS"))
	openaiKeys := splitKeys(os.Getenv("OPENAI_API_KEYS"))

	cfg := &Config{
		// Shared API keys
		GeminiAPIKeys: geminiKeys,
		OpenAIAPIKeys: openaiKeys,

		// Generation settings
		GenerationModelType: generationModelType,
		GenerationModelName: generationModelName,

		// Embedding settings
		EmbeddingModelType: embeddingModelType,
		EmbeddingModelName: embeddingModelName,
		EmbeddingDim:       embeddingDim,
		EmbeddingBatchSize: embeddingBatchSize,
		UseBatchEmbedding:  useBatchEmbedding,

		PostgresURL: postgresURL,
		TargetHour:  targetHour,
		// PortNumber:  portNumber,
	}

	if err = cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// splitKeys splits comma-separated API keys and filters empty strings
func splitKeys(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	keys := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			keys = append(keys, p)
		}
	}
	return keys
}

func (c *Config) Validate() error {
	// Validate generation model
	if c.GenerationModelType != "minilm" {
		if len(c.GetAPIKeys(c.GenerationModelType)) == 0 {
			return fmt.Errorf("API keys required for generation model type %s", c.GenerationModelType)
		}
	}
	if c.GenerationModelName == "" {
		return fmt.Errorf("GENERATION_MODEL_NAME must be specified")
	}

	// Validate embedding model
	if c.EmbeddingModelType != "minilm" {
		if len(c.GetAPIKeys(c.EmbeddingModelType)) == 0 {
			return fmt.Errorf("API keys required for embedding model type %s", c.EmbeddingModelType)
		}
	}
	if c.EmbeddingModelName == "" {
		return fmt.Errorf("EMBEDDING_MODEL_NAME must be specified")
	}

	return nil
}
