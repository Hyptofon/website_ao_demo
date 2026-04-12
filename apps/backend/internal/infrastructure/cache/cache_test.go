package cache

import (
	"context"
	"testing"
	"time"
)

func TestNoopCache_AlwaysMisses(t *testing.T) {
	c := NewNoopCache()
	ctx := context.Background()

	// Set should succeed silently
	if err := c.Set(ctx, "key", "value", time.Minute); err != nil {
		t.Errorf("NoopCache.Set should not error: %v", err)
	}

	// Get should always return empty (cache miss)
	val, err := c.Get(ctx, "key")
	if err != nil {
		t.Errorf("NoopCache.Get should not error: %v", err)
	}
	if val != "" {
		t.Errorf("NoopCache.Get should return empty, got %q", val)
	}

	// Delete should succeed silently
	if err := c.Delete(ctx, "key"); err != nil {
		t.Errorf("NoopCache.Delete should not error: %v", err)
	}
}

func TestExtractResult(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{"string_value", `{"result":"hello world"}`, "hello world"},
		{"null_value", `{"result":null}`, ""},
		{"int_value", `{"result":1}`, "1"},
		{"empty_string", `{"result":""}`, ""},
		{"complex_string", `{"result":"key:value"}`, "key:value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractResult(tt.body)
			if result != tt.expected {
				t.Errorf("extractResult(%q) = %q, want %q", tt.body, result, tt.expected)
			}
		})
	}
}

func TestNewCacheFromEnv_NoopFallback(t *testing.T) {
	cache := NewCacheFromEnv("", "")
	_, ok := cache.(*NoopCache)
	if !ok {
		t.Error("expected NoopCache when Redis URL is empty")
	}
}

func TestNewCacheFromEnv_RedisWhenConfigured(t *testing.T) {
	cache := NewCacheFromEnv("https://example.upstash.io", "test-token")
	_, ok := cache.(*RedisCache)
	if !ok {
		t.Error("expected RedisCache when URL and token are provided")
	}
}
