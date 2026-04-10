package http

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/http"

	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/chunker"
	"university-chatbot/backend/internal/infrastructure/gemini"
	"university-chatbot/backend/internal/infrastructure/parser"
	"university-chatbot/backend/internal/infrastructure/security"
	"university-chatbot/backend/internal/infrastructure/sqlite"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// NewRouter constructs the chi router with all routes and middleware.
func NewRouter(
	chatHandler *ChatHandler,
	vs domain.VectorStore,
	c *chunker.Chunker,
	pe *parser.PDFExtractor,
	jobsRepo *sqlite.JobRepository,
	me *gemini.MetadataExtractor,
	rateLimiter *security.RateLimiter,
	adminToken string,
	allowedOrigins []string,
	db *sql.DB,
) *chi.Mux {
	r := chi.NewRouter()

	// ── Global middleware ──────────────────────────────────────────────────────
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID) // Pattern #4: Request ID tracing
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.CleanPath)
	r.Use(middleware.StripSlashes)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Accept", "X-Admin-Token"},
		AllowCredentials: false,
		MaxAge:           3600,
	}))

	// ── Deep Healthcheck (Pattern #6) ──────────────────────────────────────────
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{"api": "ok"}

		// SQLite ping
		if db != nil {
			if err := db.PingContext(r.Context()); err != nil {
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
		r.With(RateLimitMiddleware(rateLimiter)).Post("/chat/stream", chatHandler.StreamChat)
		r.Post("/feedback", chatHandler.SubmitFeedback)
	})

	indexHandler := NewIndexHandler(vs, c, pe, jobsRepo, me)

	// ── Admin (Pattern #5: Shared secret auth) ────────────────────────────────
	r.Route("/admin", func(r chi.Router) {
		if adminToken != "" {
			r.Use(AdminAuthMiddleware(adminToken))
		}
		r.Post("/documents/upload", indexHandler.HandleAdminUpload)
		r.Get("/documents/jobs/{job_id}", indexHandler.GetJobStatus)
		r.Delete("/documents/{document_id}", indexHandler.HandleDeleteDocument)
	})

	return r
}

// AdminAuthMiddleware validates the X-Admin-Token header against a shared secret.
// Uses constant-time comparison to prevent timing attacks.
func AdminAuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("X-Admin-Token")
			if token == "" {
				jsonError(w, "unauthorized", "Missing X-Admin-Token header", http.StatusUnauthorized)
				return
			}
			if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
				jsonError(w, "unauthorized", "Invalid admin token", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
