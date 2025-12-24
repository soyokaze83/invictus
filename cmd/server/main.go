package main

import (
	"context"
	"log"
	"net/http"

	"github.com/soyokaze83/invictus/internal/config"
	"github.com/soyokaze83/invictus/internal/handler"
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

	// init query handler
	queryHandler := handler.NewQueryHandler(llmService)
	http.HandleFunc("/", queryHandler.ReadRoot)
	http.HandleFunc("POST /query", queryHandler.HandleQuery)

	http.ListenAndServe(":8000", nil)
}
