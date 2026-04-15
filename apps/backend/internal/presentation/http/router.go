package http

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/auth"
	"university-chatbot/backend/internal/infrastructure/chunker"
	"university-chatbot/backend/internal/infrastructure/gemini"
	"university-chatbot/backend/internal/infrastructure/parser"
	"university-chatbot/backend/internal/infrastructure/security"
	"university-chatbot/backend/internal/infrastructure/sqlite"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// RouterDeps bundles all dependencies needed to build the HTTP router.
// Moved from positional args to a struct to keep NewRouter readable as deps grow.
type RouterDeps struct {
	ChatHandler    *ChatHandler
	AdminHandler   *AdminHandler
	IndexHandler   *IndexHandler
	RateLimiter    *security.RateLimiter
	AuditRepo      domain.AuditRepo
	JWTService     *auth.JWTService
	AdminToken     string
	AllowedOrigins []string
	AllowedEmails  []string
	DB             *sql.DB
}

// NewRouter constructs the chi router with all routes and middleware.
func NewRouter(deps RouterDeps) *chi.Mux {
	r := chi.NewRouter()

	// ── Global middleware ──────────────────────────────────────────────────────
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID) // Pattern #4: Request ID tracing
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.CleanPath)
	r.Use(middleware.StripSlashes)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   deps.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Accept", "X-Admin-Token"},
		AllowCredentials: false,
		MaxAge:           3600,
	}))

	// ── Deep Healthcheck (Pattern #6) ──────────────────────────────────────────
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{"api": "ok"}

		// SQLite ping
		if deps.DB != nil {
			if err := deps.DB.PingContext(r.Context()); err != nil {
				checks["sqlite"] = "error: " + err.Error()
			} else {
				checks["sqlite"] = "ok"
			}
		}

		// Determine overall status
		status := http.StatusOK
		for _, v := range checks {
			if v != "ok" {
				status = http.StatusServiceUnavailable
				break
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(checks)
	})

	// ── Public Chat API ────────────────────────────────────────────────────────
	r.Route("/api/v1", func(r chi.Router) {
		// Rate limit middleware applies specifically to the chat endpoints
		r.With(RateLimitMiddleware(deps.RateLimiter)).Post("/chat/stream", deps.ChatHandler.StreamChat)
		r.Post("/feedback", deps.ChatHandler.SubmitFeedback)

		// Public suggestions endpoint (no auth needed)
		if deps.AdminHandler != nil {
			r.Get("/suggestions", deps.AdminHandler.HandleListSuggestions)
		}
	})

	// ── Auth Endpoints (public — no auth middleware) ────────────────────────────
	if deps.AdminHandler != nil {
		r.Get("/admin/auth/login", deps.AdminHandler.HandleLogin)
		r.Get("/admin/auth/callback", deps.AdminHandler.HandleCallback)
	}

	// ── Admin API (protected) ──────────────────────────────────────────────────
	r.Route("/admin", func(r chi.Router) {
		// Dual auth: JWT OR Admin-Token
		r.Use(DualAuthMiddleware(deps.JWTService, deps.AdminToken, deps.AllowedEmails))

		// Audit logging for all admin actions
		if deps.AuditRepo != nil {
			r.Use(AuditMiddleware(deps.AuditRepo))
		}

		// Document management
		if deps.IndexHandler != nil {
			r.Post("/documents/upload", deps.IndexHandler.HandleAdminUpload)
			r.Get("/documents/jobs/{job_id}", deps.IndexHandler.GetJobStatus)
			r.Delete("/documents/{document_id}", deps.IndexHandler.HandleDeleteDocument)
		}

		// Admin panel endpoints
		if deps.AdminHandler != nil {
			// Documents
			r.Get("/documents", deps.AdminHandler.HandleListDocuments)
			r.Get("/documents/{id}/download", deps.AdminHandler.HandleDownloadDocument)
			r.Patch("/documents/{id}/rename", deps.AdminHandler.HandleRenameDocument)

			// Analytics
			r.Get("/analytics/summary", deps.AdminHandler.HandleAnalyticsSummary)
			r.Get("/analytics/daily", deps.AdminHandler.HandleDailyStats)
			r.Get("/analytics/top-queries", deps.AdminHandler.HandleTopQueries)
			r.Get("/analytics/feedback", deps.AdminHandler.HandleFeedbackStats)

			// User queries (individual rows)
			r.Get("/queries", deps.AdminHandler.HandleRecentQueries)

			// Audit log
			r.Get("/audit", deps.AdminHandler.HandleAuditLog)

			// A/B testing prompts
			r.Get("/prompts", deps.AdminHandler.HandleListPrompts)
			r.Post("/prompts", deps.AdminHandler.HandleCreatePrompt)
			r.Patch("/prompts/{id}/active", deps.AdminHandler.HandleTogglePromptActive)
			r.Patch("/prompts/{id}", deps.AdminHandler.HandleUpdatePrompt)
			r.Delete("/prompts/{id}", deps.AdminHandler.HandleDeletePrompt)

			// Suggested questions
			r.Get("/suggestions", deps.AdminHandler.HandleListSuggestions)
			r.Post("/suggestions", deps.AdminHandler.HandleCreateSuggestion)
		}
	})

	return r
}

// NewIndexHandler constructs the handler (moved from old router.go inline creation).
func NewIndexHandlerFull(
	vs domain.VectorStore,
	c *chunker.Chunker,
	pe *parser.PDFExtractor,
	jobsRepo *sqlite.JobRepository,
	me *gemini.MetadataExtractor,
	docRepo domain.DocumentRepo,
	auditRepo domain.AuditRepo,
) *IndexHandler {
	return &IndexHandler{
		vectorStore:   vs,
		chunker:       c,
		pdfExtractor:  pe,
		jobsRepo:      jobsRepo,
		metaExtractor: me,
		documentRepo:  docRepo,
		auditRepo:     auditRepo,
	}
}
