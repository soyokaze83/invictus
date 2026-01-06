package provider

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/clems4ever/all-minilm-l6-v2-go/all_minilm_l6_v2"
	"github.com/soyokaze83/invictus/internal/domain"
)

type MiniLMProvider struct {
	model *all_minilm_l6_v2.Model
	mu    sync.RWMutex
}

func NewMiniLMProvider(ctx context.Context, runtimePath string) (*MiniLMProvider, error) {
	opts := []all_minilm_l6_v2.ModelOption{}
	if runtimePath != "" {
		opts = append(opts, all_minilm_l6_v2.WithRuntimePath(runtimePath))
	}

	model, err := all_minilm_l6_v2.NewModel(opts...)
	if err != nil {
		return nil, fmt.Errorf("minilm: failed to load model: %w", err)
	}

	return &MiniLMProvider{model: model}, nil
}

func (p *MiniLMProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.model.Compute(text, false)
}

func (p *MiniLMProvider) EmbedWithRetry(ctx context.Context, content string, maxRetries int) ([]float32, error) {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		embedding, err := p.Embed(ctx, content)
		if err == nil {
			return embedding, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("minilm: exhausted retries: %w", lastErr)
}

func (p *MiniLMProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.model.ComputeBatch(texts, false)
}

func (p *MiniLMProvider) EmbedBatchWithRetry(ctx context.Context, texts []string, maxRetries int, batchSize int) ([][]float32, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	var allEmbeddings [][]float32
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		chunk := texts[start:end]

		embeddings, err := p.EmbedBatch(ctx, chunk)
		if err != nil {
			return nil, fmt.Errorf("minilm: batch embed failed at chunk %d: %w", start/batchSize+1, err)
		}
		allEmbeddings = append(allEmbeddings, embeddings...)
		slog.Info("Embedded chunk", "chunk", start/batchSize+1, "size", len(chunk))
	}
	return allEmbeddings, nil
}

// Generation methods - MiniLM is embedding-only
func (p *MiniLMProvider) Generate(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return nil, fmt.Errorf("minilm: text generation not supported")
}

func (p *MiniLMProvider) StreamGenerate(ctx context.Context, prompt string) (<-chan string, error) {
	return nil, fmt.Errorf("minilm: text generation not supported")
}

func (p *MiniLMProvider) Close() error {
	if p.model != nil {
		p.model.Close()
	}
	return nil
}
