package domain

import (
	"context"
	"io"
)

// ─── Port interfaces (Dependency Inversion) ───────────────────────────────────
// Application layer depends only on these abstractions.
// Concrete implementations live in infrastructure/.

// VectorStore is the port for the vector database (Qdrant).
type VectorStore interface {
	// EnsureCollection creates the Qdrant collection if it does not exist.
	EnsureCollection(ctx context.Context) error

	// UpsertChunks embeds and stores text chunks.
	UpsertChunks(ctx context.Context, chunks []Chunk) error

	// HybridSearch performs a dense-vector + sparse (BM25) search.
	HybridSearch(ctx context.Context, query string, topK int) ([]SearchResult, error)

	// DeleteByDocumentID removes all chunks belonging to a document.
	DeleteByDocumentID(ctx context.Context, documentID string) error
}

// LLMClient is the port for the Large Language Model (Gemini).
type LLMClient interface {
	// Embed returns the embedding vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// StreamAnswer sends the prompt to the LLM and streams tokens to the writer.
	// It closes the writer when done or on error.
	StreamAnswer(ctx context.Context, systemPrompt, userQuery, context string, lang Language, w io.Writer) error
}

// AnalyticsRepo is the port for recording and querying analytics (SQLite).
type AnalyticsRepo interface {
	// Record persists a completed query event.
	Record(ctx context.Context, rec QueryRecord) error

	// UpdateFeedback sets the feedback for a query by its hash.
	UpdateFeedback(ctx context.Context, queryHash string, fb Feedback) error

	// Summary returns aggregated stats for the admin dashboard.
	Summary(ctx context.Context, days int) (*AnalyticsSummary, error)
}

// AnalyticsSummary is a read-only projection for the admin dashboard.
type AnalyticsSummary struct {
	TotalQueries     int     `json:"total_queries"`
	BlockedQueries   int     `json:"blocked_queries"`
	PositiveFeedback int     `json:"positive_feedback"`
	NegativeFeedback int     `json:"negative_feedback"`
	AvgResponseMs    float64 `json:"avg_response_ms"`
}
