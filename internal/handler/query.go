package handler

import (
	"encoding/json"
	"net/http"

	"github.com/soyokaze83/invictus/internal/service"
)

type QueryHandler struct {
	llm_svc *service.GeminiService
}

func NewQueryHandler(llm_svc *service.GeminiService) *QueryHandler {
	return &QueryHandler{llm_svc: llm_svc}
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

	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	fullText, err := q.llm_svc.Query(r.Context(), req.Query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fullText)
}
