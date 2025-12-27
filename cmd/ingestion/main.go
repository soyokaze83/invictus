package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

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

	embeddingModel, err := service.NewGeminiService(ctx, cfg.APIKeys, cfg.ModelName)
	if err != nil {
		slog.Error("Failed to init gemini", "error", err)
		os.Exit(1)
	}
	defer embeddingModel.Close()

	hn := hackernews.New()

	// Run immediately on start, then schedule
	runIngestion(ctx, hn, embeddingModel, vdb)

	go func() {
		http.HandleFunc("/trigger-ingestion", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			go runIngestion(ctx, hn, embeddingModel, vdb)
			w.Write([]byte("Ingestion triggered"))
		})
		slog.Info("Manual trigger endpoint listening on :8081")
		http.ListenAndServe(":8081", nil)
	}()

	wib := time.FixedZone("WIB", 7*60*60) // UTC+7

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		if time.Now().In(wib).Hour() == cfg.TargetHour {
			runIngestion(ctx, hn, embeddingModel, vdb)
		}
	}

}

func runIngestion(ctx context.Context, hn *hackernews.Client, embeddingModel *service.GeminiService, vdb *vectordb.VectorDB) {
	ids, err := hn.GetBestStories(ctx)
	if err != nil {
		slog.Error("Failed to fetch best stories", "error", err)
		return
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)

	for _, id := range ids {
		wg.Add(1)
		go func(storyID int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := processStory(ctx, hn, embeddingModel, vdb, storyID); err != nil {
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
	story, err := hn.GetStory(ctx, storyID)
	if err != nil {
		return fmt.Errorf("fetch story: %w", err)
	}

	if story.URL == "" {
		return fmt.Errorf("no URL for story %d", storyID)
	}
	if story.Content == "" {
		return fmt.Errorf("no content for story %d", storyID)
	}

	if len(story.Content) > 8000 {
		story.Content = story.Content[:8000]
	}

	embedding, err := gemini.EmbedWithRetry(ctx, story.Content, 5)
	if err != nil {
		return err
	}

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
