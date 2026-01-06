package vectordb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
	"github.com/soyokaze83/invictus/internal/domain"
)

type VectorDB struct {
	pool         *pgxpool.Pool
	embeddingDim int
}

func New(ctx context.Context, connStr string, embeddingDim int) (*VectorDB, error) {
	// First, create the vector extension and schema using a simple connection
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("vectordb: failed to connect: %w", err)
	}
	defer conn.Close(ctx)

	// Create extension and table (index created separately after data exists)
	_, err = conn.Exec(ctx, fmt.Sprintf(`
		CREATE EXTENSION IF NOT EXISTS vector;

		CREATE TABLE IF NOT EXISTS stories_hn (
			id INTEGER PRIMARY KEY,
			author TEXT,
			title TEXT,
			url TEXT,
			score INTEGER,
			timestamp TIMESTAMP,
			content TEXT,
			embedding VECTOR(%d)
		);
	`, embeddingDim))
	if err != nil {
		return nil, fmt.Errorf("vectordb: failed to initialize schema: %w", err)
	}

	// Now create the pool with type registration
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("vectordb: failed to parse config: %w", err)
	}

	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("vectordb: failed to create pool: %w", err)
	}

	return &VectorDB{pool: pool, embeddingDim: embeddingDim}, nil
}

func (v *VectorDB) Close() {
	v.pool.Close()
}

// CreateIndex creates an IVFFlat index for vector similarity search.
// Should be called after initial data is loaded for better index quality.
// lists parameter controls the number of clusters (default 100, higher = better recall but slower).
func (v *VectorDB) CreateIndex(ctx context.Context, lists int) error {
	if lists <= 0 {
		lists = 100
	}
	_, err := v.pool.Exec(ctx, fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS stories_embedding_idx
		ON stories USING ivfflat (embedding vector_cosine_ops)
		WITH (lists = %d)
	`, lists))
	if err != nil {
		return fmt.Errorf("vectordb: failed to create index: %w", err)
	}
	return nil
}

// SetProbes adjusts the query-time recall/speed tradeoff for IVFFlat.
// Higher values improve recall at the cost of query speed.
// Default is 1, recommended 10-50 for better recall.
func (v *VectorDB) SetProbes(ctx context.Context, value int) error {
	_, err := v.pool.Exec(ctx, fmt.Sprintf("SET ivfflat.probes = %d", value))
	if err != nil {
		return fmt.Errorf("vectordb: failed to set probes: %w", err)
	}
	return nil
}

// Upsert inserts or updates a single story.
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
	if err != nil {
		return fmt.Errorf("vectordb: failed to upsert story %d: %w", story.ID, err)
	}
	return nil
}

// UpsertBatch efficiently inserts or updates multiple stories using COPY.
func (v *VectorDB) UpsertBatch(ctx context.Context, stories []domain.Story) error {
	if len(stories) == 0 {
		return nil
	}

	tx, err := v.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("vectordb: failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Create temp table
	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE stories_staging (
			id INTEGER,
			author TEXT,
			title TEXT,
			url TEXT,
			score INTEGER,
			timestamp TIMESTAMP,
			content TEXT,
			embedding VECTOR
		) ON COMMIT DROP
	`)
	if err != nil {
		return fmt.Errorf("vectordb: failed to create staging table: %w", err)
	}

	// Bulk copy into staging table
	rows := make([][]any, len(stories))
	for i, s := range stories {
		rows[i] = []any{
			s.ID, s.Author, s.Title, s.URL, s.Score, s.Timestamp,
			s.Content, pgvector.NewVector(s.Embedding),
		}
	}

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"stories_staging"},
		[]string{"id", "author", "title", "url", "score", "timestamp", "content", "embedding"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("vectordb: failed to copy to staging: %w", err)
	}

	// Merge from staging to main table
	_, err = tx.Exec(ctx, `
		INSERT INTO stories (id, author, title, url, score, timestamp, content, embedding)
		SELECT id, author, title, url, score, timestamp, content, embedding
		FROM stories_staging
		ON CONFLICT (id) DO UPDATE SET
			author = EXCLUDED.author,
			title = EXCLUDED.title,
			url = EXCLUDED.url,
			score = EXCLUDED.score,
			timestamp = EXCLUDED.timestamp,
			content = EXCLUDED.content,
			embedding = EXCLUDED.embedding
	`)
	if err != nil {
		return fmt.Errorf("vectordb: failed to merge from staging: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("vectordb: failed to commit batch upsert: %w", err)
	}

	return nil
}

// Search finds the nearest stories to the query embedding using cosine distance.
func (v *VectorDB) Search(ctx context.Context, queryEmbedding []float32, limit int) ([]domain.SearchResult, error) {
	rows, err := v.pool.Query(ctx,
		`SELECT id, title, url, content, embedding <=> $1 AS distance
		FROM stories
		ORDER BY distance
		LIMIT $2`,
		pgvector.NewVector(queryEmbedding), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("vectordb: search query failed: %w", err)
	}
	defer rows.Close()

	var results []domain.SearchResult
	for rows.Next() {
		var r domain.SearchResult
		if err := rows.Scan(&r.ID, &r.Title, &r.URL, &r.Content, &r.Distance); err != nil {
			return nil, fmt.Errorf("vectordb: failed to scan result: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vectordb: error iterating results: %w", err)
	}
	return results, nil
}
