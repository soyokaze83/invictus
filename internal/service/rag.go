package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"text/template"

	"github.com/soyokaze83/invictus/internal/domain"
	"github.com/soyokaze83/invictus/internal/prompts"
	"github.com/soyokaze83/invictus/internal/provider"
	"github.com/soyokaze83/invictus/internal/vectordb"
)

const maxSearchLimit = 10

func RAG(
	ctx context.Context,
	generationModel provider.LLMProvider,
	embeddingModel provider.LLMProvider,
	vdb *vectordb.VectorDB,
	textQuery string,
) (string, error) {

	// Embed input prompt from user
	queryEmbedding, err := embeddingModel.Embed(ctx, textQuery)
	if err != nil {
		slog.Error("Failed to embed input query", "error", err)
		return "", err
	}

	// Retrieve documents through vectordb search
	searchResults, err := vdb.Search(ctx, queryEmbedding, maxSearchLimit)
	if err != nil {
		slog.Error("Failed to retrieve documents", "error", err)
		searchResults = []domain.SearchResult{}
	}

	// Construct prompt
	var sb strings.Builder
	for _, doc := range searchResults {
		fmt.Fprintf(&sb, "Title: %s\nURL: %s\nContent: %s\n\n", doc.Title, doc.URL, doc.Content)
	}
	contextPrompt := sb.String()

	fullPrompt, err := RAGPrompt(prompts.AgentPrompt, contextPrompt, textQuery)
	if err != nil {
		slog.Error("Failed to construct prompt", "error", err)
		return "", err
	}

	// Generate with generation model based off prompt
	responseText, err := generationModel.Generate(ctx, fullPrompt)
	if err != nil {
		slog.Error("Failed to generate response", "error", err)
		return "", err
	}

	return responseText.Content, nil
}

func RAGPrompt(systemPrompt string, contextPrompt string, query string) (string, error) {
	tmpl, err := template.New("rag").Parse(systemPrompt)
	if err != nil {
		return "", err
	}

	promptData := domain.PromptData{
		Context: contextPrompt,
		Query:   query,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, promptData); err != nil {
		return "", err
	}

	return buf.String(), nil
}
