package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ─── Prometheus Metrics (TZ §8.2) ──────────────────────────────────────────
// All metrics registered via promauto for automatic collector registration.

var (
	// ChatRequestsTotal counts all incoming chat requests.
	ChatRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "chat_requests_total",
		Help: "Total number of chat requests",
	}, []string{"language", "status"}) // status: "ok", "blocked", "error"

	// ChatBlockedTotal counts queries blocked by off-topic filter or rate limiter.
	ChatBlockedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "chat_blocked_total",
		Help: "Total number of blocked chat requests",
	}, []string{"reason"}) // reason: "offtopic", "ratelimit", "xss"

	// FeedbackTotal counts user feedback submissions.
	FeedbackTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "feedback_total",
		Help: "Total number of feedback submissions",
	}, []string{"type"}) // type: "positive", "negative"

	// ChatLatencySeconds measures end-to-end chat response latency.
	ChatLatencySeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "chat_latency_seconds",
		Help:    "Chat response latency in seconds",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
	}, []string{"language"})

	// CacheHitsTotal counts cache hits and misses for response caching.
	CacheHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cache_hits_total",
		Help: "Total cache hits and misses",
	}, []string{"result"}) // result: "hit", "miss"

	// DocumentUploadsTotal counts document upload attempts.
	DocumentUploadsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "document_uploads_total",
		Help: "Total document upload attempts",
	}, []string{"status"}) // status: "success", "failed"
)
