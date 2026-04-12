package cache

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ─── Upstash Redis Cache (REST API) ─────────────────────────────────────────
// Uses Upstash REST API instead of go-redis to avoid CGO and minimize deps.
// Upstash REST is ideal for serverless/edge — works over HTTPS, no TCP pool needed.

// RedisCache implements domain.CacheStore using Upstash Redis REST API.
type RedisCache struct {
	baseURL    string // e.g. https://possible-lamb-80062.upstash.io
	token      string
	httpClient *http.Client
}

// NewRedisCache creates a new Upstash Redis cache client.
func NewRedisCache(restURL, restToken string) *RedisCache {
	return &RedisCache{
		baseURL: strings.TrimRight(restURL, "/"),
		token:   restToken,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Get retrieves a cached value by key. Returns ("", nil) on cache miss.
func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	body, err := c.command(ctx, "GET", key)
	if err != nil {
		return "", err
	}
	// Upstash returns {"result":null} for missing keys
	if body == "" || body == "null" {
		return "", nil
	}
	return body, nil
}

// Set stores a value with an optional TTL.
func (c *RedisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if ttl > 0 {
		_, err := c.command(ctx, "SET", key, value, "EX", fmt.Sprintf("%d", int(ttl.Seconds())))
		return err
	}
	_, err := c.command(ctx, "SET", key, value)
	return err
}

// Delete removes a cached key.
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	_, err := c.command(ctx, "DEL", key)
	return err
}

// command executes a single Redis command via Upstash REST API.
func (c *RedisCache) command(ctx context.Context, args ...string) (string, error) {
	// Build URL: BASE_URL/command/arg1/arg2/...
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = url.PathEscape(arg)
	}
	endpoint := c.baseURL + "/" + strings.Join(parts, "/")

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("redis: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("redis: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("redis: status %d: %s", resp.StatusCode, string(body))
	}

	// Parse Upstash response: {"result":"value"} or {"result":null}
	result := extractResult(string(body))
	return result, nil
}

// extractResult extracts the "result" field from Upstash JSON response.
// Minimal parsing to avoid json.Unmarshal overhead for simple string values.
func extractResult(body string) string {
	// Fast path: {"result":"value"}
	prefix := `{"result":"`
	if strings.HasPrefix(body, prefix) {
		end := strings.LastIndex(body, `"`)
		if end > len(prefix) {
			return body[len(prefix):end]
		}
	}
	// null result (cache miss)
	if strings.Contains(body, `"result":null`) {
		return ""
	}
	// Integer/boolean result
	if strings.HasPrefix(body, `{"result":`) {
		start := len(`{"result":`)
		end := strings.Index(body[start:], "}")
		if end > 0 {
			return body[start : start+end]
		}
	}
	return ""
}

// ─── NoopCache (fallback when Redis is not configured) ──────────────────────

// NoopCache is a no-op cache that always misses. Used when Redis is not configured.
type NoopCache struct{}

func NewNoopCache() *NoopCache { return &NoopCache{} }

func (c *NoopCache) Get(_ context.Context, _ string) (string, error)                 { return "", nil }
func (c *NoopCache) Set(_ context.Context, _, _ string, _ time.Duration) error        { return nil }
func (c *NoopCache) Delete(_ context.Context, _ string) error                          { return nil }

// NewCacheFromEnv creates the appropriate cache implementation based on config.
// Returns NoopCache if Redis is not configured — the system works fine without it.
func NewCacheFromEnv(redisURL, redisToken string) interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
} {
	if redisURL == "" || redisToken == "" {
		slog.Info("Redis not configured, using NoopCache (no caching)")
		return NewNoopCache()
	}
	slog.Info("Connecting to Upstash Redis", "url", redisURL)
	return NewRedisCache(redisURL, redisToken)
}
