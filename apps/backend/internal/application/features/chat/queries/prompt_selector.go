package queries

import (
	"context"
	"log/slog"
	"math/rand"

	"university-chatbot/backend/internal/domain"
)

// ─── A/B Prompt Selector ────────────────────────────────────────────────────
// Randomly selects an active prompt variant for a given language.
// Falls back to default prompts from domain/prompts.go when no variants exist.

// PromptSelector chooses system prompts from the variant pool.
type PromptSelector struct {
	repo domain.PromptRepo
}

// NewPromptSelector creates a new A/B testing prompt selector.
func NewPromptSelector(repo domain.PromptRepo) *PromptSelector {
	return &PromptSelector{repo: repo}
}

// PromptSelection represents the chosen prompt and its variant ID (for tracking).
type PromptSelection struct {
	PromptText string
	VariantID  int64 // 0 = default (no A/B testing)
}

// Select picks a random active prompt variant for the given language.
// Returns the default system prompt if no variants are available.
func (s *PromptSelector) Select(ctx context.Context, lang domain.Language) PromptSelection {
	if s.repo == nil {
		return defaultPrompt(lang)
	}

	variants, err := s.repo.ActiveVariants(ctx, lang)
	if err != nil {
		slog.Warn("Failed to fetch prompt variants, using default", "error", err)
		return defaultPrompt(lang)
	}

	if len(variants) == 0 {
		return defaultPrompt(lang)
	}

	// Random selection (uniform distribution)
	chosen := variants[rand.Intn(len(variants))]

	// Increment usage count asynchronously
	go func() {
		if err := s.repo.IncrementUsage(context.Background(), chosen.ID); err != nil {
			slog.Warn("Failed to increment prompt usage", "variant_id", chosen.ID, "error", err)
		}
	}()

	return PromptSelection{
		PromptText: chosen.PromptText,
		VariantID:  chosen.ID,
	}
}

// RecordFeedback records a feedback score for the prompt variant used in a chat.
func (s *PromptSelector) RecordFeedback(ctx context.Context, variantID int64, feedback domain.Feedback) {
	if s.repo == nil || variantID == 0 {
		return
	}

	score := float64(feedback) // -1, 0, or 1
	if err := s.repo.RecordScore(ctx, variantID, score); err != nil {
		slog.Warn("Failed to record prompt score", "variant_id", variantID, "error", err)
	}
}

func defaultPrompt(lang domain.Language) PromptSelection {
	if lang == domain.LangEn {
		return PromptSelection{PromptText: domain.SystemPromptEN, VariantID: 0}
	}
	return PromptSelection{PromptText: domain.SystemPromptUA, VariantID: 0}
}
