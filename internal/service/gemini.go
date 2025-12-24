package service

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type GeminiService struct {
	client *genai.Client
	model  string
}

func NewGeminiService(ctx context.Context, apiKey string, model string) (*GeminiService, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}

	return &GeminiService{
		client: client,
		model:  model,
	}, nil
}

func (s *GeminiService) Close() {
	s.client.Close()
}

func (s *GeminiService) Query(ctx context.Context, query string) (string, error) {
	model := s.client.GenerativeModel(s.model)
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
