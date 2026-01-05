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
	APIKeys            []string
	ModelType          string
	ModelName          string
	EmbeddingModel     string
	EmbeddingDim       int
	EmbeddingBatchSize int
	UseBatchEmbedding  bool
	PostgresURL        string
	TargetHour         int
	PortNumber         int
}

func LoadConfig() (*Config, error) {

	// load env variable
	_ = godotenv.Load()

	modelType := os.Getenv("MODEL_TYPE")
	modelName := os.Getenv("MODEL_NAME")
	embeddingModel := os.Getenv("EMBEDDING_MODEL_NAME")
	embeddingDim, err := strconv.Atoi(os.Getenv("EMBEDDING_DIM"))
	if err != nil {
		embeddingDim = 3072
		log.Println("Invalid or missing EMBEDDING_DIM, using 3072 as default")
	}
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
	portNumber, err := strconv.Atoi(os.Getenv("PORT_NUMBER"))
	if err != nil {
		portNumber = 8000
		log.Println("Invalid port number, using 8000 as default")
	}

	cfg := &Config{
		ModelType:          modelType,
		ModelName:          modelName,
		PostgresURL:        postgresURL,
		EmbeddingModel:     embeddingModel,
		EmbeddingDim:       embeddingDim,
		EmbeddingBatchSize: embeddingBatchSize,
		UseBatchEmbedding:  useBatchEmbedding,
		TargetHour:         targetHour,
		PortNumber:         portNumber,
	}

	keyMapping := map[string]string{
		"gemini": "GEMINI_API_KEY",
		"openai": "OPENAI_API_KEY",
	}
	apiKey, ok := keyMapping[cfg.ModelType]
	if !ok {
		return nil, fmt.Errorf("Unsupported model type %s", cfg.ModelType)
	}

	// set API keys to config object
	cfg.APIKeys = strings.Split(os.Getenv(apiKey), ",")
	if err = cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	// validate API keys
	for _, apiKey := range c.APIKeys {
		if apiKey == "" {
			return fmt.Errorf("API key is required for model type %s", c.ModelType)
		}
	}
	// validate model name
	if c.ModelName == "" {
		return fmt.Errorf("MODEL_NAME must be specified")
	}
	return nil
}
