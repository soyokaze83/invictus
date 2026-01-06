# RAG Chatbot Implementation Plan for Invictus

## Overview

Convert the existing Query Server from a direct LLM passthrough into a RAG (Retrieval-Augmented Generation) chatbot with HackerNews knowledge base.

## Requirements

- **Generation Model**: Gemini (configurable via `GENERATION_MODEL_TYPE`)
- **Embedding Model**: MiniLM (configurable via `EMBEDDING_MODEL_TYPE`)
- **Conversation Memory**: In-memory session (lost on restart)
- **Context Size**: 5-10 HackerNews stories retrieved per query
- **Citations**: Include source URLs in responses

## Environment Configuration

The project uses split model configuration (see `.env.example`):

```env
# Generation model (for chat responses)
GENERATION_MODEL_TYPE=gemini
GENERATION_MODEL_NAME=gemini-2.5-flash

# Embedding model (for vector search)
EMBEDDING_MODEL_TYPE=minilm
EMBEDDING_MODEL_NAME=libonnxruntime.so

# API Keys (comma-separated for rotation)
GEMINI_API_KEYS=key1,key2,key3
OPENAI_API_KEYS=key1,key2,key3

# Database
POSTGRES_URL=postgres://user:pass@host:5432/db

# Server
PORT_NUMBER=8000
```

This allows using different providers for generation vs embedding (e.g., Gemini for chat, MiniLM for local embeddings).

---

## Current Architecture

```
User Query --> QueryHandler --> LLM.Generate() --> Response
```

**Problem**: The query server doesn't use the vector database. It passes queries directly to the LLM without any HackerNews context.

---

## Target Architecture

```
User Query + SessionID
       |
       v
  QueryHandler
       |
       +-> SessionStore.GetHistory(sessionID)
       +-> LLM.EmbedWithRetry(query)
       +-> VectorDB.Search(embedding, limit=10)
       +-> BuildRAGPrompt(query, context, history)
       +-> LLM.Generate(ragPrompt)
       +-> SessionStore.AppendHistory()
       v
  Response with Citations
```

---

## Implementation Steps

### Step 1: Add New Domain Types

**File**: `internal/domain/domain.go`

Add these types:

```go
// ChatMessage represents a single message in conversation history
type ChatMessage struct {
    Role    string    `json:"role"`    // "user" or "assistant"
    Content string    `json:"content"`
    Time    time.Time `json:"time"`
}

// Citation represents a source used in the response
type Citation struct {
    Title string `json:"title"`
    URL   string `json:"url"`
    ID    int64  `json:"id"`
}

// RAGRequest extends the query request with session support
type RAGRequest struct {
    Query     string `json:"query"`
    SessionID string `json:"session_id,omitempty"`
    Stream    bool   `json:"stream,omitempty"`
}

// RAGResponse includes the response with citations and session info
type RAGResponse struct {
    SessionID string     `json:"session_id"`
    Content   string     `json:"content"`
    Citations []Citation `json:"citations"`
}
```

---

### Step 2: Create Session Store

**File**: `internal/session/store.go` (NEW)

Create an in-memory session store:

```go
package session

import (
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/soyokaze83/invictus/internal/domain"
)

const (
    MaxHistoryLength = 10        // Keep last 10 exchanges (20 messages)
    SessionTTL       = time.Hour // Sessions expire after 1 hour of inactivity
)

type Session struct {
    ID        string
    History   []domain.ChatMessage
    CreatedAt time.Time
    UpdatedAt time.Time
}

type Store struct {
    mu       sync.RWMutex
    sessions map[string]*Session
}

func NewStore() *Store {
    return &Store{
        sessions: make(map[string]*Session),
    }
}

// GetOrCreate returns existing session or creates new one
func (s *Store) GetOrCreate(sessionID string) *Session {
    s.mu.Lock()
    defer s.mu.Unlock()

    if sessionID == "" {
        sessionID = uuid.New().String()
    }

    session, exists := s.sessions[sessionID]
    if !exists {
        session = &Session{
            ID:        sessionID,
            History:   make([]domain.ChatMessage, 0),
            CreatedAt: time.Now(),
            UpdatedAt: time.Now(),
        }
        s.sessions[sessionID] = session
    }
    return session
}

// AppendMessage adds a message to session history
func (s *Store) AppendMessage(sessionID string, msg domain.ChatMessage) {
    s.mu.Lock()
    defer s.mu.Unlock()

    session, exists := s.sessions[sessionID]
    if !exists {
        return
    }

    session.History = append(session.History, msg)
    session.UpdatedAt = time.Now()

    // Trim to MaxHistoryLength pairs
    if len(session.History) > MaxHistoryLength*2 {
        session.History = session.History[len(session.History)-MaxHistoryLength*2:]
    }
}

// GetHistory returns conversation history for a session
func (s *Store) GetHistory(sessionID string) []domain.ChatMessage {
    s.mu.RLock()
    defer s.mu.RUnlock()

    session, exists := s.sessions[sessionID]
    if !exists {
        return nil
    }
    return session.History
}

// Cleanup removes expired sessions
func (s *Store) Cleanup() {
    s.mu.Lock()
    defer s.mu.Unlock()

    cutoff := time.Now().Add(-SessionTTL)
    for id, session := range s.sessions {
        if session.UpdatedAt.Before(cutoff) {
            delete(s.sessions, id)
        }
    }
}
```

---

### Step 3: Create RAG Service

**File**: `internal/rag/service.go` (NEW)

The RAG service uses **two separate LLM providers**:
- `embeddingLLM` - for embedding queries (e.g., MiniLM)
- `generationLLM` - for generating responses (e.g., Gemini)

```go
package rag

import (
    "context"
    "fmt"
    "strings"
    "time"

    "github.com/soyokaze83/invictus/internal/domain"
    "github.com/soyokaze83/invictus/internal/provider"
    "github.com/soyokaze83/invictus/internal/session"
    "github.com/soyokaze83/invictus/internal/vectordb"
)

const (
    DefaultContextLimit = 10
    MaxEmbedRetries     = 3
)

type Service struct {
    embeddingLLM   provider.LLMProvider  // For embedding queries
    generationLLM  provider.LLMProvider  // For generating responses
    vectorDB       *vectordb.VectorDB
    sessionStore   *session.Store
    contextLimit   int
}

func NewService(embeddingLLM, generationLLM provider.LLMProvider, vdb *vectordb.VectorDB, ss *session.Store) *Service {
    return &Service{
        embeddingLLM:   embeddingLLM,
        generationLLM:  generationLLM,
        vectorDB:       vdb,
        sessionStore:   ss,
        contextLimit:   DefaultContextLimit,
    }
}

// Query executes the full RAG pipeline
func (s *Service) Query(ctx context.Context, req domain.RAGRequest) (*domain.RAGResponse, error) {
    // 1. Get or create session
    sess := s.sessionStore.GetOrCreate(req.SessionID)

    // 2. Embed the user query (using embedding provider)
    queryEmbedding, err := s.embeddingLLM.EmbedWithRetry(ctx, req.Query, MaxEmbedRetries)
    if err != nil {
        return nil, fmt.Errorf("failed to embed query: %w", err)
    }

    // 3. Search for relevant context
    searchResults, err := s.vectorDB.Search(ctx, queryEmbedding, s.contextLimit)
    if err != nil {
        return nil, fmt.Errorf("failed to search context: %w", err)
    }

    // 4. Get conversation history
    history := s.sessionStore.GetHistory(sess.ID)

    // 5. Build the RAG prompt
    prompt := s.buildPrompt(req.Query, searchResults, history)

    // 6. Generate response (using generation provider)
    llmReq := domain.LLMRequest{
        Prompt: prompt,
        Stream: req.Stream,
    }

    llmResp, err := s.generationLLM.Generate(ctx, llmReq)
    if err != nil {
        return nil, fmt.Errorf("failed to generate response: %w", err)
    }

    // 7. Store conversation in session
    s.sessionStore.AppendMessage(sess.ID, domain.ChatMessage{
        Role:    "user",
        Content: req.Query,
        Time:    time.Now(),
    })
    s.sessionStore.AppendMessage(sess.ID, domain.ChatMessage{
        Role:    "assistant",
        Content: llmResp.Content,
        Time:    time.Now(),
    })

    // 8. Build citations from search results
    citations := make([]domain.Citation, len(searchResults))
    for i, sr := range searchResults {
        citations[i] = domain.Citation{
            ID:    sr.ID,
            Title: sr.Title,
            URL:   sr.URL,
        }
    }

    return &domain.RAGResponse{
        SessionID: sess.ID,
        Content:   llmResp.Content,
        Citations: citations,
    }, nil
}

// buildPrompt constructs the RAG prompt with context and history
func (s *Service) buildPrompt(query string, results []domain.SearchResult, history []domain.ChatMessage) string {
    var sb strings.Builder

    // System instruction
    sb.WriteString(`You are a helpful AI assistant that answers questions about tech news and HackerNews stories.
You MUST:
1. Answer based on the provided context from HackerNews stories
2. Cite your sources using [Source N] format where N corresponds to the source number
3. If the context doesn't contain relevant information, say so honestly
4. Be concise but informative

`)

    // Add context
    sb.WriteString("## Relevant HackerNews Stories:\n\n")
    for i, r := range results {
        sb.WriteString(fmt.Sprintf("[Source %d] %s\n", i+1, r.Title))
        sb.WriteString(fmt.Sprintf("URL: %s\n", r.URL))
        sb.WriteString(fmt.Sprintf("Content: %s\n\n", truncate(r.Content, 500)))
    }

    // Add conversation history (if any)
    if len(history) > 0 {
        sb.WriteString("\n## Previous Conversation:\n")
        for _, msg := range history {
            role := "User"
            if msg.Role == "assistant" {
                role = "Assistant"
            }
            sb.WriteString(fmt.Sprintf("%s: %s\n", role, truncate(msg.Content, 300)))
        }
        sb.WriteString("\n")
    }

    // Add current query
    sb.WriteString("## Current Question:\n")
    sb.WriteString(query)
    sb.WriteString("\n\n## Your Response:\n")

    return sb.String()
}

func truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen] + "..."
}
```

---

### Step 4: Update Handler

**File**: `internal/handler/query.go`

Replace the current implementation:

```go
package handler

import (
    "encoding/json"
    "log/slog"
    "net/http"

    "github.com/soyokaze83/invictus/internal/domain"
    "github.com/soyokaze83/invictus/internal/rag"
)

type QueryHandler struct {
    ragService *rag.Service
}

func NewQueryHandler(ragService *rag.Service) *QueryHandler {
    return &QueryHandler{ragService: ragService}
}

func (q *QueryHandler) ReadRoot(w http.ResponseWriter, r *http.Request) {
    response := map[string]string{
        "status":  "ok",
        "service": "invictus-rag",
        "version": "1.0.0",
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

func (q *QueryHandler) HandleQuery(w http.ResponseWriter, r *http.Request) {
    var req domain.RAGRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        slog.Error("Failed to decode request", "error", err)
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    if req.Query == "" {
        http.Error(w, "query is required", http.StatusBadRequest)
        return
    }

    response, err := q.ragService.Query(r.Context(), req)
    if err != nil {
        slog.Error("Failed to process RAG query", "error", err)
        http.Error(w, "internal server error", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}
```

---

### Step 5: Update Server Initialization

**File**: `cmd/server/main.go`

Key change: Initialize **two separate LLM providers** - one for embedding queries and one for generating responses.

Use `cfg.GetAPIKeys(providerType)` to get the appropriate API keys for each provider.

```go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/soyokaze83/invictus/internal/config"
    "github.com/soyokaze83/invictus/internal/handler"
    "github.com/soyokaze83/invictus/internal/middleware"
    "github.com/soyokaze83/invictus/internal/provider"
    "github.com/soyokaze83/invictus/internal/rag"
    "github.com/soyokaze83/invictus/internal/session"
    "github.com/soyokaze83/invictus/internal/vectordb"
)

func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()

    // Load configurations
    cfg, err := config.LoadConfig()
    if err != nil {
        slog.Error("Failed to load config", "error", err)
        os.Exit(1)
    }

    // Initialize VectorDB connection
    vdb, err := vectordb.New(ctx, cfg.PostgresURL, cfg.EmbeddingDim)
    if err != nil {
        slog.Error("Failed to initialize VectorDB", "error", err)
        os.Exit(1)
    }
    defer vdb.Close()

    // Set IVFFlat probes for better recall
    if err := vdb.SetProbes(ctx, 20); err != nil {
        slog.Warn("Failed to set probes", "error", err)
    }

    // Initialize EMBEDDING provider (e.g., MiniLM for local embeddings)
    embeddingProvider, err := provider.NewProvider(
        ctx,
        provider.ProviderType(cfg.EmbeddingModelType),
        cfg.EmbeddingModelName,
        cfg.GetAPIKeys(cfg.EmbeddingModelType),  // Use shared API keys
    )
    if err != nil {
        slog.Error("Failed to initialize embedding provider", "error", err)
        os.Exit(1)
    }
    defer embeddingProvider.Close()

    // Initialize GENERATION provider (e.g., Gemini for chat)
    generationProvider, err := provider.NewProvider(
        ctx,
        provider.ProviderType(cfg.GenerationModelType),
        cfg.GenerationModelName,
        cfg.GetAPIKeys(cfg.GenerationModelType),  // Use shared API keys
    )
    if err != nil {
        slog.Error("Failed to initialize generation provider", "error", err)
        os.Exit(1)
    }
    defer generationProvider.Close()

    // Initialize session store
    sessionStore := session.NewStore()

    // Start session cleanup goroutine
    go func() {
        ticker := time.NewTicker(15 * time.Minute)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                sessionStore.Cleanup()
                slog.Debug("Session cleanup completed")
            }
        }
    }()

    // Initialize RAG service with BOTH providers
    ragService := rag.NewService(embeddingProvider, generationProvider, vdb, sessionStore)

    // Initialize handlers
    queryHandler := handler.NewQueryHandler(ragService)

    // Apply mux on routes
    mux := http.NewServeMux()
    mux.HandleFunc("/", queryHandler.ReadRoot)
    mux.HandleFunc("POST /query", queryHandler.HandleQuery)
    mux.HandleFunc("POST /chat", queryHandler.HandleQuery) // Alias

    loggedMux := middleware.LoggingMiddleware(mux)

    // Start server with graceful shutdown
    server := &http.Server{
        Addr:    fmt.Sprintf(":%d", cfg.PortNumber),
        Handler: loggedMux,
    }

    go func() {
        slog.Info("Starting RAG server", "port", cfg.PortNumber)
        if err := server.ListenAndServe(); err != http.ErrServerClosed {
            slog.Error("Server error", "error", err)
        }
    }()

    <-ctx.Done()
    slog.Info("Shutting down server...")

    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer shutdownCancel()
    server.Shutdown(shutdownCtx)
}
```

---

### Step 6: Add UUID Dependency

Run:
```bash
go get github.com/google/uuid
```

---

## Files Summary

| File | Action | Description |
|------|--------|-------------|
| `internal/config/config.go` | **Already Updated** | Split model config (Generation vs Embedding) |
| `internal/domain/domain.go` | **Modify** | Add ChatMessage, Citation, RAGRequest, RAGResponse |
| `internal/session/store.go` | **Create** | In-memory session management |
| `internal/rag/service.go` | **Create** | RAG orchestration with two LLM providers |
| `internal/handler/query.go` | **Modify** | Use RAG service instead of direct LLM |
| `cmd/server/main.go` | **Modify** | Initialize both providers, VectorDB, SessionStore, RAG service |

---

## API Contract

### Request

**Endpoint**: `POST /query` or `POST /chat`

```json
{
    "query": "What are the latest AI developments?",
    "session_id": "optional-uuid-here"
}
```

### Response

```json
{
    "session_id": "550e8400-e29b-41d4-a716-446655440000",
    "content": "Based on recent HackerNews stories, there are several AI developments [Source 1]. Google announced updates to their platform [Source 2]...",
    "citations": [
        {
            "id": 12345,
            "title": "Google Announces AI Updates",
            "url": "https://example.com/google-ai"
        },
        {
            "id": 12346,
            "title": "OpenAI Releases New Model",
            "url": "https://example.com/openai"
        }
    ]
}
```

---

## Error Handling

| Error Type | HTTP Status | Response |
|------------|-------------|----------|
| Invalid JSON body | 400 | "invalid request body" |
| Missing query | 400 | "query is required" |
| Embedding failure | 500 | "internal server error" |
| VectorDB search failure | 500 | "internal server error" |
| LLM generation failure | 500 | "internal server error" |

---

## Testing

After implementation, test with:

```bash
# Start the server
go run cmd/server/main.go

# Test query (new session)
curl -X POST http://localhost:8000/query \
  -H "Content-Type: application/json" \
  -d '{"query": "What are people saying about AI?"}'

# Test with session (multi-turn)
curl -X POST http://localhost:8000/query \
  -H "Content-Type: application/json" \
  -d '{"query": "Tell me more", "session_id": "<session_id_from_previous_response>"}'
```
