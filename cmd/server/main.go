package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/soyokaze83/invictus/internal/config"
	"github.com/soyokaze83/invictus/internal/handler"
	"github.com/soyokaze83/invictus/internal/middleware"
	"github.com/soyokaze83/invictus/internal/provider"
	"github.com/soyokaze83/invictus/internal/vectordb"
)

func main() {

	ctx := context.Background()

	// Load configurations
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		return
	}

	// Initialize LLM provider (generation model)
	generationModel, err := provider.NewProvider(
		ctx,
		provider.ProviderType(cfg.GenerationModelType),
		cfg.GenerationModelName,
		cfg.GetAPIKeys(cfg.GenerationModelType),
	)
	if err != nil {
		slog.Error("Failed to initialize LLM", "error", err)
		return
	}
	defer generationModel.Close()

	// Initialize LLM provider (embedding model)
	embeddingModel, err := provider.NewProvider(
		ctx,
		provider.ProviderType(cfg.EmbeddingModelType),
		cfg.EmbeddingModelName,
		cfg.GetAPIKeys(cfg.EmbeddingModelType),
	)
	if err != nil {
		slog.Error("Failed to initialize embedding model", "error", err)
		return
	}
	defer embeddingModel.Close()

	// Initialize vector database
	vdb, err := vectordb.New(ctx, cfg.PostgresURL, cfg.EmbeddingDim)
	if err != nil {
		slog.Error("Failed to init vectordb", "error", err)
		return
	}
	defer vdb.Close()

	// Initialize handlers
	queryHandler := handler.NewQueryHandler(generationModel, embeddingModel, vdb)

	// Apply mux on routers
	mux := http.NewServeMux()
	mux.HandleFunc("/", queryHandler.ReadRoot)
	mux.HandleFunc("POST /query", queryHandler.HandleQuery)
	mux.HandleFunc("POST /rag-query", queryHandler.RAG)

	loggedMux := middleware.LoggingMiddleware(mux)
	http.ListenAndServe(":8082", loggedMux)
}
