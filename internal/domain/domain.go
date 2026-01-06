package domain

import "time"

// ========== LLM-Related ==========

type LLMRequest struct {
	ID     string `json:"id"`
	Prompt string `json:"query"`
}

type LLMResponse struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	// ContentStream string `json:"contentStream"`
}

type PromptData struct {
	Context string
	Query   string
}

// ========== OBJECTS ==========

type Story struct {
	ID        int
	Author    string
	Title     string
	URL       string
	Score     int
	Timestamp time.Time
	Content   string
	Embedding []float32 // optional, for vector storage
}

type ChatMessage struct {
}

type SearchResult struct {
	ID       int64
	Title    string
	URL      string
	Content  string
	Distance float64
}
