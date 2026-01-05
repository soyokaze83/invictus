package domain

import "time"

type LLMRequest struct {
	ID     string `json:"id"`
	Prompt string `json:"query"`
	Stream bool   `json:"stream"`
}

type LLMResponse struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

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

type SearchResult struct {
	ID       int64
	Title    string
	URL      string
	Content  string
	Distance float64
}
