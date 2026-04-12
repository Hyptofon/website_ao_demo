package queries

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"university-chatbot/backend/internal/domain"
)

// ─── Reranker ───────────────────────────────────────────────────────────────
// Post-retrieval reranking using Gemini as a cross-encoder.
// Evaluates query+chunk relevance and re-orders results by score.
// Falls back to original ranking on error (graceful degradation).

// Reranker re-scores search results using the LLM for improved relevance.
type Reranker struct {
	llm     domain.LLMClient
	enabled bool
}

// NewReranker creates a reranker. Set enabled=false to skip reranking entirely.
func NewReranker(llm domain.LLMClient, enabled bool) *Reranker {
	return &Reranker{llm: llm, enabled: enabled}
}

// rerankResult holds a score assigned by the LLM.
type rerankResult struct {
	Index int     `json:"index"`
	Score float64 `json:"score"`
}

type rerankResponse struct {
	Rankings []rerankResult `json:"rankings"`
}

// Rerank re-scores the search results based on query relevance.
// Returns the same results, re-ordered by the LLM's relevance score.
// On error, returns original results unchanged (graceful fallback).
func (r *Reranker) Rerank(ctx context.Context, query string, results []domain.SearchResult) []domain.SearchResult {
	if !r.enabled || r.llm == nil || len(results) <= 1 {
		return results
	}

	// Build reranking prompt
	var sb strings.Builder
	sb.WriteString("Rate the relevance of each document excerpt to the user's question.\n")
	sb.WriteString("Return JSON: {\"rankings\": [{\"index\": 0, \"score\": 0.95}, ...]}\n")
	sb.WriteString("Score 0.0 = irrelevant, 1.0 = perfectly relevant.\n\n")
	sb.WriteString(fmt.Sprintf("Question: %s\n\n", query))

	for i, res := range results {
		// Limit excerpt to avoid token overflow
		excerpt := res.Chunk.Text
		if len(excerpt) > 500 {
			excerpt = excerpt[:500] + "..."
		}
		sb.WriteString(fmt.Sprintf("--- Document %d: %s ---\n%s\n\n", i, res.Chunk.DocumentName, excerpt))
	}

	var response rerankResponse
	if err := r.llm.GenerateJSON(ctx, sb.String(), &response); err != nil {
		slog.Warn("Reranking failed, using original order", "error", err)
		return results
	}

	if len(response.Rankings) == 0 {
		return results
	}

	// Build index→score lookup
	scores := make(map[int]float64)
	for _, rr := range response.Rankings {
		if rr.Index >= 0 && rr.Index < len(results) {
			scores[rr.Index] = rr.Score
		}
	}

	// Sort by LLM score (descending), using simple insertion sort for small N
	reranked := make([]domain.SearchResult, len(results))
	copy(reranked, results)

	for i := 1; i < len(reranked); i++ {
		for j := i; j > 0; j-- {
			scoreJ := scores[j]
			scoreJM1 := scores[j-1]
			if scoreJ > scoreJM1 {
				reranked[j], reranked[j-1] = reranked[j-1], reranked[j]
				// Swap mapping too
				scores[j], scores[j-1] = scores[j-1], scores[j]
			}
		}
	}

	slog.Debug("Reranking complete", "original_top", results[0].Chunk.DocumentName,
		"reranked_top", reranked[0].Chunk.DocumentName)

	return reranked
}
