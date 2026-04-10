package gemini

import (
	"context"
	"errors"
	"testing"
)

func TestWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	callCount := 0
	result, err := withRetry(context.Background(), "test-op", 3, func() (string, error) {
		callCount++
		return "ok", nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestWithRetry_SuccessOnSecondAttempt(t *testing.T) {
	callCount := 0
	result, err := withRetry(context.Background(), "test-op", 3, func() (string, error) {
		callCount++
		if callCount == 1 {
			return "", errors.New("503 Service Unavailable")
		}
		return "recovered", nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "recovered" {
		t.Errorf("expected 'recovered', got %q", result)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestWithRetry_AllAttemptsFail(t *testing.T) {
	callCount := 0
	_, err := withRetry(context.Background(), "test-op", 2, func() (string, error) {
		callCount++
		return "", errors.New("429 RESOURCE_EXHAUSTED")
	})

	if err == nil {
		t.Error("expected error after all attempts fail")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestWithRetry_NonRetryableError(t *testing.T) {
	callCount := 0
	_, err := withRetry(context.Background(), "test-op", 3, func() (string, error) {
		callCount++
		return "", errors.New("invalid argument: document is empty")
	})

	if err == nil {
		t.Error("expected error")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (no retry), got %d", callCount)
	}
}

func TestWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	callCount := 0
	_, err := withRetry(ctx, "test-op", 3, func() (string, error) {
		callCount++
		return "", errors.New("503 Server Error")
	})

	if err == nil {
		t.Error("expected error when context is cancelled")
	}
	// Should have attempted once, then context cancelled
	if callCount == 0 {
		t.Error("expected at least 1 call attempt")
	}
}

func TestWithRetryVoid_Success(t *testing.T) {
	err := withRetryVoid(context.Background(), "test-op", 3, func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestWithRetryVoid_RetryableFailure(t *testing.T) {
	count := 0
	err := withRetryVoid(context.Background(), "test-op", 2, func() error {
		count++
		return errors.New("UNAVAILABLE")
	})
	if err == nil {
		t.Error("expected error")
	}
	if count != 2 {
		t.Errorf("expected 2 calls, got %d", count)
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		err      string
		expected bool
	}{
		{"503 Service Unavailable", true},
		{"429 RESOURCE_EXHAUSTED", true},
		{"UNAVAILABLE", true},
		{"DEADLINE_EXCEEDED", true},
		{"connection reset by peer", true},
		{"EOF", true},
		{"invalid argument", false},
		{"document is empty", false},
		{"", false},
	}

	for _, tt := range tests {
		result := isRetryableError(errors.New(tt.err))
		if result != tt.expected {
			t.Errorf("isRetryableError(%q) = %v, want %v", tt.err, result, tt.expected)
		}
	}
}

func TestIsRetryableError_Nil(t *testing.T) {
	if isRetryableError(nil) {
		t.Error("nil error should not be retryable")
	}
}
