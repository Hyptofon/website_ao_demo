package commands

import (
	"context"
	"testing"

	"university-chatbot/backend/internal/domain"
)

// mockAnalyticsRepo implements domain.AnalyticsRepo for testing.
type mockAnalyticsRepo struct {
	lastHash     string
	lastFeedback domain.Feedback
}

func (m *mockAnalyticsRepo) Record(_ context.Context, _ domain.QueryRecord) error { return nil }
func (m *mockAnalyticsRepo) UpdateFeedback(_ context.Context, hash string, fb domain.Feedback) error {
	m.lastHash = hash
	m.lastFeedback = fb
	return nil
}
func (m *mockAnalyticsRepo) Summary(_ context.Context, _ int) (*domain.AnalyticsSummary, error) {
	return &domain.AnalyticsSummary{}, nil
}
func (m *mockAnalyticsRepo) TopQueries(_ context.Context, _, _ int) ([]domain.TopQuery, error) {
	return nil, nil
}
func (m *mockAnalyticsRepo) DailyStats(_ context.Context, _ int) ([]domain.DailyStat, error) {
	return nil, nil
}
func (m *mockAnalyticsRepo) FeedbackStats(_ context.Context, _ int) (*domain.FeedbackStat, error) {
	return &domain.FeedbackStat{}, nil
}
func (m *mockAnalyticsRepo) RecentQueries(_ context.Context, _, _ int) ([]domain.QueryRow, error) {
	return nil, nil
}

func TestSubmitFeedback_ValidPositive(t *testing.T) {
	repo := &mockAnalyticsRepo{}
	h := NewSubmitFeedbackHandler(repo)

	err := h.Handle(context.Background(), SubmitFeedbackCommand{
		QueryHash: "abc123",
		Feedback:  domain.FeedbackPositive,
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if repo.lastHash != "abc123" {
		t.Errorf("expected hash=abc123, got %s", repo.lastHash)
	}
	if repo.lastFeedback != domain.FeedbackPositive {
		t.Errorf("expected positive feedback, got %d", repo.lastFeedback)
	}
}

func TestSubmitFeedback_ValidNegative(t *testing.T) {
	repo := &mockAnalyticsRepo{}
	h := NewSubmitFeedbackHandler(repo)

	err := h.Handle(context.Background(), SubmitFeedbackCommand{
		QueryHash: "def456",
		Feedback:  domain.FeedbackNegative,
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestSubmitFeedback_EmptyHash(t *testing.T) {
	repo := &mockAnalyticsRepo{}
	h := NewSubmitFeedbackHandler(repo)

	err := h.Handle(context.Background(), SubmitFeedbackCommand{
		QueryHash: "",
		Feedback:  domain.FeedbackPositive,
	})
	if err == nil {
		t.Error("expected error for empty query hash")
	}
}

func TestSubmitFeedback_WhitespaceHash(t *testing.T) {
	repo := &mockAnalyticsRepo{}
	h := NewSubmitFeedbackHandler(repo)

	err := h.Handle(context.Background(), SubmitFeedbackCommand{
		QueryHash: "   ",
		Feedback:  domain.FeedbackPositive,
	})
	if err == nil {
		t.Error("expected error for whitespace-only query hash")
	}
}

func TestSubmitFeedback_InvalidFeedbackValue(t *testing.T) {
	repo := &mockAnalyticsRepo{}
	h := NewSubmitFeedbackHandler(repo)

	err := h.Handle(context.Background(), SubmitFeedbackCommand{
		QueryHash: "ghi789",
		Feedback:  domain.Feedback(0), // Neither positive nor negative
	})
	if err == nil {
		t.Error("expected error for neutral feedback (0)")
	}
}

func TestSubmitFeedback_InvalidFeedbackNumber(t *testing.T) {
	repo := &mockAnalyticsRepo{}
	h := NewSubmitFeedbackHandler(repo)

	err := h.Handle(context.Background(), SubmitFeedbackCommand{
		QueryHash: "abc",
		Feedback:  domain.Feedback(5),
	})
	if err == nil {
		t.Error("expected error for feedback=5")
	}
}

func TestHashMessage(t *testing.T) {
	h1 := HashMessage("test message")
	h2 := HashMessage("test message")
	h3 := HashMessage("different message")

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
	if len(h1) != 16 {
		t.Errorf("expected 16-char hex hash, got %d chars: %s", len(h1), h1)
	}
}
