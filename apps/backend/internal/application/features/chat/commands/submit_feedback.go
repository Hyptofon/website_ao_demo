package commands

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"university-chatbot/backend/internal/domain"
)

// SubmitFeedbackCommand carries the user's 👍/👎 rating.
type SubmitFeedbackCommand struct {
	QueryHash string          `json:"query_hash"`
	Feedback  domain.Feedback `json:"feedback"`
}

// SubmitFeedbackHandler persists user feedback to analytics.
type SubmitFeedbackHandler struct {
	analytics domain.AnalyticsRepo
}

// NewSubmitFeedbackHandler constructs the handler.
func NewSubmitFeedbackHandler(ar domain.AnalyticsRepo) *SubmitFeedbackHandler {
	return &SubmitFeedbackHandler{analytics: ar}
}

// Handle updates feedback for the given query hash.
func (h *SubmitFeedbackHandler) Handle(ctx context.Context, cmd SubmitFeedbackCommand) error {
	if strings.TrimSpace(cmd.QueryHash) == "" {
		return fmt.Errorf("query_hash must not be empty")
	}
	if cmd.Feedback != domain.FeedbackPositive && cmd.Feedback != domain.FeedbackNegative {
		return fmt.Errorf("feedback must be 1 (positive) or -1 (negative)")
	}
	return h.analytics.UpdateFeedback(ctx, cmd.QueryHash, cmd.Feedback)
}

// HashMessage is a helper that produces the same short hash as AskBotHandler.
func HashMessage(msg string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(msg)))
	return fmt.Sprintf("%x", hash[:8])
}
