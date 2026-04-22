package http

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/auth"
	"university-chatbot/backend/internal/infrastructure/chunker"
	"university-chatbot/backend/internal/infrastructure/gemini"
	_ "university-chatbot/backend/internal/infrastructure/metrics" // register Prometheus metrics
	"university-chatbot/backend/internal/infrastructure/parser"
	"university-chatbot/backend/internal/infrastructure/security"
	"university-chatbot/backend/internal/infrastructure/sqlite"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RouterDeps bundles all dependencies needed to build the HTTP router.
// Moved from positional args to a struct to keep NewRouter readable as deps grow.
type RouterDeps struct {
	ChatHandler      *ChatHandler
	AdminHandler     *AdminHandler
	IndexHandler     *IndexHandler
	RateLimiter      *security.RateLimiter
	AuditRepo        domain.AuditRepo
	JWTService       *auth.JWTService
	AdminToken       string
	AllowedOrigins   []string
	AllowedEmails    []string
	DB               *sql.DB
	AdminSettings    *sqlite.AdminSettingsRepo
	// AdminPathSegment is a 32-char hex derived from SHA256(ADMIN_TOKEN).
	// Used to mount the admin routes at /admin-{segment}/* instead of /admin/*,
	// making the URL unpredictable to external parties (TZ §3.3, §4.2).
	AdminPathSegment string
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
		AllowCredentials: true,
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

	// ── Prometheus Metrics (TZ §8.2) ──────────────────────────────────────────
	r.Handle("/metrics", promhttp.Handler())

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

	// ── Auth Endpoints (public — no auth middleware) ────────────────────────
	// Mounted under the hidden admin path so the OAuth callback URL is also unpredictable.
	adminPrefix := "/admin-" + deps.AdminPathSegment
	if deps.AdminHandler != nil {
		r.Get(adminPrefix+"/auth/login", deps.AdminHandler.HandleLogin)
		r.Get(adminPrefix+"/auth/callback", deps.AdminHandler.HandleCallback)
		r.Post(adminPrefix+"/auth/refresh", deps.AdminHandler.HandleRefreshToken)
	}

	// ── Admin API (protected) ─────────────────────────────────────────────
	// All admin routes are mounted at /admin-{randomHex32} to make the
	// admin surface unpredictable. The path segment is derived from ADMIN_TOKEN.
	r.Route(adminPrefix, func(r chi.Router) {
		// Dual auth: JWT OR Admin-Token
		r.Use(DualAuthMiddleware(deps.JWTService, deps.AdminToken, deps.AllowedEmails, deps.AdminSettings))

		// Audit logging for all admin actions
		if deps.AuditRepo != nil {
			r.Use(AuditMiddleware(deps.AuditRepo))
		}

		// Document management
		if deps.IndexHandler != nil {
			r.Post("/documents/upload", deps.IndexHandler.HandleAdminUpload)
			r.Get("/documents/jobs/{job_id}", deps.IndexHandler.GetJobStatus)
			r.Delete("/documents/{document_id}", deps.IndexHandler.HandleDeleteDocument)
			r.Post("/documents/{document_id}/reindex", deps.IndexHandler.HandleReindexDocument)
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
			r.Get("/analytics/export/csv", deps.AdminHandler.HandleExportCSV)

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
