package queries

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/metrics"
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
	memory         domain.ConversationMemory // Phase 3: optional history
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

// WithMemory sets the optional conversation memory (Phase 3).
func (h *AskBotHandler) WithMemory(memory domain.ConversationMemory) *AskBotHandler {
	h.memory = memory
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
	var contextBuf bytes.Buffer
	sources := make([]domain.Source, 0, len(results))
	seenDocs := make(map[string]bool)

	queryWords := tokenizeQuery(req.Message)

	for _, r := range results {
		// Hybrid scoring: accept a chunk if it passes the semantic threshold,
		// OR if it has a moderate score AND contains a keyword from the query.
		// This implements the intended BM25 approximation from the design doc.
		hasKeyword := chunkContainsKeyword(r.Chunk.Text, queryWords)
		if r.Score < 0.7 && !(r.Score >= 0.5 && hasKeyword) {
			continue
		}

		fmt.Fprintf(&contextBuf, "--- Документ: %s (стор. %d) ---\n%s\n\n",
			r.Chunk.DocumentName, r.Chunk.PageNumber, r.Chunk.Text)

		if !seenDocs[r.Chunk.DocumentName] && len(sources) < 15 {
			seenDocs[r.Chunk.DocumentName] = true
			sources = append(sources, domain.Source{
				DocumentName: r.Chunk.DocumentName,
				Score:        r.Score,
				PageNumber:   r.Chunk.PageNumber,
			})
		}
	}

	// --- 4. Guard: if no relevant context — return fallback without calling LLM ---
	if contextBuf.Len() == 0 {
		fallback := domain.FallbackResponseUA
		if req.Language == domain.LangEn {
			fallback = domain.FallbackResponseEN
		}
		fallbackEscaped := strings.ReplaceAll(fallback, "\n", "\\n")
		fmt.Fprintf(w, "data: %s\n\n", fallbackEscaped)
		if f, ok := w.(interface{ Flush() }); ok {
			f.Flush()
		}

		elapsed := time.Since(start).Milliseconds()
		go func() {
			if err := h.analytics.Record(context.Background(), domain.QueryRecord{
				QueryHash:  queryHash,
				Language:   req.Language,
				ResponseMs: elapsed,
				SourcesCnt: 0,
				IsBlocked:  false,
			}); err != nil {
				slog.Error("Failed to record analytics", "error", err)
			}
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
		sysPrompt = domain.SystemPromptUA
		if req.Language == domain.LangEn {
			sysPrompt = domain.SystemPromptEN
		}
	}

	// --- 5.1 Load conversation history ---
	if h.memory != nil {
		hist, err := h.memory.GetHistory(ctx, req.SessionID, 5) // limit to 5 messages
		if err == nil && len(hist) > 0 {
			// SECURITY: Serialize history to JSON to prevent Prompt Injection.
			// Raw string concatenation allows attackers to escape history blocks via formatting.
			histJSON, _ := json.Marshal(hist)
			contextBuf.WriteString("\n\n--- Chat History (JSON) ---\n")
			contextBuf.Write(histJSON)
			contextBuf.WriteString("\n---------------------------\n")
		}
	}


	// --- 5.5. Phase 3: Check response cache before calling LLM ---
	// Cache key = hash of (language + query + context) to avoid cross-language collisions.
	// TTL = 1 hour per TZ §4.2.
	ctxHash := sha256.Sum256([]byte(string(req.Language) + req.Message + contextBuf.String()))
	cacheKey := fmt.Sprintf("resp:%x", ctxHash[:12])

	if h.cache != nil {
		cached, cacheErr := h.cache.Get(ctx, cacheKey)
		if cacheErr == nil && cached != "" {
			metrics.CacheHitsTotal.WithLabelValues("hit").Inc()
			// Cache hit — stream the cached response directly
			cachedEscaped := strings.ReplaceAll(cached, "\n", "\\n")
			fmt.Fprintf(w, "data: %s\n\n", cachedEscaped)
			if f, ok := w.(interface{ Flush() }); ok {
				f.Flush()
			}

			elapsed := time.Since(start).Milliseconds()
			go func() {
				if err := h.analytics.Record(context.Background(), domain.QueryRecord{
					QueryHash:  queryHash,
					Language:   req.Language,
					ResponseMs: elapsed,
					SourcesCnt: len(sources),
					IsBlocked:  false,
				}); err != nil {
					slog.Error("Failed to record analytics", "error", err)
				}
			}()

			return &AskBotResult{
				Sources:   sources,
				QueryHash: queryHash,
				StartedAt: start,
				VariantID: variantID,
			}, nil
		}
	}

	// --- 6. Stream LLM response (capture for caching) ---
	metrics.CacheHitsTotal.WithLabelValues("miss").Inc()
	var capturedResponse strings.Builder
	teeWriter := io.MultiWriter(w, &capturedResponse)

	if err := h.llm.StreamAnswer(ctx, sysPrompt, req.Message, contextBuf.String(), req.Language, teeWriter); err != nil {
		return nil, fmt.Errorf("llm stream: %w", err)
	}

	// --- 6.5. Store response in cache and memory (async, best-effort) ---
	if capturedResponse.Len() > 0 {
		go func() {
			bgCtx := context.Background()
			if h.cache != nil {
				if err := h.cache.Set(bgCtx, cacheKey, capturedResponse.String(), time.Hour); err != nil {
					slog.Warn("Failed to cache response", "error", err)
				}
			}
			if h.memory != nil {
				_ = h.memory.AddMessage(bgCtx, req.SessionID, domain.Message{Role: "user", Content: req.Message})
				_ = h.memory.AddMessage(bgCtx, req.SessionID, domain.Message{Role: "assistant", Content: capturedResponse.String()})
			}
		}()
	}

	elapsed := time.Since(start).Milliseconds()

	// --- 7. Record analytics in background ---
	go func() {
		if err := h.analytics.Record(context.Background(), domain.QueryRecord{
			QueryHash:  queryHash,
			Language:   req.Language,
			ResponseMs: elapsed,
			SourcesCnt: len(sources),
			IsBlocked:  false,
		}); err != nil {
			slog.Error("Failed to record analytics", "error", err)
		}
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

