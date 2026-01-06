package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/soyokaze83/invictus/internal/domain"
	"github.com/soyokaze83/invictus/internal/provider"
	"github.com/soyokaze83/invictus/internal/service"
	"github.com/soyokaze83/invictus/internal/vectordb"
)

type QueryHandler struct {
	generationModel provider.LLMProvider
	embeddingModel  provider.LLMProvider
	vdb             *vectordb.VectorDB
}

type QueryRequest struct {
	Query string `json:"query"`
}

func NewQueryHandler(generationModel provider.LLMProvider, embeddingModel provider.LLMProvider, vdb *vectordb.VectorDB) *QueryHandler {
	return &QueryHandler{
		generationModel: generationModel,
		embeddingModel:  embeddingModel,
		vdb:             vdb,
	}
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

	responseText, err := q.generationModel.Generate(r.Context(), req.Prompt)
	if err != nil {
		slog.Error("Failed to generate a response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responseText)
}

func (q *QueryHandler) RAG(w http.ResponseWriter, r *http.Request) {

	var req domain.LLMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	responseText, err := service.RAG(r.Context(), q.generationModel, q.embeddingModel, q.vdb, req.Prompt)
	if err != nil {
		slog.Error("Failed to generate RAG response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responseText)
}
