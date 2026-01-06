package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/soyokaze83/invictus/internal/config"
	"github.com/soyokaze83/invictus/internal/handler"
	"github.com/soyokaze83/invictus/internal/middleware"
	"github.com/soyokaze83/invictus/internal/provider"
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
	llm, err := provider.NewProvider(
		ctx,
		provider.ProviderType(cfg.GenerationModelType),
		cfg.GenerationModelName,
		cfg.GetAPIKeys(cfg.GenerationModelType),
	)
	if err != nil {
		slog.Error("Failed to initialize LLM", "error", err)
		return
	}

	// init handlers
	queryHandler := handler.NewQueryHandler(llm)

	// apply mux on routes
	mux := http.NewServeMux()
	mux.HandleFunc("/", queryHandler.ReadRoot)
	mux.HandleFunc("POST /query", queryHandler.HandleQuery)

	loggedMux := middleware.LoggingMiddleware(mux)
	http.ListenAndServe(":8000", loggedMux)
}
