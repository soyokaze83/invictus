package vectordb

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
	"github.com/soyokaze83/invictus/internal/domain"
)

type VectorDB struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, connStr string) (*VectorDB, error) {
	// First, create the vector extension using a simple connection
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, err
	}
	_, err = conn.Exec(ctx,
		`CREATE EXTENSION IF NOT EXISTS vector;

		CREATE TABLE IF NOT EXISTS stories (
			id INTEGER PRIMARY KEY,
			author TEXT,
			title TEXT,
			url TEXT,
			score INTEGER,
			timestamp TIMESTAMP,
			content TEXT,
			embedding VECTOR(3072)
		); `)
	conn.Close(ctx)
	if err != nil {
		return nil, err
	}

	// Now create the pool with type registration (extension exists now)
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}

	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	return &VectorDB{pool: pool}, nil
}

func (v *VectorDB) Close() {
	v.pool.Close()
}

// Ingestion
func (v *VectorDB) Upsert(ctx context.Context, story domain.Story) error {
	_, err := v.pool.Exec(ctx,
		`INSERT INTO stories (id, author, title, url, score, timestamp, content, embedding)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET 
		author = EXCLUDED.author,
		title = EXCLUDED.title,
		url = EXCLUDED.url,
		score = EXCLUDED.score,
		timestamp = EXCLUDED.timestamp,
		content = EXCLUDED.content,
		embedding = EXCLUDED.embedding`,
		story.ID, story.Author, story.Title, story.URL, story.Score, story.Timestamp,
		story.Content, pgvector.NewVector(story.Embedding),
	)
	return err
}

// Retrieval
func (v *VectorDB) Search(ctx context.Context, queryEmbedding []float32, limit int) ([]domain.SearchResult, error) {
	rows, err := v.pool.Query(ctx,
		`SELECT id, title, url, content, embedding <=> $1 AS distance
		FROM stories
		ORDER BY distance
		LIMIT $2`,
		pgvector.NewVector(queryEmbedding), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.SearchResult
	for rows.Next() {
		var r domain.SearchResult
		if err := rows.Scan(&r.ID, &r.Title, &r.URL, &r.Content, &r.Distance); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
