package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"university-chatbot/backend/internal/infrastructure/security"
)

// RateLimitMiddleware applies sliding window rate limiting.
func RateLimitMiddleware(rl *security.RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realIP(r)
			
			// Hybrid Rate Limiting: combine IP and Email (if authenticated)
			// This prevents NAT/shared IP false positives for logged-in admins.
			limitKey := ip
			if email := AdminEmailFromCtx(r.Context()); email != "" {
				limitKey = fmt.Sprintf("%s:%s", ip, email)
			}

			allowed, retryAfter := rl.Allow(limitKey, 1) // Base weight is 1
			if !allowed {
				secs := int(retryAfter.Seconds())
				// TZ §6.1: Set standard HTTP Retry-After header
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				// Provide both a human-readable message AND a machine-readable integer
				// so the frontend countdown timer can use retry_after_seconds directly
				// instead of parsing the message string with a regex.
				payload := map[string]interface{}{
					"error":               "rate_limit_exceeded",
					"retry_after_seconds": secs,
					"message":             "Rate limit exceeded",
				}

				if strings.Contains(r.URL.Path, "/stream") {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusTooManyRequests)
					b, _ := json.Marshal(payload)
					fmt.Fprintf(w, "event: error\ndata: %s\n\n", b)
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(payload)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
