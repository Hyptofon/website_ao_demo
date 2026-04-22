package http

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/auth"
)

// ─── Dual Auth Middleware ────────────────────────────────────────────────────
// Supports both JWT (Google OAuth) AND legacy Admin-Token header.
// This ensures backward compatibility while migrating to OAuth.

type contextKey string

const adminEmailKey contextKey = "admin_email"

// DualAuthMiddleware validates admin access via JWT token OR shared Admin-Token.
// JWT takes priority. If neither is valid, returns 401.
func DualAuthMiddleware(jwtSvc *auth.JWTService, adminToken string, allowedEmails []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Try JWT from Authorization: Bearer <token>
			if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				claims, err := jwtSvc.ValidateToken(tokenStr)
				if err == nil {
					// Check email whitelist
					if !isEmailAllowed(claims.Email, allowedEmails) {
						jsonError(w, "forbidden", "Email not in admin whitelist", http.StatusForbidden)
						return
					}
					ctx := context.WithValue(r.Context(), adminEmailKey, claims.Email)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				slog.Debug("JWT validation failed, trying Admin-Token", "error", err)
			}

			// 2. Fallback: X-Admin-Token header (legacy)
			if adminToken != "" {
				token := r.Header.Get("X-Admin-Token")
				if token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) == 1 {
					ctx := context.WithValue(r.Context(), adminEmailKey, "admin@token-auth")
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			jsonError(w, "unauthorized", "Valid JWT or Admin-Token required", http.StatusUnauthorized)
		})
	}
}

// AdminEmailFromCtx extracts the admin email from the request context.
func AdminEmailFromCtx(ctx context.Context) string {
	email, _ := ctx.Value(adminEmailKey).(string)
	if email == "" {
		return "unknown"
	}
	return email
}

// isEmailAllowed checks if an email is in the whitelist.
// Supports both exact email matches ("user@example.com") and domain patterns
// ("@university.edu.ua") per TZ §3.3.
// Empty whitelist = allow all (for development).
func isEmailAllowed(email string, allowed []string) bool {
	if len(allowed) == 0 {
		return true // no whitelist configured → allow all authenticated users
	}
	email = strings.ToLower(strings.TrimSpace(email))
	for _, a := range allowed {
		a = strings.ToLower(strings.TrimSpace(a))
		// Domain-based matching: "@university.edu.ua"
		if strings.HasPrefix(a, "@") && strings.HasSuffix(email, a) {
			return true
		}
		// Exact email match
		if a == email {
			return true
		}
	}
	return false
}

// ─── Audit Logging Middleware ────────────────────────────────────────────────

// statusResponseWriter wraps http.ResponseWriter to capture the HTTP status code.
// This allows AuditMiddleware to record whether an action succeeded or failed,
// enabling accurate audit trails (e.g., skip logging 401/403 failures).
type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusResponseWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusResponseWriter) Status() int {
	if sw.status == 0 {
		return http.StatusOK // WriteHeader was never called → implicit 200
	}
	return sw.status
}

// AuditMiddleware automatically logs admin actions based on HTTP method and path.
// Captures the HTTP response status code to avoid logging failed requests (4xx/5xx).
// Email and path are captured synchronously before the goroutine starts so that
// the request object is not recycled by the time the goroutine accesses those values.
func AuditMiddleware(auditRepo domain.AuditRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			srw := &statusResponseWriter{ResponseWriter: w}

			// Execute the handler first
			next.ServeHTTP(srw, r)

			// Capture values from r synchronously before the goroutine starts.
			// After ServeHTTP returns the request may be garbage-collected.
			action := inferAction(r.Method, r.URL.Path)
			if action == "" {
				return
			}
			// Only audit successful mutating operations.
			// 4xx = client error (not a real admin action), 5xx = server error.
			httpStatus := srw.Status()
			if httpStatus >= 400 {
				return
			}

			adminEmail := AdminEmailFromCtx(r.Context())
			urlPath := r.URL.Path
			ip := realIP(r)

			// Log the action asynchronously (don't slow down the response).
			// Use context.Background() because r.Context() is cancelled after
			// ServeHTTP returns, which would silently abort the DB insert.
			go func() {
				entry := domain.AuditEntry{
					AdminEmail: adminEmail,
					Action:     domain.AdminAction(action),
					Target:     urlPath,
					Details:    fmt.Sprintf("status=%d", httpStatus),
					IP:         ip,
				}
				if err := auditRepo.Record(context.Background(), entry); err != nil {
					slog.Error("Failed to record audit entry", "error", err)
				}
			}()
		})
	}
}


// inferAction maps HTTP method + path to an admin action for auditing.
func inferAction(method, path string) string {
	switch {
	case method == "POST" && strings.Contains(path, "/upload"):
		return string(domain.ActionUploadDocument)
	case method == "DELETE" && strings.Contains(path, "/documents/"):
		return string(domain.ActionDeleteDocument)
	case method == "GET" && strings.Contains(path, "/analytics"):
		return string(domain.ActionViewAnalytics)
	case method == "GET" && strings.Contains(path, "/audit"):
		return string(domain.ActionViewAuditLog)
	default:
		return ""
	}
}

// ─── OAuth CSRF State ────────────────────────────────────────────────────────

// GenerateState creates a cryptographically random state token for OAuth CSRF protection.
func GenerateState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// csrfEntry holds the token value and its creation time for TTL-based cleanup.
type csrfEntry struct {
	createdAt time.Time
}

// csrfStateTTL defines how long an OAuth state token is valid.
// Google's OAuth flow typically completes well within 10 minutes.
const csrfStateTTL = 10 * time.Minute

// csrfStore is a thread-safe in-memory store for OAuth state tokens.
// Uses RWMutex: multiple concurrent reads (ValidateState checks) are safe,
// writes (StoreState, delete) acquire an exclusive lock.
// In production with multiple backend instances, replace with Redis.
var (
	csrfStore   = make(map[string]csrfEntry)
	csrfStoreMu sync.RWMutex
)

func init() {
	// Background cleanup goroutine — evicts expired CSRF tokens every 5 minutes
	// to prevent unbounded memory growth from abandoned OAuth flows.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			csrfStoreMu.Lock()
			cutoff := time.Now().Add(-csrfStateTTL)
			for state, entry := range csrfStore {
				if entry.createdAt.Before(cutoff) {
					delete(csrfStore, state)
				}
			}
			csrfStoreMu.Unlock()
		}
	}()
}

// StoreState saves an OAuth state token with a creation timestamp.
func StoreState(state string) {
	csrfStoreMu.Lock()
	defer csrfStoreMu.Unlock()
	csrfStore[state] = csrfEntry{createdAt: time.Now()}
}

// ValidateState checks and consumes an OAuth state token (one-time use).
// Returns false if the token is unknown or has expired.
func ValidateState(state string) bool {
	csrfStoreMu.Lock()
	defer csrfStoreMu.Unlock()
	entry, ok := csrfStore[state]
	if !ok {
		return false
	}
	// Reject expired tokens even if present
	if time.Since(entry.createdAt) > csrfStateTTL {
		delete(csrfStore, state)
		return false
	}
	delete(csrfStore, state) // one-time use
	return true
}

// stateResponse is used for JSON responses in auth endpoints.
type stateResponse struct {
	URL   string `json:"url,omitempty"`
	Token string `json:"token,omitempty"`
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
}

// authErrorResponse wraps errors from auth endpoints.
type authErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeAuthError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(authErrorResponse{
		Error:   "auth_error",
		Message: msg,
	})
}
