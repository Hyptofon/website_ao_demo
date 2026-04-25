package domain

import (
	"errors"
	"time"
)

// ─── Sentinel Errors ─────────────────────────────────────────────────────────

// ErrLLMOverloaded is returned by the LLM client when the upstream API
// responds with 429 (rate limit) or 503 (service unavailable).
// Use errors.Is(err, domain.ErrLLMOverloaded) instead of strings.Contains.
var ErrLLMOverloaded = errors.New("llm_overloaded")

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

// JobStatus represents the state of a background job.
type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

// UploadJob represents an async job to extract and index a document.
type UploadJob struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	Status      JobStatus `json:"status"`
	Error       string    `json:"error,omitempty"`
	Progress    int       `json:"progress"`               // 0-100 percentage
	CurrentStep string    `json:"current_step,omitempty"`  // Human-readable step description
	ChunksCount int       `json:"chunks_count,omitempty"`  // Total chunks produced
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ChatRequest is the inbound DTO from the HTTP layer.
type ChatRequest struct {
	SessionID string   `json:"session_id" validate:"required"`
	Message   string   `json:"message" validate:"required,max=500"`
	Language  Language `json:"language" validate:"required,oneof=uk en"`
}

// QueryRecord is the analytics row persisted to SQLite.
type QueryRecord struct {
	ID          int64     `json:"id"`
	QueryHash   string    `json:"query_hash"`
	QueryText   string    `json:"query_text"`
	Language    Language  `json:"language"`
	ResponseMs  int64     `json:"response_ms"`
	SourcesCnt  int       `json:"sources_cnt"`
	Feedback    Feedback  `json:"feedback"`
	IsBlocked   bool      `json:"is_blocked"`
	CreatedAt   time.Time `json:"created_at"`
}

// QueryRow is a read-only view of a query for admin inspection.
type QueryRow struct {
	QueryHash  string `json:"query_hash"`
	QueryText  string `json:"query_text"`
	Language   string `json:"language"`
	ResponseMs int64  `json:"response_ms"`
	SourcesCnt int    `json:"sources_cnt"`
	Feedback   int    `json:"feedback"`
	IsBlocked  int    `json:"is_blocked"`
	CreatedAt  string `json:"created_at"`
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

	// ErrUnauthorized is returned when the admin is not authenticated.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden is returned when the admin email is not in the whitelist.
	ErrForbidden = errors.New("forbidden: email not in admin whitelist")

	// ErrDocumentNotFound is returned when a document record does not exist.
	ErrDocumentNotFound = errors.New("document not found")

	// ErrAdminAlreadyExists is returned when trying to add a duplicate admin.
	ErrAdminAlreadyExists = errors.New("admin user already exists")

	// ErrAdminNotFound is returned when trying to remove a non-existent admin.
	ErrAdminNotFound = errors.New("admin user not found")
)

// ─── Phase 2: Admin & Audit ─────────────────────────────────────────────────

// AdminAction represents a type of auditable admin operation.
type AdminAction string

const (
	ActionLogin            AdminAction = "login"
	ActionLogout           AdminAction = "logout"
	ActionUploadDocument   AdminAction = "upload_document"
	ActionDeleteDocument   AdminAction = "delete_document"
	ActionRenameDocument   AdminAction = "rename_document"
	ActionReindexDocument  AdminAction = "reindex_document"
	ActionReindexAll       AdminAction = "reindex_all"
	ActionExportCSV        AdminAction = "export_csv"
	ActionViewAnalytics    AdminAction = "view_analytics"
	ActionViewAuditLog     AdminAction = "view_audit_log"
	ActionAddAdmin         AdminAction = "add_admin"
	ActionRemoveAdmin      AdminAction = "remove_admin"
)

// AuditEntry records a single admin action for the audit log.
type AuditEntry struct {
	ID         int64       `json:"id"`
	AdminEmail string      `json:"admin_email"`
	Action     AdminAction `json:"action"`
	Target     string      `json:"target,omitempty"`  // e.g. document filename
	Details    string      `json:"details,omitempty"` // extra context (JSON)
	IP         string      `json:"ip,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}

// DocumentRecord represents a tracked document in the knowledge base.
type DocumentRecord struct {
	ID         string    `json:"id"`
	Filename   string    `json:"filename"`
	DocType    string    `json:"doc_type"`
	Language   Language  `json:"language"`
	ChunkCount int       `json:"chunk_count"`
	Summary    string    `json:"summary,omitempty"`
	UploadedBy string    `json:"uploaded_by"` // admin email
	UploadedAt time.Time `json:"uploaded_at"`
}

// AdminUser represents an authorized administrator.
type AdminUser struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	AddedBy      string    `json:"added_by"`
	AddedAt      time.Time `json:"added_at"`
	TokenVersion int       `json:"-"` // incremented on logout to invalidate refresh tokens
}

// ─── Phase 2: Extended Analytics ─────────────────────────────────────────────

// TopQuery is an aggregated view of the most frequent queries.
type TopQuery struct {
	QueryText string `json:"query_text"`
	Count     int    `json:"count"`
	Language  string `json:"language"`
	LastSeen  string `json:"last_seen"` // RFC3339
}

// DailyStat holds analytics for a single day.
type DailyStat struct {
	Date             string  `json:"date"` // YYYY-MM-DD
	TotalQueries     int     `json:"total_queries"`
	BlockedQueries   int     `json:"blocked_queries"`
	AvgResponseMs    float64 `json:"avg_response_ms"`
	PositiveFeedback int     `json:"positive_feedback"`
	NegativeFeedback int     `json:"negative_feedback"`
}

// FeedbackStat provides a high-level view of user satisfaction.
type FeedbackStat struct {
	Total    int     `json:"total"`
	Positive int     `json:"positive"`
	Negative int     `json:"negative"`
	Ratio    float64 `json:"ratio"` // positive / total, 0-1
}

// ─── Phase 3: A/B Testing & Suggestions ──────────────────────────────────────

// PromptVariant represents a system prompt variant for A/B testing.
type PromptVariant struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`       // e.g. "concise_v2"
	Language   Language `json:"language"`
	PromptText string   `json:"prompt_text"`
	IsActive   bool     `json:"is_active"`
	UsageCount int64    `json:"usage_count"`
	AvgScore   float64  `json:"avg_score"` // avg feedback as -1..1
}

// SuggestedQuestion is a pre-defined or auto-generated question hint.
type SuggestedQuestion struct {
	ID       int64    `json:"id"`
	Question string   `json:"question"`
	Language Language `json:"language"`
	IsAuto   bool     `json:"is_auto"`   // auto-generated from analytics
	Priority int      `json:"priority"` // lower = shown first
}
