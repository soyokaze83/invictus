package provider

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/soyokaze83/invictus/internal/domain"
)

type OpenAIProvider struct {
	BaseProvider
	client openai.Client
	mu     sync.RWMutex
}

func NewOpenAIProvider(ctx context.Context, modelName string, apiKeys []string) (*OpenAIProvider, error) {

	newProvider := &OpenAIProvider{
		BaseProvider: BaseProvider{
			modelName:    modelName,
			apiKeys:      apiKeys,
			currAPIIndex: 0,
		},
	}

	if err := newProvider.validate(); err != nil {
		slog.Error("Failed to pass provider validation", "error", err)
		return nil, fmt.Errorf("Failed validation LLM client!")
	}

	newProvider.createClient = func(ctx context.Context, apiKey string) error {
		client := openai.NewClient(option.WithAPIKey(apiKey))
		newProvider.client = client
		return nil
	}

	// OpenAI client does not have a closing functionality
	newProvider.closeClient = func() error {
		return nil
	}

	// Create initial client
	if err := newProvider.createClient(ctx, apiKeys[0]); err != nil {
		return nil, fmt.Errorf("Error creating initial client.")
	}

	return newProvider, nil
}

func (p *OpenAIProvider) Generate(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return nil, fmt.Errorf("Unimplemented")
}

func (p *OpenAIProvider) StreamGenerate(ctx context.Context, prompt string) (<-chan string, error) {
	return nil, fmt.Errorf("Unimplemented")
}

func (p *OpenAIProvider) EmbedWithRetry(ctx context.Context, content string, maxRetries int) ([]float32, error) {
	return nil, fmt.Errorf("Unimplemented")
}

func (p *OpenAIProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("Unimplemented")
}

func (p *OpenAIProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("Unimplemented")
}

func (p *OpenAIProvider) EmbedBatchWithRetry(ctx context.Context, texts []string, maxRetries int, batchSize int) ([][]float32, error) {
	return nil, fmt.Errorf("Unimplemented")
}

func (p *OpenAIProvider) Close() error {
	return p.closeClient()
}
