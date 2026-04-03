package http

import (
	"encoding/json"
	"net/http"

	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/chunker"
	"university-chatbot/backend/internal/infrastructure/parser"
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
	allowedOrigins []string,
) *chi.Mux {
	r := chi.NewRouter()

	// ── Global middleware ──────────────────────────────────────────────────────
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.CleanPath)
	r.Use(middleware.StripSlashes)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Accept"},
		AllowCredentials: false,
		MaxAge:           3600,
	}))

	// ── Health check ───────────────────────────────────────────────────────────
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// ── Public Chat API ────────────────────────────────────────────────────────
	r.Route("/api/v1", func(r chi.Router) {
		// SSE: POST body carries the query, response is streamed
		r.Post("/chat/stream", chatHandler.StreamChat)
		r.Post("/feedback", chatHandler.SubmitFeedback)
	})

	// ── Admin (Phase 1: no OAuth, just a shared secret header) ────────────────
	r.Route("/admin", func(r chi.Router) {
		r.Post("/documents/upload", AdminUploadHandler(vs, c, pe))
	})

	return r
}
