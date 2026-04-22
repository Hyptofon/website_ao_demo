package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"university-chatbot/backend/internal/application/features/chat/commands"
	"university-chatbot/backend/internal/application/features/chat/queries"
	"university-chatbot/backend/internal/infrastructure/auth"
	"university-chatbot/backend/internal/infrastructure/cache"
	"university-chatbot/backend/internal/infrastructure/chunker"
	"university-chatbot/backend/internal/infrastructure/gemini"
	"university-chatbot/backend/internal/infrastructure/parser"
	"university-chatbot/backend/internal/infrastructure/qdrant"
	"university-chatbot/backend/internal/infrastructure/security"
	"university-chatbot/backend/internal/infrastructure/sqlite"
	chathttp "university-chatbot/backend/internal/presentation/http"
)

func main() {
	indexDir := flag.String("index", "", "Index .txt files from this directory and exit")
	flag.Parse()

	// ── Structured Logging (Pattern #2) ──────────────────────────────────────
	logLevel := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// ── Load .env file (ignore error if file missing — env vars may be set externally)
	if err := godotenv.Load(); err != nil {
		slog.Info("No .env file found, using environment variables")
	}

	// ── Load & validate config (Pattern #3) ──────────────────────────────────
	cfg := loadConfig()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	ctx := context.Background()

	// ── Infrastructure: Gemini ─────────────────────────────────────────────────
	slog.Info("Initialising Gemini client...")
	geminiClient, err := gemini.NewClient(ctx, cfg.GeminiAPIKey)
	if err != nil {
		log.Fatalf("Gemini init: %v", err)
	}
	defer geminiClient.Close()

	// ── Infrastructure: Qdrant ─────────────────────────────────────────────────
	slog.Info("Connecting to Qdrant...", "url", cfg.QdrantURL)
	qdrantClient, err := qdrant.NewClient(ctx, cfg.QdrantURL, cfg.QdrantAPIKey, geminiClient)
	if err != nil {
		log.Fatalf("Qdrant init: %v", err)
	}

	if err := qdrantClient.EnsureCollection(ctx); err != nil {
		log.Fatalf("Qdrant ensure collection: %v", err)
	}
	slog.Info("Qdrant collection ready")

	// ── Infrastructure: SQLite DB ──────────────────────────────────────────────
	slog.Info("Opening SQLite DB...", "path", cfg.DBPath)
	if err := os.MkdirAll(extractDir(cfg.DBPath), 0755); err != nil {
		log.Fatalf("Create DB dir: %v", err)
	}
	db, err := sqlite.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("SQLite init: %v", err)
	}
	defer db.Close()
	slog.Info("SQLite ready", "path", cfg.DBPath)

	// ── Infrastructure: Repositories ───────────────────────────────────────────
	analyticsRepo, err := sqlite.NewAnalyticsRepo(db)
	if err != nil {
		log.Fatalf("SQLite analytics repo init: %v", err)
	}
	jobsRepo := sqlite.NewJobRepository(db)
	auditRepo := sqlite.NewAuditRepo(db)
	documentRepo := sqlite.NewDocumentRepo(db)
	promptRepo := sqlite.NewPromptRepo(db)
	suggestionsRepo := sqlite.NewSuggestionsRepo(db)
	settingsRepo := sqlite.NewAdminSettingsRepo(db)

	// ── Infrastructure: Security ───────────────────────────────────────────────
	rateLimiter := security.NewRateLimiter(cfg.RateLimitPerMin, 5*time.Minute, 3)
	offTopicFilter := security.NewOffTopicFilter()

	// ── Infrastructure: Cache (Phase 3 — Redis/Upstash) ────────────────────────
	cacheStore := cache.NewCacheFromEnv(cfg.UpstashRedisURL, cfg.UpstashRedisToken)

	// ── Infrastructure: Auth (Phase 2 — Google OAuth + JWT) ────────────────────
	oauthSvc := auth.NewOAuthService(auth.OAuthConfig{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.OAuthRedirectURL,
	})
	jwtSvc := auth.NewJWTService(cfg.JWTSecret)

	// ── Document processing ────────────────────────────────────────────────────
	chunkr := chunker.NewChunker()
	pdfExtractor := parser.NewPDFExtractor(geminiClient.RawClient())
	metaExtractor := gemini.NewMetadataExtractor(geminiClient.RawClient())

	// ── If --index flag is provided: index documents and exit ──────────────────
	if *indexDir != "" {
		slog.Info("Indexing documents", "dir", *indexDir)
		ih := chathttp.NewIndexHandler(qdrantClient, chunkr, pdfExtractor, jobsRepo, metaExtractor)
		if err := ih.IndexDocumentsFromDir(ctx, *indexDir); err != nil {
			log.Fatalf("Indexing failed: %v", err)
		}
		slog.Info("Indexing complete")
		return
	}

	// ── Application layer: Phase 3 features ──────────────────────────────────
	promptSelector := queries.NewPromptSelector(promptRepo)
	reranker := queries.NewReranker(geminiClient, cfg.EnableReranking)

	// ── Application layer: CQRS Handlers ─────────────────────────────────────
	askBotHandler := queries.NewAskBotHandler(qdrantClient, geminiClient, analyticsRepo).
		WithCache(cacheStore).
		WithPromptSelector(promptSelector).
		WithReranker(reranker)
	feedbackHandler := commands.NewSubmitFeedbackHandler(analyticsRepo)

	// ── Presentation: Handlers ────────────────────────────────────────────────
	chatHttp := chathttp.NewChatHandler(askBotHandler, feedbackHandler, rateLimiter.Ban, offTopicFilter, analyticsRepo)
	indexHandler := chathttp.NewIndexHandlerFull(qdrantClient, chunkr, pdfExtractor, jobsRepo, metaExtractor, documentRepo, auditRepo)
	adminHandler := chathttp.NewAdminHandler(oauthSvc, jwtSvc, analyticsRepo, auditRepo, documentRepo, promptRepo, suggestionsRepo, qdrantClient, cfg.AdminAllowedEmails, cfg.FrontendURL, settingsRepo)

	// ── Presentation: HTTP Router ─────────────────────────────────────────────
	// Derive admin path segment from ADMIN_TOKEN so the admin URL is
	// unpredictable to external parties while remaining deterministic
	// for the configured token value.
	// e.g. ADMIN_TOKEN='dev-test-token-32chars-longenough'
	//   → SHA256[:16] bytes → 32 hex chars
	//   → backend: /admin-7f3a9b2c1d4e5f6a7b8c9d0e1f2a3b4c
	//   → frontend: http://localhost:4321/admin-7f3a9b2c1d4e5f6a7b8c9d0e1f2a3b4c
	adminPathSegment := adminPathFromToken(cfg.AdminToken)
	slog.Info("Admin panel mounted", "path", "/admin-"+adminPathSegment)

	router := chathttp.NewRouter(chathttp.RouterDeps{
		ChatHandler:      chatHttp,
		AdminHandler:     adminHandler,
		IndexHandler:     indexHandler,
		RateLimiter:      rateLimiter,
		AuditRepo:        auditRepo,
		JWTService:       jwtSvc,
		AdminToken:       cfg.AdminToken,
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedEmails:    cfg.AdminAllowedEmails,
		DB:               db,
		AdminSettings:    settingsRepo,
		AdminPathSegment: adminPathSegment,
	})

	// ── HTTP Server ────────────────────────────────────────────────────────────
	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second, // long for SSE streaming
		IdleTimeout:  120 * time.Second,
	}

	// ── Graceful Shutdown (Pattern #1) ───────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("Server listening", "addr", "http://0.0.0.0"+addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server: %v", err)
		}
	}()

	// Block until shutdown signal
	sig := <-quit
	slog.Info("Received shutdown signal, starting graceful shutdown...", "signal", sig.String())

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Forced shutdown", "error", err)
	}

	slog.Info("Server stopped gracefully")
}

// ─── Config (Pattern #3: Validated Configuration) ─────────────────────────────

type config struct {
	GeminiAPIKey       string
	QdrantURL          string
	QdrantAPIKey       string
	DBPath             string
	Port               int
	AllowedOrigins     []string
	AdminToken         string // Legacy: shared secret for admin API
	RateLimitPerMin    int

	// Phase 2: Google OAuth
	GoogleClientID     string
	GoogleClientSecret string
	OAuthRedirectURL   string
	JWTSecret          string
	AdminAllowedEmails []string

	// Phase 3: Upstash Redis
	UpstashRedisURL    string
	UpstashRedisToken  string

	// Phase 3: Feature flags
	EnableReranking    bool
	FrontendURL        string
}

func loadConfig() config {
	port, _ := strconv.Atoi(getEnvOr("PORT", "8080"))
	rateLimit, _ := strconv.Atoi(getEnvOr("RATE_LIMIT_PER_MIN", "10"))

	// Parse admin emails whitelist
	var adminEmails []string
	if emailsStr := os.Getenv("ADMIN_ALLOWED_EMAILS"); emailsStr != "" {
		for _, e := range strings.Split(emailsStr, ",") {
			if trimmed := strings.TrimSpace(e); trimmed != "" {
				adminEmails = append(adminEmails, trimmed)
			}
		}
	}

	return config{
		GeminiAPIKey:       requireEnv("GEMINI_API_KEY"),
		QdrantURL:          getEnvOr("QDRANT_URL", "localhost"),
		QdrantAPIKey:       os.Getenv("QDRANT_API_KEY"),
		DBPath:             getEnvOr("DB_PATH", "./data/analytics.db"),
		Port:               port,
		AllowedOrigins:     strings.Split(getEnvOr("ALLOWED_ORIGINS", "http://localhost:4321,http://localhost:3000"), ","),
		AdminToken:         getEnvOr("ADMIN_TOKEN", ""),
		RateLimitPerMin:    rateLimit,

		// Phase 2: Google OAuth
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		OAuthRedirectURL:   getEnvOr("OAUTH_REDIRECT_URL", "http://localhost:8080/admin/auth/callback"),
		JWTSecret:          getEnvOr("JWT_SECRET", "change-me-in-production-please-32chars"),
		AdminAllowedEmails: adminEmails,

		// Phase 3: Upstash Redis
		UpstashRedisURL:    os.Getenv("UPSTASH_REDIS_REST_URL"),
		UpstashRedisToken:  os.Getenv("UPSTASH_REDIS_REST_TOKEN"),

		// Phase 3: Feature flags
		EnableReranking:    os.Getenv("ENABLE_RERANKING") == "true",
		FrontendURL:        getEnvOr("FRONTEND_URL", "http://localhost:4321/admin"),
	}
}

// Validate checks config values at startup to fail fast on misconfiguration.
func (c *config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("PORT must be 1-65535, got %d", c.Port)
	}
	if len(c.AllowedOrigins) == 0 || (len(c.AllowedOrigins) == 1 && c.AllowedOrigins[0] == "") {
		return fmt.Errorf("ALLOWED_ORIGINS must not be empty")
	}
	if c.RateLimitPerMin < 1 || c.RateLimitPerMin > 1000 {
		return fmt.Errorf("RATE_LIMIT_PER_MIN must be 1-1000, got %d", c.RateLimitPerMin)
	}
	if c.AdminToken == "" && c.GoogleClientID == "" {
		slog.Warn("Neither ADMIN_TOKEN nor GOOGLE_CLIENT_ID is set — admin endpoints are unprotected!")
	}
	if c.JWTSecret == "change-me-in-production-please-32chars" {
		slog.Warn("JWT_SECRET is using default value — please set a secure secret in production!")
	}
	return nil
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("Required env var %q is not set", key)
	}
	return v
}

func getEnvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func extractDir(path string) string {
	idx := strings.LastIndexAny(path, "/\\")
	if idx < 0 {
		return "."
	}
	return path[:idx]
}

// adminPathFromToken derives a 32-char hex string from the ADMIN_TOKEN
// using the first 16 bytes of its SHA-256 hash.
// This makes the admin URL unpredictable to external parties while keeping
// it deterministic for the configured token: any change to ADMIN_TOKEN
// automatically changes the admin URL (no manual secret management needed).
//
// Example:
//
//	ADMIN_TOKEN="dev-test-token..." → /admin-7f3a9b2c1d4e5f6a7b8c9d0e1f2a3b4c
func adminPathFromToken(token string) string {
	if token == "" {
		// Fallback for dev environments without an admin token.
		return "panel"
	}
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:16]) // 16 bytes = 32 hex chars
}
