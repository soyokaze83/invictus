package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/soyokaze83/invictus/internal/domain"
	"github.com/soyokaze83/invictus/internal/provider"
)

type QueryHandler struct {
	llm provider.LLMProvider
}

type QueryRequest struct {
	Query string `json:"query"`
}

func NewQueryHandler(llm provider.LLMProvider) *QueryHandler {
	return &QueryHandler{llm: llm}
}

func (q *QueryHandler) ReadRoot(w http.ResponseWriter, r *http.Request) {
	response := []map[string]string{
		{"id": "1", "response": "hello world"},
		{"id": "2", "response": "hello nation"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (q *QueryHandler) HandleQuery(w http.ResponseWriter, r *http.Request) {

	var req domain.LLMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	responseText, err := q.llm.Generate(r.Context(), req)
	if err != nil {
		slog.Error("Failed to generate a response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responseText)
}
