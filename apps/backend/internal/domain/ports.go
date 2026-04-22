package domain

import (
	"context"
	"io"
	"time"
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

	// RenameDocumentPayload updates the 'document_name' payload for all points belonging to a document.
	RenameDocumentPayload(ctx context.Context, documentID string, newName string) error
}

// LLMClient is the port for the Large Language Model (Gemini).
type LLMClient interface {
	// Embed returns the embedding vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// StreamAnswer sends the prompt to the LLM and streams tokens to the writer.
	// It closes the writer when done or on error.
	StreamAnswer(ctx context.Context, systemPrompt, userQuery, context string, lang Language, w io.Writer) error

	// GenerateJSON sends a prompt and unmarshals the JSON response into result.
	// Uses Gemini JSON mode (response_mime_type: application/json) for reliable structured output.
	// Inspired by python_service-dev LLMProvider.generate_structured() protocol.
	GenerateJSON(ctx context.Context, prompt string, result any) error
}

// AnalyticsRepo is the port for recording and querying analytics (SQLite).
type AnalyticsRepo interface {
	// Record persists a completed query event.
	Record(ctx context.Context, rec QueryRecord) error

	// UpdateFeedback sets the feedback for a query by its hash.
	UpdateFeedback(ctx context.Context, queryHash string, fb Feedback) error

	// Summary returns aggregated stats for the admin dashboard.
	Summary(ctx context.Context, days int) (*AnalyticsSummary, error)

	// TopQueries returns the most frequent queries in the last N days.
	TopQueries(ctx context.Context, days, limit int) ([]TopQuery, error)

	// DailyStats returns per-day aggregated analytics.
	DailyStats(ctx context.Context, days int) ([]DailyStat, error)

	// FeedbackStats returns aggregated feedback counts and ratio.
	FeedbackStats(ctx context.Context, days int) (*FeedbackStat, error)

	// RecentQueries returns individual query rows for admin inspection.
	RecentQueries(ctx context.Context, days, limit int) ([]QueryRow, error)
}

// ─── Conversation Memory ──────────────────────────────────────────────────────

// ConversationMemory is the port for storing short-term chat history.
type ConversationMemory interface {
	// GetHistory retrieves the last 'limit' messages for a session.
	GetHistory(ctx context.Context, sessionID string, limit int) ([]Message, error)

	// AddMessage appends a message to the session's history.
	AddMessage(ctx context.Context, sessionID string, msg Message) error
}

// AnalyticsSummary is a read-only projection for the admin dashboard.
type AnalyticsSummary struct {
	TotalQueries     int     `json:"total_queries"`
	BlockedQueries   int     `json:"blocked_queries"`
	PositiveFeedback int     `json:"positive_feedback"`
	NegativeFeedback int     `json:"negative_feedback"`
	AvgResponseMs    float64 `json:"avg_response_ms"`
}

// ─── Phase 2: Audit & Document ports ────────────────────────────────────────

// AuditRepo records and retrieves admin audit log entries.
type AuditRepo interface {
	// Record persists an audit log entry.
	Record(ctx context.Context, entry AuditEntry) error

	// List returns paginated audit entries (newest first).
	List(ctx context.Context, offset, limit int) ([]AuditEntry, int, error)
}

// DocumentRepo manages document metadata in the knowledge base.
type DocumentRepo interface {
	// Create inserts a new document record.
	Create(ctx context.Context, doc *DocumentRecord) error

	// List returns all document records (newest first).
	List(ctx context.Context) ([]DocumentRecord, error)

	// GetByID returns a single document by ID.
	GetByID(ctx context.Context, id string) (*DocumentRecord, error)

	// Delete removes a document record by ID.
	Delete(ctx context.Context, id string) error
	
	// Rename updates the filename of a document
	Rename(ctx context.Context, id string, newName string) error

	// UpdateChunkCount sets the final chunk count after indexing.
	UpdateChunkCount(ctx context.Context, id string, count int) error
}

// ─── Phase 3: Cache, A/B Testing, Suggestions ──────────────────────────────

// CacheStore is the port for caching embeddings and responses (Redis/Upstash).
type CacheStore interface {
	// Get retrieves a cached value by key. Returns ("", nil) on miss.
	Get(ctx context.Context, key string) (string, error)

	// Set stores a value with a TTL. ttl=0 means no expiration.
	Set(ctx context.Context, key string, value string, ttl time.Duration) error

	// Delete removes a cached key.
	Delete(ctx context.Context, key string) error
}

// PromptRepo manages prompt variant storage for A/B testing.
type PromptRepo interface {
	// ActiveVariants returns all active prompt variants for a language.
	ActiveVariants(ctx context.Context, lang Language) ([]PromptVariant, error)

	// IncrementUsage atomically increments usage_count for a variant.
	IncrementUsage(ctx context.Context, variantID int64) error

	// RecordScore records feedback score for a variant (updates running average).
	RecordScore(ctx context.Context, variantID int64, score float64) error

	// List returns all prompt variants (for admin panel).
	List(ctx context.Context) ([]PromptVariant, error)

	// Create inserts a new prompt variant.
	Create(ctx context.Context, variant *PromptVariant) error

	// SetActive toggles the is_active flag.
	SetActive(ctx context.Context, id int64, active bool) error

	// Update modifies the text of a prompt variant.
	Update(ctx context.Context, id int64, text string) error

	// Delete removes a prompt variant.
	Delete(ctx context.Context, id int64) error
}

// SuggestionsRepo manages suggested questions.
type SuggestionsRepo interface {
	// List returns active suggestions for a language.
	List(ctx context.Context, lang Language, limit int) ([]SuggestedQuestion, error)

	// Upsert creates or updates a suggested question.
	Upsert(ctx context.Context, q *SuggestedQuestion) error

	// DeleteAuto removes all auto-generated suggestions for refresh.
	DeleteAuto(ctx context.Context, lang Language) error
}
