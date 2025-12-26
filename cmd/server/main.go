package main

import (
	"context"
	"log"
	"net/http"

	"github.com/soyokaze83/invictus/internal/config"
	"github.com/soyokaze83/invictus/internal/handler"
	"github.com/soyokaze83/invictus/internal/middleware"
	"github.com/soyokaze83/invictus/internal/service"
)

func main() {

	ctx := context.Background()

	// load configs
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Println(err)
		return
	}

	// init services
	llmService, err := service.NewGeminiService(ctx, cfg.APIKey, cfg.ModelName)
	if err != nil {
		log.Println("Error initializing LLM model")
		return
	}

	// init handlers
	queryHandler := handler.NewQueryHandler(llmService)

	// apply mux on routes
	mux := http.NewServeMux()
	mux.HandleFunc("/", queryHandler.ReadRoot)
	mux.HandleFunc("POST /query", queryHandler.HandleQuery)

	loggedMux := middleware.LoggingMiddleware(mux)
	http.ListenAndServe(":8000", loggedMux)
}
