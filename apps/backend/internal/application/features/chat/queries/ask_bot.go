package queries

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"time"

	"university-chatbot/backend/internal/domain"
)

// System prompts are now centralized in domain/prompts.go

// AskBotQuery is the input for a single chat turn.
type AskBotQuery struct {
	Request *domain.ChatRequest
}

// AskBotResult carries sources and timing info back to the handler.
type AskBotResult struct {
	Sources    []domain.Source
	QueryHash  string
	StartedAt  time.Time
}

// AskBotHandler orchestrates RAG: embed → search → generate (streaming).
type AskBotHandler struct {
	vectorStore domain.VectorStore
	llm         domain.LLMClient
	analytics   domain.AnalyticsRepo
}

// NewAskBotHandler constructs the handler with injected dependencies.
func NewAskBotHandler(vs domain.VectorStore, llm domain.LLMClient, ar domain.AnalyticsRepo) *AskBotHandler {
	return &AskBotHandler{vectorStore: vs, llm: llm, analytics: ar}
}

// Handle executes the RAG pipeline and streams tokens to w.
func (h *AskBotHandler) Handle(ctx context.Context, q AskBotQuery, w io.Writer) (*AskBotResult, error) {
	start := time.Now()
	req := q.Request

	// --- 1. Build query hash for analytics (no PII stored) ---
	hash := sha256.Sum256([]byte(strings.TrimSpace(req.Message)))
	queryHash := fmt.Sprintf("%x", hash[:8])

	// --- 2. Hybrid search for relevant context ---
	results, err := h.vectorStore.HybridSearch(ctx, req.Message, 5)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	// --- 3. Build context string from top chunks (O(1) deduplication) ---
	var contextBuf bytes.Buffer
	sources := make([]domain.Source, 0, len(results))
	seenDocs := make(map[string]bool)

	for _, r := range results {
		if r.Score < 0.5 {
			continue
		}
		
		fmt.Fprintf(&contextBuf, "--- Документ: %s (стор. %d) ---\n%s\n\n",
			r.Chunk.DocumentName, r.Chunk.PageNumber, r.Chunk.Text)

		if !seenDocs[r.Chunk.DocumentName] && len(sources) < 5 {
			seenDocs[r.Chunk.DocumentName] = true
			sources = append(sources, domain.Source{
				DocumentName: r.Chunk.DocumentName,
				Score:        r.Score,
				PageNumber:   r.Chunk.PageNumber,
			})
		}
	}

	// --- 4. Select system prompt by language ---
	sysPrompt := domain.SystemPromptUA
	if req.Language == domain.LangEn {
		sysPrompt = domain.SystemPromptEN
	}

	// --- 5. Stream LLM response ---
	if err := h.llm.StreamAnswer(ctx, sysPrompt, req.Message, contextBuf.String(), req.Language, w); err != nil {
		return nil, fmt.Errorf("llm stream: %w", err)
	}

	elapsed := time.Since(start).Milliseconds()

	// --- 6. Record analytics in background ---
	go func() {
		// Use a detached context so cancellation of the HTTP request doesn't abort the DB insert
		rec := domain.QueryRecord{
			QueryHash:  queryHash,
			Language:   req.Language,
			ResponseMs: elapsed,
			SourcesCnt: len(sources),
			IsBlocked:  false,
		}
		_ = h.analytics.Record(context.Background(), rec)
	}()

	return &AskBotResult{
		Sources:   sources,
		QueryHash: queryHash,
		StartedAt: start,
	}, nil
}
