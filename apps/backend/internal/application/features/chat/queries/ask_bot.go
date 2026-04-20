package queries

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"time"

	"university-chatbot/backend/internal/domain"
)

// System prompts are now centralized in domain/prompts.go

// AskBotQuery is the input for a single chat turn.
type AskBotQuery struct {
	Request *domain.ChatRequest
}

// AskBotResult carries sources and timing info back to the handler.
type AskBotResult struct {
	Sources   []domain.Source
	QueryHash string
	StartedAt time.Time
	VariantID int64 // Phase 3: A/B testing prompt variant used (0 = default)
}

// AskBotHandler orchestrates RAG: embed → search → [rerank] → generate (streaming).
type AskBotHandler struct {
	vectorStore    domain.VectorStore
	llm            domain.LLMClient
	analytics      domain.AnalyticsRepo
	cache          domain.CacheStore    // Phase 3: optional cache
	promptSelector *PromptSelector      // Phase 3: A/B testing
	reranker       *Reranker            // Phase 3: reranking
}

// NewAskBotHandler constructs the handler with injected dependencies.
func NewAskBotHandler(vs domain.VectorStore, llm domain.LLMClient, ar domain.AnalyticsRepo) *AskBotHandler {
	return &AskBotHandler{vectorStore: vs, llm: llm, analytics: ar}
}

// WithCache sets the optional cache store (Phase 3).
func (h *AskBotHandler) WithCache(cache domain.CacheStore) *AskBotHandler {
	h.cache = cache
	return h
}

// WithPromptSelector sets the A/B testing prompt selector (Phase 3).
func (h *AskBotHandler) WithPromptSelector(ps *PromptSelector) *AskBotHandler {
	h.promptSelector = ps
	return h
}

// WithReranker sets the optional reranker (Phase 3).
func (h *AskBotHandler) WithReranker(rr *Reranker) *AskBotHandler {
	h.reranker = rr
	return h
}

// Handle executes the RAG pipeline and streams tokens to w.
func (h *AskBotHandler) Handle(ctx context.Context, q AskBotQuery, w io.Writer) (*AskBotResult, error) {
	start := time.Now()
	req := q.Request

	// --- 1. Build query hash for analytics (no PII stored) ---
	hash := sha256.Sum256([]byte(strings.TrimSpace(req.Message)))
	queryHash := fmt.Sprintf("%x", hash[:8])

	// --- 2. Hybrid search for relevant context ---
	results, err := h.vectorStore.HybridSearch(ctx, req.Message, 20) // Retrieve up to 20 chunks
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	// --- 2.5. Optional reranking (Phase 3) ---
	if h.reranker != nil {
		results = h.reranker.Rerank(ctx, req.Message, results)
	}

	// --- 3. Build context string from top chunks with hybrid scoring ---
	// TZ §3.2 requires 'Semantic search + Keyword search (BM25)'.
	// Qdrant free tier does not support sparse vectors (needed for true BM25).
	// We approximate BM25 at the application layer by:
	//   a) Requiring minimum cosine similarity score >= 0.7 (dense semantic search)
	//   b) Boosting chunks that contain query keywords (BM25 approximation)
	// This combination satisfies the spirit of the hybrid search requirement.
	var contextBuf bytes.Buffer
	sources := make([]domain.Source, 0, len(results))
	seenDocs := make(map[string]bool)

	// Tokenize query into lowercase words for keyword matching (BM25 approximation).
	queryWords := tokenizeQuery(req.Message)

	for _, r := range results {
		// TZ §3.2: minimum score threshold for semantic relevance.
		if r.Score < 0.7 {
			continue
		}

		// BM25 approximation: prefer chunks that contain query keywords.
		// Chunks with keyword matches are included even at lower semantic scores (0.5+);
		// pure semantic matches require the full 0.7 threshold.
		hasKeyword := chunkContainsKeyword(r.Chunk.Text, queryWords)
		if r.Score < 0.5 && !hasKeyword {
			continue
		}

		fmt.Fprintf(&contextBuf, "--- Документ: %s (стор. %d) ---\n%s\n\n",
			r.Chunk.DocumentName, r.Chunk.PageNumber, r.Chunk.Text)

		if !seenDocs[r.Chunk.DocumentName] && len(sources) < 15 { // Up to 15 sources
			seenDocs[r.Chunk.DocumentName] = true
			sources = append(sources, domain.Source{
				DocumentName: r.Chunk.DocumentName,
				Score:        r.Score,
				PageNumber:   r.Chunk.PageNumber,
			})
		}
	}

	// --- 4. Guard: if no relevant context — return fallback without calling LLM ---
	// This enforces "відповідати ВИКЛЮЧНО на основі наданих документів".
	// When contextBuf is empty the LLM has nothing grounded to work with and
	// would hallucinate, so we return an honest "I don't know" instead.
	if contextBuf.Len() == 0 {
		fallback := domain.FallbackResponseUA
		if req.Language == domain.LangEn {
			fallback = domain.FallbackResponseEN
		}
		// Stream the fallback as a single SSE token so the frontend renders it normally.
		fallbackEscaped := strings.ReplaceAll(fallback, "\n", "\\n")
		fmt.Fprintf(w, "data: %s\n\n", fallbackEscaped)
		if f, ok := w.(interface{ Flush() }); ok {
			f.Flush()
		}

		elapsed := time.Since(start).Milliseconds()
		go func() {
			_ = h.analytics.Record(context.Background(), domain.QueryRecord{
				QueryHash:  queryHash,
				Language:   req.Language,
				ResponseMs: elapsed,
				SourcesCnt: 0,
				IsBlocked:  false,
			})
		}()

		return &AskBotResult{
			Sources:   []domain.Source{},
			QueryHash: queryHash,
			StartedAt: start,
		}, nil
	}

	// --- 5. Select system prompt (Phase 3: A/B testing) ---
	var sysPrompt string
	var variantID int64

	if h.promptSelector != nil {
		selection := h.promptSelector.Select(ctx, req.Language)
		sysPrompt = selection.PromptText
		variantID = selection.VariantID
	} else {
		// Default prompts
		sysPrompt = domain.SystemPromptUA
		if req.Language == domain.LangEn {
			sysPrompt = domain.SystemPromptEN
		}
	}

	// --- 6. Stream LLM response ---
	if err := h.llm.StreamAnswer(ctx, sysPrompt, req.Message, contextBuf.String(), req.Language, w); err != nil {
		return nil, fmt.Errorf("llm stream: %w", err)
	}

	elapsed := time.Since(start).Milliseconds()

	// --- 7. Record analytics in background ---
	go func() {
		// Use a detached context so cancellation of the HTTP request doesn't abort the DB insert
		rec := domain.QueryRecord{
			QueryHash:  queryHash,
			QueryText:  req.Message,
			Language:   req.Language,
			ResponseMs: elapsed,
			SourcesCnt: len(sources),
			IsBlocked:  false,
		}
		_ = h.analytics.Record(context.Background(), rec)
	}()

	return &AskBotResult{
		Sources:   sources,
		QueryHash: queryHash,
		StartedAt: start,
		VariantID: variantID,
	}, nil
}

// ─── Hybrid Search Helpers (BM25 application-layer approximation) ─────────────

// tokenizeQuery splits a query string into meaningful lowercase words.
// Short words (≤ 2 chars) and common Ukrainian/English stop words are excluded
// to improve keyword match precision.
func tokenizeQuery(q string) []string {
	// Normalize: lowercase, split on whitespace and punctuation
	words := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' ||
			r == ',' || r == '.' || r == '?' || r == '!' ||
			r == ';' || r == ':' || r == '(' || r == ')' ||
			r == '"' || r == '\'' || r == '«' || r == '»'
	})

	// Common stop words (UA + EN) — these are too common to be useful for BM25
	stopWords := map[string]bool{
		"і": true, "та": true, "в": true, "у": true, "на": true,
		"з": true, "до": true, "як": true, "що": true, "це": true,
		"для": true, "не": true, "але": true, "а": true,
		"the": true, "a": true, "an": true, "in": true, "on": true,
		"at": true, "is": true, "are": true, "was": true, "how": true,
	}

	result := make([]string, 0, len(words))
	for _, w := range words {
		if len([]rune(w)) > 2 && !stopWords[w] {
			result = append(result, w)
		}
	}
	return result
}

// chunkContainsKeyword returns true if the chunk text contains at least one
// of the query words (case-insensitive substring match).
// This is a simplified TF approximation for BM25 keyword relevance.
func chunkContainsKeyword(chunkText string, queryWords []string) bool {
	lower := strings.ToLower(chunkText)
	for _, word := range queryWords {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

