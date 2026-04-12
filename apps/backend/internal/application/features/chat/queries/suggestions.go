package queries

import (
	"context"
	"log/slog"

	"university-chatbot/backend/internal/domain"
)

// ─── Suggested Questions Handler ────────────────────────────────────────────
// Returns a mix of manually curated and auto-generated question suggestions.
// Auto-generated suggestions come from top queries in analytics.

// SuggestedQuestionsHandler returns relevant question suggestions.
type SuggestedQuestionsHandler struct {
	suggestRepo   domain.SuggestionsRepo
	analyticsRepo domain.AnalyticsRepo
}

// NewSuggestedQuestionsHandler creates the handler.
func NewSuggestedQuestionsHandler(sr domain.SuggestionsRepo, ar domain.AnalyticsRepo) *SuggestedQuestionsHandler {
	return &SuggestedQuestionsHandler{suggestRepo: sr, analyticsRepo: ar}
}

// Handle returns suggested questions for the given language.
func (h *SuggestedQuestionsHandler) Handle(ctx context.Context, lang domain.Language, limit int) ([]domain.SuggestedQuestion, error) {
	if limit <= 0 {
		limit = 5
	}

	questions, err := h.suggestRepo.List(ctx, lang, limit)
	if err != nil {
		return nil, err
	}

	return questions, nil
}

// RefreshAutoSuggestions regenerates auto-suggestions from top queries.
// Should be called periodically (e.g., daily via cron or admin action).
func (h *SuggestedQuestionsHandler) RefreshAutoSuggestions(ctx context.Context, lang domain.Language) error {
	// Get top queries from last 30 days
	topQueries, err := h.analyticsRepo.TopQueries(ctx, 30, 10)
	if err != nil {
		return err
	}

	// Delete old auto-generated suggestions
	if err := h.suggestRepo.DeleteAuto(ctx, lang); err != nil {
		slog.Warn("Failed to delete old auto-suggestions", "error", err)
	}

	// Create new auto-suggestions from top queries
	for i, q := range topQueries {
		if q.Count < 3 {
			continue // Skip queries with too few occurrences
		}
		suggestion := &domain.SuggestedQuestion{
			Question: q.QueryHash, // In production, resolve hash to actual query text
			Language: lang,
			IsAuto:   true,
			Priority: 100 + i, // After manual suggestions
		}
		if err := h.suggestRepo.Upsert(ctx, suggestion); err != nil {
			slog.Warn("Failed to create auto-suggestion", "error", err)
		}
	}

	return nil
}
