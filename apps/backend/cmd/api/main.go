package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
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

	// ── Load .env file (ignore error if file missing — env vars may be set externally)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// ── Load config from environment ───────────────────────────────────────────
	cfg := loadConfig()

	ctx := context.Background()

	// ── Infrastructure: Gemini ─────────────────────────────────────────────────
	log.Println("Initialising Gemini client...")
	geminiClient, err := gemini.NewClient(ctx, cfg.GeminiAPIKey)
	if err != nil {
		log.Fatalf("Gemini init: %v", err)
	}
	defer geminiClient.Close()

	// ── Infrastructure: Qdrant ─────────────────────────────────────────────────
	log.Println("Connecting to Qdrant...")
	qdrantClient, err := qdrant.NewClient(ctx, cfg.QdrantURL, cfg.QdrantAPIKey, geminiClient)
	if err != nil {
		log.Fatalf("Qdrant init: %v", err)
	}

	if err := qdrantClient.EnsureCollection(ctx); err != nil {
		log.Fatalf("Qdrant ensure collection: %v", err)
	}
	log.Println("Qdrant collection ready")

	// ── Infrastructure: SQLite analytics ──────────────────────────────────────
	log.Println("Opening SQLite analytics DB...")
	if err := os.MkdirAll(extractDir(cfg.DBPath), 0755); err != nil {
		log.Fatalf("Create DB dir: %v", err)
	}
	analyticsRepo, err := sqlite.NewAnalyticsRepo(cfg.DBPath)
	if err != nil {
		log.Fatalf("SQLite init: %v", err)
	}
	defer analyticsRepo.Close()
	log.Println("SQLite ready at", cfg.DBPath)

	// ── Infrastructure: Security ───────────────────────────────────────────────
	// 10 requests per 5 minutes, spam penalty = 3x
	rateLimiter := security.NewRateLimiter(10, 5*time.Minute, 3)
	offTopicFilter := security.NewOffTopicFilter()

	// ── Document processing ────────────────────────────────────────────────────
	chunkr := chunker.NewChunker()
	pdfExtractor := parser.NewPDFExtractor(geminiClient.RawClient())

	// ── If --index flag is provided: index documents and exit ──────────────────
	if *indexDir != "" {
		log.Printf("Indexing documents from %q ...", *indexDir)
		ih := chathttp.NewIndexHandler(qdrantClient, chunkr, pdfExtractor)
		if err := ih.IndexDocumentsFromDir(ctx, *indexDir); err != nil {
			log.Fatalf("Indexing failed: %v", err)
		}
		log.Println("Indexing complete.")
		return
	}

	// ── Application layer: CQRS Handlers ─────────────────────────────────────
	askBotHandler := queries.NewAskBotHandler(qdrantClient, geminiClient, analyticsRepo)
	feedbackHandler := commands.NewSubmitFeedbackHandler(analyticsRepo)

	// ── Presentation: HTTP Router ─────────────────────────────────────────────
	chatHttp := chathttp.NewChatHandler(askBotHandler, feedbackHandler, rateLimiter, offTopicFilter)
	router := chathttp.NewRouter(chatHttp, qdrantClient, chunkr, pdfExtractor, cfg.AllowedOrigins)

	// ── HTTP Server ────────────────────────────────────────────────────────────
	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second, // long for SSE streaming
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Server listening on http://0.0.0.0%s", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server: %v", err)
	}
}

// ─── Config ───────────────────────────────────────────────────────────────────

type config struct {
	GeminiAPIKey   string
	QdrantURL      string
	QdrantAPIKey   string
	DBPath         string
	Port           string
	AllowedOrigins []string
}

func loadConfig() config {
	return config{
		GeminiAPIKey:   requireEnv("GEMINI_API_KEY"),
		QdrantURL:      getEnvOr("QDRANT_URL", "localhost"),
		QdrantAPIKey:   os.Getenv("QDRANT_API_KEY"),
		DBPath:         getEnvOr("DB_PATH", "./data/analytics.db"),
		Port:           getEnvOr("PORT", "8080"),
		AllowedOrigins: strings.Split(getEnvOr("ALLOWED_ORIGINS", "http://localhost:4321,http://localhost:3000"), ","),
	}
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
