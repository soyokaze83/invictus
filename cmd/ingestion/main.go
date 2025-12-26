package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/soyokaze83/invictus/internal/config"
	"github.com/soyokaze83/invictus/internal/domain"
	"github.com/soyokaze83/invictus/internal/hackernews"
	"github.com/soyokaze83/invictus/internal/service"
	"github.com/soyokaze83/invictus/internal/vectordb"
)

func main() {
	ctx := context.Background()

	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		return
	}

	// Initialize clients
	vdb, err := vectordb.New(ctx, cfg.PostgresURL)
	if err != nil {
		slog.Error("Failed to init vectordb", "error", err)
		os.Exit(1)
	}
	defer vdb.Close()

	embedding_model, err := service.NewGeminiService(ctx, cfg.APIKey, cfg.ModelName)
	if err != nil {
		slog.Error("Failed to init gemini", "error", err)
		os.Exit(1)
	}
	defer embedding_model.Close()

	hn := hackernews.New()

	// Fetch best story IDs
	ids, err := hn.GetBestStories(ctx)
	if err != nil {
		slog.Error("Failed to fetch best stories", "error", err)
		os.Exit(1)
	}

	// Process concurrently with worker pool
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // 5 concurrent workers

	for _, id := range ids {
		wg.Add(1)
		go func(storyID int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := processStory(ctx, hn, embedding_model, vdb, storyID); err != nil {
				slog.Warn("Failed to process story", "id", storyID, "error", err)
			}
		}(id)
	}

	wg.Wait()
	slog.Info("Ingestion complete")
}

func processStory(
	ctx context.Context,
	hn *hackernews.Client,
	gemini *service.GeminiService,
	vdb *vectordb.VectorDB,
	storyID int,
) error {
	// fetch story information
	story, err := hn.GetStory(ctx, storyID)
	if err != nil {
		return fmt.Errorf("fetch story: %w", err)
	}

	// check for empty url and content
	if story.URL == "" {
		return fmt.Errorf("no URL for story %d", storyID)
	}
	if story.Content == "" {
		return fmt.Errorf("no content for story %d", storyID)
	}

	// truncate content for embedding
	if len(story.Content) > 8000 {
		story.Content = story.Content[:8000]
	}

	// generate embeddings with model
	embedding, err := gemini.Embed(ctx, story.Content)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}

	// store story in vectordb
	embedded_story := domain.Story{
		ID:        storyID,
		Author:    story.Author,
		Title:     story.Title,
		URL:       story.URL,
		Score:     story.Score,
		Timestamp: story.Timestamp,
		Content:   story.Content,
		Embedding: embedding,
	}

	if err := vdb.Upsert(ctx, embedded_story); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}

	slog.Info("Ingested story", "id", embedded_story.ID, "title", embedded_story.Title)
	return nil
}
