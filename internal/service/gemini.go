package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type GeminiService struct {
	client       *genai.Client
	model        string
	apiKeys      []string
	currentIndex int
	mu           sync.RWMutex
}

func NewGeminiService(ctx context.Context, apiKeys []string, model string) (*GeminiService, error) {
	if len(apiKeys) == 0 {
		return nil, errors.New("no API keys provided")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKeys[0]))
	if err != nil {
		return nil, err
	}

	return &GeminiService{
		client:       client,
		model:        model,
		apiKeys:      apiKeys,
		currentIndex: 0,
	}, nil
}

func (s *GeminiService) Close() {
	s.client.Close()
}

func (s *GeminiService) Query(ctx context.Context, query string) (string, error) {

	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	model := client.GenerativeModel(s.model)
	resp, err := model.GenerateContent(ctx, genai.Text(query))
	if err != nil {
		return "", err
	}

	fullText := s.getResponseText(resp)
	return fullText, nil
}

func (s *GeminiService) getResponseText(resp *genai.GenerateContentResponse) string {
	var fullText string
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				fullText += fmt.Sprintf("%v", part)
			}
		}
	}
	return fullText
}

func (s *GeminiService) embed(ctx context.Context, text string) ([]float32, error) {

	// prevent using closed client between workers
	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	em := client.EmbeddingModel("gemini-embedding-001")
	res, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, err
	}
	return res.Embedding.Values, nil
}

func (s *GeminiService) EmbedWithRetry(ctx context.Context, content string, maxRetries int) ([]float32, error) {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		embedding, err := s.embed(ctx, content)
		if err == nil {
			return embedding, nil
		}

		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) && apiErr.Code == 429 {
			slog.Info("Rate limited, rotating API key", "attempt", i+1)
			if err := s.rotateClient(ctx); err != nil {
				return nil, err
			}
			time.Sleep(time.Duration(1<<i) * time.Second)
			lastErr = err
			continue
		}
		return nil, fmt.Errorf("embed: %w", err)
	}
	return nil, fmt.Errorf("exhausted retries: %w", lastErr)
}

func (s *GeminiService) rotateClient(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client.Close()
	s.currentIndex = (s.currentIndex + 1) % len(s.apiKeys)
	client, err := genai.NewClient(ctx, option.WithAPIKey(s.apiKeys[s.currentIndex]))
	if err != nil {
		return err
	}
	s.client = client
	return nil
}
