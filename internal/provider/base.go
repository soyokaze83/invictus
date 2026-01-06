package provider

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/soyokaze83/invictus/internal/domain"
)

type ProviderType string

const (
	ProviderGemini ProviderType = "gemini"
	ProviderOpenAI ProviderType = "openai"
	ProviderMiniLM ProviderType = "minilm"
)

type BaseProvider struct {
	modelName    string
	apiKeys      []string
	currAPIIndex int
	mu           sync.RWMutex

	// Functions to open and close the client
	createClient func(ctx context.Context, apiKey string) error
	closeClient  func() error
}

type LLMProvider interface {

	// Generation functions
	Generate(ctx context.Context, prompt string) (*domain.LLMResponse, error)
	StreamGenerate(ctx context.Context, promtp string) (<-chan string, error)

	// Embedding functions (single)
	EmbedWithRetry(ctx context.Context, content string, maxRetries int) ([]float32, error)
	Embed(ctx context.Context, text string) ([]float32, error)

	// Embedding functions (batch)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	EmbedBatchWithRetry(ctx context.Context, texts []string, maxRetries int, batchSize int) ([][]float32, error)

	// Client functions
	Close() error
}

func NewProvider(ctx context.Context, providerType ProviderType, modelName string, apiKeys []string) (LLMProvider, error) {
	switch providerType {
	case ProviderGemini:
		provider, err := NewGeminiProvider(ctx, modelName, apiKeys)
		if err != nil {
			return nil, err
		}
		return provider, nil
	case ProviderOpenAI:
		provider, err := NewOpenAIProvider(ctx, modelName, apiKeys)
		if err != nil {
			return nil, err
		}
		return provider, nil
	case ProviderMiniLM:
		// modelName is used as runtime path for minilm (e.g., "libonnxruntime.so")
		provider, err := NewMiniLMProvider(ctx, modelName)
		if err != nil {
			return nil, err
		}
		return provider, nil
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

func (p *BaseProvider) rotateClient(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.closeClient(); err != nil {
		slog.Error("Failed to close client", "error", err)
		return err
	}

	p.currAPIIndex = (p.currAPIIndex + 1) % len(p.apiKeys)
	if err := p.createClient(ctx, p.apiKeys[p.currAPIIndex]); err != nil {
		slog.Error("Failed to create client", "error", err)
		return err
	}

	return nil
}

func (p *BaseProvider) validate() error {
	if p.modelName == "" {
		return fmt.Errorf("Model name is required")
	}
	if len(p.apiKeys) == 0 {
		return fmt.Errorf("At least one API key is required")
	}
	return nil
}
