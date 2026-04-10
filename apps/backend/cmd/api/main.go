package main

import (
	"context"
	"database/sql"
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
	// JSON logs in production, pretty text in dev. Zero external dependencies.
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

	analyticsRepo, err := sqlite.NewAnalyticsRepo(db)
	if err != nil {
		log.Fatalf("SQLite analytics repo init: %v", err)
	}
	jobsRepo := sqlite.NewJobRepository(db)

	// ── Infrastructure: Security ───────────────────────────────────────────────
	rateLimiter := security.NewRateLimiter(cfg.RateLimitPerMin, 5*time.Minute, 3)
	offTopicFilter := security.NewOffTopicFilter()

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

	// ── Application layer: CQRS Handlers ─────────────────────────────────────
	askBotHandler := queries.NewAskBotHandler(qdrantClient, geminiClient, analyticsRepo)
	feedbackHandler := commands.NewSubmitFeedbackHandler(analyticsRepo)

	// ── Presentation: HTTP Router ─────────────────────────────────────────────
	chatHttp := chathttp.NewChatHandler(askBotHandler, feedbackHandler, rateLimiter.Ban, offTopicFilter)
	router := chathttp.NewRouter(chatHttp, qdrantClient, chunkr, pdfExtractor, jobsRepo, metaExtractor, rateLimiter, cfg.AdminToken, cfg.AllowedOrigins, db)

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
	// Listen for SIGTERM/SIGINT in background, serve in foreground.
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
	GeminiAPIKey    string
	QdrantURL       string
	QdrantAPIKey    string
	DBPath          string
	Port            int
	AllowedOrigins  []string
	AdminToken      string // Pattern #5: Admin Auth
	RateLimitPerMin int
}

func loadConfig() config {
	port, _ := strconv.Atoi(getEnvOr("PORT", "8080"))
	rateLimit, _ := strconv.Atoi(getEnvOr("RATE_LIMIT_PER_MIN", "10"))

	return config{
		GeminiAPIKey:    requireEnv("GEMINI_API_KEY"),
		QdrantURL:       getEnvOr("QDRANT_URL", "localhost"),
		QdrantAPIKey:    os.Getenv("QDRANT_API_KEY"),
		DBPath:          getEnvOr("DB_PATH", "./data/analytics.db"),
		Port:            port,
		AllowedOrigins:  strings.Split(getEnvOr("ALLOWED_ORIGINS", "http://localhost:4321,http://localhost:3000"), ","),
		AdminToken:      getEnvOr("ADMIN_TOKEN", ""),
		RateLimitPerMin: rateLimit,
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
	if c.AdminToken == "" {
		slog.Warn("ADMIN_TOKEN is not set — admin endpoints are unprotected!")
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

// Keep the sql.DB reference accessible for legacy code that may need it.
var _ *sql.DB
