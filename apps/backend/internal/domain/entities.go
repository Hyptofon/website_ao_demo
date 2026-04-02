package domain

import (
	"errors"
	"time"
)

// ─── Core Domain Entities ────────────────────────────────────────────────────

// Language represents supported UI/query languages.
type Language string

const (
	LangUk Language = "uk"
	LangEn Language = "en"
)

// Feedback represents user rating of a bot response.
type Feedback int

const (
	FeedbackNone     Feedback = 0
	FeedbackPositive Feedback = 1
	FeedbackNegative Feedback = -1
)

// Message is a single turn in the conversation.
type Message struct {
	ID       string    `json:"id"`
	Role     string    `json:"role"` // "user" | "assistant"
	Content  string    `json:"content"`
	Sources  []Source  `json:"sources,omitempty"`
	Language Language  `json:"language"`
	SentAt   time.Time `json:"sent_at"`
}

// Source references a document chunk used in a RAG answer.
type Source struct {
	DocumentName string  `json:"document_name"`
	Score        float32 `json:"score"`
	PageNumber   int     `json:"page_number,omitempty"`
}

// ChatRequest is the inbound DTO from the HTTP layer.
type ChatRequest struct {
	SessionID string   `json:"session_id"`
	Message   string   `json:"message"`
	Language  Language `json:"language"`
}

// QueryRecord is the analytics row persisted to SQLite.
type QueryRecord struct {
	ID          int64     `json:"id"`
	QueryHash   string    `json:"query_hash"`
	Language    Language  `json:"language"`
	ResponseMs  int64     `json:"response_ms"`
	SourcesCnt  int       `json:"sources_cnt"`
	Feedback    Feedback  `json:"feedback"`
	IsBlocked   bool      `json:"is_blocked"`
	CreatedAt   time.Time `json:"created_at"`
}

// Document represents an indexed knowledge-base document.
type Document struct {
	ID         string    `json:"id"`
	Filename   string    `json:"filename"`
	DocType    string    `json:"doc_type"`
	Language   Language  `json:"language"`
	UploadedAt time.Time `json:"uploaded_at"`
	ChunkCount int       `json:"chunk_count"`
}

// Chunk is a text fragment ready for embedding / Qdrant upsert.
type Chunk struct {
	ID           string            `json:"id"`
	DocumentID   string            `json:"document_id"`
	DocumentName string            `json:"document_name"`
	Text         string            `json:"text"`
	PageNumber   int               `json:"page_number"`
	Metadata     map[string]string `json:"metadata"`
}

// SearchResult wraps a retrieved chunk with its relevance score.
type SearchResult struct {
	Chunk Chunk
	Score float32
}

// ─── Domain Errors ────────────────────────────────────────────────────────────

var (
	// ErrOffTopic is returned when a query is detected as off-topic.
	ErrOffTopic = errors.New("query is off-topic")

	// ErrRateLimited is returned when the IP has exceeded its request quota.
	ErrRateLimited = errors.New("rate limit exceeded")

	// ErrInputTooLong is returned when the query exceeds 500 characters.
	ErrInputTooLong = errors.New("input exceeds maximum length")

	// ErrEmptyInput is returned when the query is blank.
	ErrEmptyInput = errors.New("input must not be empty")

	// ErrNoContext is returned when Qdrant returns no relevant chunks.
	ErrNoContext = errors.New("no relevant context found")
)
