package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	APIKey     string
	ModelType  string
	ModelName  string
	PortNumber int
}

func LoadConfig() (*Config, error) {

	// load env variable
	err := godotenv.Load()
	if err != nil {
		return nil, err
	}

	modelType := os.Getenv("MODEL_TYPE")
	modelName := os.Getenv("MODEL_NAME")
	portNumber, err := strconv.Atoi(os.Getenv("PORT_NUMBER"))
	if err != nil {
		portNumber = 8000
		log.Println("Invalid port number, using 8000 as default")
	}

	cfg := &Config{
		ModelType:  modelType,
		ModelName:  modelName,
		PortNumber: portNumber,
	}

	keyMapping := map[string]string{
		"gemini": "GEMINI_API_KEY",
		"openai": "OPENAI_API_KEY",
	}
	apiKey, ok := keyMapping[cfg.ModelType]
	if !ok {
		return nil, fmt.Errorf("Unsupported model type %s", cfg.ModelType)
	}

	// set apiKey to config object
	cfg.APIKey = os.Getenv(apiKey)

	err = cfg.Validate()
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("API key is required for model type %s", c.ModelType)
	}
	if c.ModelName == "" {
		return fmt.Errorf("MODEL_NAME must be specified")
	}
	return nil
}
