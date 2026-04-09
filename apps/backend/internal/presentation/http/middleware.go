package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"university-chatbot/backend/internal/infrastructure/security"
)

// RateLimitMiddleware applies sliding window rate limiting.
func RateLimitMiddleware(rl *security.RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realIP(r)
			allowed, retryAfter := rl.Allow(ip, 1) // Base weight is 1
			if !allowed {
				secs := int(retryAfter.Seconds())
				// Since we don't know the requested language before parsing the body, we default.
				// However, SSE responses require specific formatting.
				if strings.Contains(r.URL.Path, "/stream") {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusTooManyRequests)
					b, _ := json.Marshal(map[string]string{
						"error":   "rate_limit_exceeded",
						"message": fmt.Sprintf("Ви надіслали забагато запитань. Зачекайте %d сек.", secs),
					})
					fmt.Fprintf(w, "event: error\ndata: %s\n\n", b)
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					return
				}
				jsonError(w, "rate_limit_exceeded", fmt.Sprintf("Too many requests. Wait %d seconds.", secs), http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
