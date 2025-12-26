package domain

import "time"

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
