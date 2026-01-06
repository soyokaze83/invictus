package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/soyokaze83/invictus/internal/domain"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type GeminiProvider struct {
	BaseProvider
	client *genai.Client
	mu     sync.RWMutex
}

func NewGeminiProvider(ctx context.Context, modelName string, apiKeys []string) (*GeminiProvider, error) {

	newProvider := &GeminiProvider{
		BaseProvider: BaseProvider{
			modelName:    modelName,
			apiKeys:      apiKeys,
			currAPIIndex: 0,
		},
	}

	if err := newProvider.validate(); err != nil {
		return nil, fmt.Errorf("Failed validation on LLM client")
	}

	newProvider.createClient = func(ctx context.Context, apiKey string) error {
		client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
		if err != nil {
			return err
		}
		newProvider.client = client
		return nil
	}

	newProvider.closeClient = func() error {
		if newProvider.client != nil {
			return newProvider.client.Close()
		}
		return nil
	}

	// Create initial client
	if err := newProvider.createClient(ctx, apiKeys[0]); err != nil {
		return nil, fmt.Errorf("Error creating initial client")
	}

	return newProvider, nil
}

func (p *GeminiProvider) Generate(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	p.mu.RLock()
	client := p.client
	p.mu.RUnlock()

	model := client.GenerativeModel(p.modelName)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, err
	}

	// Extract text from response
	var text string
	for _, candidate := range resp.Candidates {
		for _, part := range candidate.Content.Parts {
			if t, ok := part.(genai.Text); ok {
				text += string(t)
			}
		}
	}

	return &domain.LLMResponse{Content: text}, nil
}

func (p *GeminiProvider) StreamGenerate(ctx context.Context, prompt string) (<-chan string, error) {
	p.mu.RLock()
	client := p.client
	p.mu.RUnlock()

	model := client.GenerativeModel(p.modelName)
	iter := model.GenerateContentStream(ctx, genai.Text(prompt))

	ch := make(chan string)
	go func() {
		defer close(ch)
		for {
			resp, err := iter.Next()
			if err != nil {
				return // iterator exhausted or error
			}
			for _, candidate := range resp.Candidates {
				for _, part := range candidate.Content.Parts {
					if t, ok := part.(genai.Text); ok {
						ch <- string(t)
					}
				}
			}
		}
	}()

	return ch, nil
}

func (p *GeminiProvider) EmbedWithRetry(ctx context.Context, text string, maxRetries int) ([]float32, error) {
	var apiErr *googleapi.Error

	for i := range maxRetries {
		embedding, err := p.Embed(ctx, text)
		if err == nil {
			return embedding, nil
		}

		// Check for rate limited error & rotate API key
		if errors.As(err, &apiErr) && apiErr.Code == 429 {
			slog.Info("Rate limited, rotating API key", "attempt", i+1)
			if err := p.rotateClient(ctx); err != nil {
				return nil, err
			}
			time.Sleep(time.Duration(1<<i) * time.Second)
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("Exhausted amount of retries")
}

func (p *GeminiProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	p.mu.RLock()
	client := p.client
	p.mu.RUnlock()

	em := client.EmbeddingModel("gemini-embedding-001")
	res, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, err
	}

	return res.Embedding.Values, nil
}

func (p *GeminiProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	p.mu.RLock()
	client := p.client
	p.mu.RUnlock()

	em := client.EmbeddingModel("gemini-embedding-001")

	// Build batch request
	batch := em.NewBatch()
	for _, text := range texts {
		batch.AddContent(genai.Text(text))
	}

	res, err := em.BatchEmbedContents(ctx, batch)
	if err != nil {
		return nil, fmt.Errorf("batch embed failed: %w", err)
	}

	embeddings := make([][]float32, len(res.Embeddings))
	for i, emb := range res.Embeddings {
		embeddings[i] = emb.Values
	}

	return embeddings, nil
}

func (p *GeminiProvider) EmbedBatchWithRetry(ctx context.Context, texts []string, maxRetries int, batchSize int) ([][]float32, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	// Process texts in chunks
	var allEmbeddings [][]float32
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		chunk := texts[start:end]

		var apiErr *googleapi.Error
		var chunkEmbeddings [][]float32
		var lastErr error

		for i := range maxRetries {
			chunkEmbeddings, lastErr = p.EmbedBatch(ctx, chunk)
			if lastErr == nil {
				break
			}

			// Check for rate limited error & rotate API key
			if errors.As(lastErr, &apiErr) && apiErr.Code == 429 {
				slog.Info("Rate limited on batch embed, rotating API key", "attempt", i+1, "chunk", start/batchSize+1)
				if err := p.rotateClient(ctx); err != nil {
					return nil, err
				}
				time.Sleep(time.Duration(1<<i) * time.Second)
				continue
			}

			return nil, lastErr
		}

		if lastErr != nil {
			return nil, fmt.Errorf("exhausted retries for batch embed on chunk %d: %w", start/batchSize+1, lastErr)
		}

		allEmbeddings = append(allEmbeddings, chunkEmbeddings...)
		slog.Info("Embedded chunk", "chunk", start/batchSize+1, "size", len(chunk))
	}

	return allEmbeddings, nil
}

func (p *GeminiProvider) Close() error {
	return p.closeClient()
}
