package gemini

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ─── Retry with Exponential Backoff ──────────────────────────────────────────
// Inspired by python_service-dev/infrastructure/ai/gemini_service.py _with_retry_async()
// Uses tenacity-like pattern: exponential backoff, retryable error detection, context-aware.

const (
	defaultMaxRetries = 3
	minBackoff        = 2 * time.Second
	maxBackoff        = 60 * time.Second
	backoffMultiplier = 2.0
)

// retryableErrors are error substrings that indicate a transient failure.
var retryableErrors = []string{
	"503",
	"429",
	"UNAVAILABLE",
	"RESOURCE_EXHAUSTED",
	"DEADLINE_EXCEEDED",
	"DeadlineExceeded",
	"ServiceUnavailable",
	"InternalServerError",
	"connection reset",
	"connection refused",
	"EOF",
}

// withRetry executes fn with exponential backoff for retryable errors.
// It respects context cancellation and logs each retry attempt.
func withRetry[T any](ctx context.Context, operationName string, maxAttempts int, fn func() (T, error)) (T, error) {
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxRetries
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			if attempt > 1 {
				slog.Info("Retry succeeded", "op", operationName, "attempt", attempt, "max", maxAttempts)
			}
			return result, nil
		}

		lastErr = err

		// Don't retry non-retryable errors
		if !isRetryableError(err) {
			return result, err
		}

		// Don't retry on the last attempt — just return the error
		if attempt == maxAttempts {
			slog.Error("All retry attempts failed", "op", operationName, "attempts", maxAttempts, "error", err)
			break
		}

		// Calculate backoff: min(2^attempt * multiplier, maxBackoff)
		backoffSecs := math.Min(
			math.Pow(backoffMultiplier, float64(attempt)),
			maxBackoff.Seconds(),
		)
		backoff := time.Duration(math.Max(backoffSecs, minBackoff.Seconds())) * time.Second

		slog.Warn("Retrying operation", "op", operationName, "attempt", attempt, "max", maxAttempts, "backoff", backoff, "error", err)

		select {
		case <-ctx.Done():
			var zero T
			return zero, fmt.Errorf("%s: context cancelled during retry: %w", operationName, ctx.Err())
		case <-time.After(backoff):
			// Continue to next attempt
		}
	}

	var zero T
	return zero, fmt.Errorf("%s: all %d attempts failed: %w", operationName, maxAttempts, lastErr)
}

// withRetryVoid is a convenience wrapper for functions that return only an error.
func withRetryVoid(ctx context.Context, operationName string, maxAttempts int, fn func() error) error {
	_, err := withRetry(ctx, operationName, maxAttempts, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

// isRetryableError checks if an error is transient and worth retrying.
// Uses gRPC status codes first (reliable), then falls back to string matching.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check gRPC status codes (more reliable than string matching)
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable, codes.ResourceExhausted,
			codes.DeadlineExceeded, codes.Internal, codes.Aborted:
			return true
		}
	}

	// Fallback: string matching for non-gRPC errors (connection resets, etc.)
	errStr := err.Error()
	for _, marker := range retryableErrors {
		if strings.Contains(errStr, marker) {
			return true
		}
	}
	return false
}
