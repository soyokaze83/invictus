package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/soyokaze83/invictus/internal/config"
	"github.com/soyokaze83/invictus/internal/domain"
	"github.com/soyokaze83/invictus/internal/hackernews"
	"github.com/soyokaze83/invictus/internal/provider"
	"github.com/soyokaze83/invictus/internal/utils"
	"github.com/soyokaze83/invictus/internal/vectordb"
)

func main() {
	// Setup graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Load configurations
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize pgvector database
	vdb, err := vectordb.New(ctx, cfg.PostgresURL, cfg.EmbeddingDim)
	if err != nil {
		slog.Error("Failed to init vectordb", "error", err)
		os.Exit(1)
	}
	defer vdb.Close()

	// Initialize embedding model
	embeddingModel, err := provider.NewProvider(
		ctx,
		provider.ProviderType(cfg.EmbeddingModelType),
		cfg.EmbeddingModelName,
		cfg.GetAPIKeys(cfg.EmbeddingModelType),
	)
	if err != nil {
		slog.Error("Failed to initialize embedding model", "error", err)
		os.Exit(1)
	}
	defer embeddingModel.Close()

	hn := hackernews.New()

	// Run immediately on start
	runIngestion(ctx, hn, embeddingModel, vdb, cfg.EmbeddingBatchSize, cfg.UseBatchEmbedding)

	// Start HTTP server for manual triggers
	server := &http.Server{Addr: ":8081"}
	http.HandleFunc("/trigger-ingestion", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		go runIngestion(ctx, hn, embeddingModel, vdb, cfg.EmbeddingBatchSize, cfg.UseBatchEmbedding)
		w.Write([]byte("Ingestion triggered"))
	})

	go func() {
		slog.Info("Manual trigger endpoint listening on :8081")
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	// Daily scheduler
	wib := time.FixedZone("WIB", 7*60*60) // UTC+7
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Shutting down gracefully...")
			server.Shutdown(context.Background())
			return
		case <-ticker.C:
			if time.Now().In(wib).Hour() == cfg.TargetHour {
				runIngestion(ctx, hn, embeddingModel, vdb, cfg.EmbeddingBatchSize, cfg.UseBatchEmbedding)
			}
		}
	}
}

func runIngestion(ctx context.Context, hn *hackernews.Client, llm provider.LLMProvider, vdb *vectordb.VectorDB, batchSize int, useBatch bool) {
	slog.Info("Starting ingestion")

	// Phase 1: Fetch story IDs
	ids, err := hn.GetBestStories(ctx)
	if err != nil {
		slog.Error("Failed to fetch best stories", "error", err)
		return
	}
	slog.Info("Fetched story IDs", "count", len(ids))

	// Phase 2: Fetch story content in parallel
	stories := fetchStoriesParallel(ctx, hn, ids, 5)
	if len(stories) == 0 {
		slog.Warn("No stories fetched successfully")
		return
	}
	slog.Info("Fetched story content", "successful", len(stories), "total", len(ids))

	// Phase 3: Batch embed all texts (sanitize UTF-8 first)
	texts := make([]string, len(stories))
	for i, s := range stories {
		texts[i] = utils.SanitizeUTF8(s.Content)
	}

	var embeddings [][]float32
	if useBatch {
		embeddings, err = llm.EmbedBatchWithRetry(ctx, texts, 5, batchSize)
		if err != nil {
			slog.Error("Failed to batch embed", "error", err)
			return
		}
	} else {
		slog.Info("Using single embedding mode")
		embeddings = make([][]float32, len(texts))
		for i, text := range texts {
			embeddings[i], err = llm.EmbedWithRetry(ctx, text, 5)
			if err != nil {
				slog.Error("Failed to embed text", "index", i, "error", err)
				return
			}
			if (i+1)%10 == 0 {
				slog.Info("Embedding progress", "completed", i+1, "total", len(texts))
			}
		}
	}
	slog.Info("Generated embeddings", "count", len(embeddings))

	// Phase 4: Combine stories with embeddings and batch upsert
	domainStories := make([]domain.Story, len(stories))
	for i, s := range stories {
		domainStories[i] = domain.Story{
			ID:        s.ID,
			Author:    s.Author,
			Title:     s.Title,
			URL:       s.URL,
			Score:     s.Score,
			Timestamp: s.Timestamp,
			Content:   s.Content,
			Embedding: embeddings[i],
		}
	}

	if err := vdb.UpsertBatch(ctx, domainStories); err != nil {
		slog.Error("Failed to batch upsert", "error", err)
		return
	}

	// Create IVFFlat index if it doesn't exist (works better after data is loaded)
	if err := vdb.CreateIndex(ctx, 100); err != nil {
		slog.Warn("Failed to create index (may already exist)", "error", err)
	}

	slog.Info("Ingestion complete", "stories_ingested", len(domainStories))
}

// fetchStoriesParallel fetches stories concurrently with a semaphore limit.
// Skips failures and returns only successfully fetched stories.
func fetchStoriesParallel(ctx context.Context, hn *hackernews.Client, ids []int, concurrency int) []*domain.Story {
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		sem     = make(chan struct{}, concurrency)
		stories []*domain.Story
	)

	for _, id := range ids {
		wg.Add(1)
		go func(storyID int) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
				defer func() { <-sem }()
			}

			story, err := hn.GetStory(ctx, storyID)
			if err != nil {
				slog.Warn("Failed to fetch story", "id", storyID, "error", err)
				return
			}

			// Skip stories without URL or content
			if story.URL == "" || story.Content == "" {
				slog.Debug("Skipping story without content", "id", storyID)
				return
			}

			// Truncate content to 8000 runes for embedding (safe UTF-8 truncation)
			story.Content = utils.TruncateUTF8(story.Content, 8000)

			mu.Lock()
			stories = append(stories, story)
			mu.Unlock()
		}(id)
	}

	wg.Wait()
	return stories
}
