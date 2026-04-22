package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/auth"
)

// ─── Admin Handler ──────────────────────────────────────────────────────────
// Handles all admin panel API endpoints: auth, analytics, documents, audit log.

// AdminHandler manages admin-facing HTTP endpoints.
type AdminHandler struct {
	oauthSvc      *auth.OAuthService
	jwtSvc        *auth.JWTService
	analyticsRepo domain.AnalyticsRepo
	auditRepo     domain.AuditRepo
	documentRepo  domain.DocumentRepo
	promptRepo    domain.PromptRepo
	suggestRepo   domain.SuggestionsRepo
	vectorStore   domain.VectorStore
	allowedEmails []string
	frontendURL   string // URL of the frontend admin page for OAuth redirect
}

// NewAdminHandler constructs the admin handler with all dependencies.
func NewAdminHandler(
	oauthSvc *auth.OAuthService,
	jwtSvc *auth.JWTService,
	analyticsRepo domain.AnalyticsRepo,
	auditRepo domain.AuditRepo,
	documentRepo domain.DocumentRepo,
	promptRepo domain.PromptRepo,
	suggestRepo domain.SuggestionsRepo,
	vectorStore domain.VectorStore,
	allowedEmails []string,
	frontendURL string,
) *AdminHandler {
	if frontendURL == "" {
		frontendURL = "http://localhost:4321/admin"
	}
	return &AdminHandler{
		oauthSvc:      oauthSvc,
		jwtSvc:        jwtSvc,
		analyticsRepo: analyticsRepo,
		auditRepo:     auditRepo,
		documentRepo:  documentRepo,
		promptRepo:    promptRepo,
		suggestRepo:   suggestRepo,
		vectorStore:   vectorStore,
		allowedEmails: allowedEmails,
		frontendURL:   frontendURL,
	}
}

// ─── Auth Endpoints ─────────────────────────────────────────────────────────

// HandleLogin redirects to Google OAuth consent screen.
// GET /admin/auth/login
func (h *AdminHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	state := GenerateState()
	StoreState(state)

	authURL := h.oauthSvc.GetAuthURL(state)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stateResponse{URL: authURL})
}

// HandleCallback processes the OAuth callback and issues a JWT.
// GET /admin/auth/callback
func (h *AdminHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// Validate CSRF state
	state := r.URL.Query().Get("state")
	if !ValidateState(state) {
		writeAuthError(w, "Invalid or expired OAuth state", http.StatusBadRequest)
		return
	}

	// Exchange code for access token
	code := r.URL.Query().Get("code")
	if code == "" {
		writeAuthError(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	accessToken, err := h.oauthSvc.ExchangeCode(r.Context(), code)
	if err != nil {
		slog.Error("OAuth code exchange failed", "error", err)
		writeAuthError(w, "Failed to exchange authorization code", http.StatusInternalServerError)
		return
	}

	// Get user info
	userInfo, err := h.oauthSvc.GetUserInfo(r.Context(), accessToken)
	if err != nil {
		slog.Error("OAuth get user info failed", "error", err)
		writeAuthError(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}

	// Check email whitelist
	if !isEmailAllowed(userInfo.Email, h.allowedEmails) {
		slog.Warn("OAuth login rejected: email not in whitelist", "email", userInfo.Email)
		writeAuthError(w, "Access denied: email not authorized", http.StatusForbidden)
		return
	}

	// Generate JWT
	token, err := h.jwtSvc.GenerateToken(userInfo)
	if err != nil {
		slog.Error("JWT generation failed", "error", err)
		writeAuthError(w, "Failed to generate access token", http.StatusInternalServerError)
		return
	}

	// Generate refresh token (30 days, TZ §3.3)
	refreshToken, err := h.jwtSvc.GenerateRefreshToken(userInfo)
	if err != nil {
		slog.Error("Refresh token generation failed", "error", err)
		// Non-fatal: proceed without refresh token
	}

	// Record login in audit log.
	// Use context.Background() — r.Context() is cancelled when the HTTP handler
	// returns, which would cause the DB insert to silently fail.
	if h.auditRepo != nil {
		go func() {
			_ = h.auditRepo.Record(context.Background(), domain.AuditEntry{
				AdminEmail: userInfo.Email,
				Action:     domain.ActionLogin,
				Target:     "oauth",
				IP:         realIP(r),
			})
		}()
	}

	slog.Info("Admin login successful", "email", userInfo.Email)

	// Set refresh token as httpOnly cookie (not accessible via JS)
	if refreshToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    refreshToken,
			Path:     "/",
			MaxAge:   30 * 24 * 3600, // 30 days
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	// Redirect to frontend admin page with the token in the URL fragment.
	//
	// SECURITY: We intentionally use a URL fragment (#token=...) instead of a
	// query parameter (?token=...). Fragments are:
	//   1. Never sent to any server (the browser strips them before sending HTTP requests)
	//   2. Not included in server access logs
	//   3. Not sent in the Referer header to third-party resources
	//
	// The frontend reads window.location.hash on mount, stores the token in
	// localStorage, and immediately clears the fragment via history.replaceState.
	redirectURL, err := url.Parse(h.frontendURL)
	if err != nil {
		slog.Error("Invalid frontend URL", "url", h.frontendURL, "error", err)
		writeAuthError(w, "Invalid redirect URL configured", http.StatusInternalServerError)
		return
	}
	redirectURL.Fragment = "token=" + token

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// HandleRefreshToken exchanges a valid refresh token for a new access token.
// POST /admin/auth/refresh
// TZ §3.3: «refresh-token на 30 днів»
func (h *AdminHandler) HandleRefreshToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		jsonError(w, "missing_refresh_token", "No refresh token provided", http.StatusUnauthorized)
		return
	}

	claims, err := h.jwtSvc.ValidateRefreshToken(cookie.Value)
	if err != nil {
		jsonError(w, "invalid_refresh_token", "Refresh token is invalid or expired", http.StatusUnauthorized)
		return
	}

	// Check email whitelist
	if !isEmailAllowed(claims.Email, h.allowedEmails) {
		jsonError(w, "forbidden", "Email no longer authorized", http.StatusForbidden)
		return
	}

	// Generate new access token
	newToken, err := h.jwtSvc.GenerateToken(&auth.GoogleUserInfo{
		Email: claims.Email,
		Name:  claims.Name,
	})
	if err != nil {
		jsonError(w, "token_error", "Failed to generate new access token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":      newToken,
		"expires_in": "86400",
	})
}

// ─── Analytics Endpoints ────────────────────────────────────────────────────

// HandleAnalyticsSummary returns aggregated stats.
// GET /admin/analytics/summary?days=30
func (h *AdminHandler) HandleAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	days := queryInt(r, "days", 30)

	summary, err := h.analyticsRepo.Summary(r.Context(), days)
	if err != nil {
		slog.Error("Analytics summary failed", "error", err)
		jsonError(w, "db_error", "Failed to get analytics summary", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// HandleDailyStats returns per-day analytics for charting.
// GET /admin/analytics/daily?days=30
func (h *AdminHandler) HandleDailyStats(w http.ResponseWriter, r *http.Request) {
	days := queryInt(r, "days", 30)

	stats, err := h.analyticsRepo.DailyStats(r.Context(), days)
	if err != nil {
		slog.Error("Daily stats failed", "error", err)
		jsonError(w, "db_error", "Failed to get daily stats", http.StatusInternalServerError)
		return
	}

	if stats == nil {
		stats = []domain.DailyStat{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HandleTopQueries returns the most frequent queries.
// GET /admin/analytics/top-queries?days=30&limit=20
func (h *AdminHandler) HandleTopQueries(w http.ResponseWriter, r *http.Request) {
	days := queryInt(r, "days", 30)
	limit := queryInt(r, "limit", 20)

	queries, err := h.analyticsRepo.TopQueries(r.Context(), days, limit)
	if err != nil {
		slog.Error("Top queries failed", "error", err)
		jsonError(w, "db_error", "Failed to get top queries", http.StatusInternalServerError)
		return
	}

	if queries == nil {
		queries = []domain.TopQuery{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(queries)
}

// HandleFeedbackStats returns feedback ratio and counts.
// GET /admin/analytics/feedback?days=30
func (h *AdminHandler) HandleFeedbackStats(w http.ResponseWriter, r *http.Request) {
	days := queryInt(r, "days", 30)

	stats, err := h.analyticsRepo.FeedbackStats(r.Context(), days)
	if err != nil {
		slog.Error("Feedback stats failed", "error", err)
		jsonError(w, "db_error", "Failed to get feedback stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HandleExportCSV exports analytics data as CSV.
// GET /admin/analytics/export/csv?days=30
// TZ §3.3: «Експорт даних у CSV»
func (h *AdminHandler) HandleExportCSV(w http.ResponseWriter, r *http.Request) {
	days := queryInt(r, "days", 30)
	limit := queryInt(r, "limit", 1000)

	rows, err := h.analyticsRepo.RecentQueries(r.Context(), days, limit)
	if err != nil {
		slog.Error("CSV export failed", "error", err)
		jsonError(w, "db_error", "Failed to export analytics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="analytics_export.csv"`)
	w.WriteHeader(http.StatusOK)

	// BOM for Excel UTF-8 compatibility
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	// CSV header
	fmt.Fprintf(w, "query_hash,language,response_ms,sources_cnt,feedback,is_blocked,created_at\n")

	for _, q := range rows {
		fmt.Fprintf(w, "%s,%s,%d,%d,%d,%d,%s\n",
			q.QueryHash, q.Language, q.ResponseMs, q.SourcesCnt, q.Feedback, q.IsBlocked, q.CreatedAt)
	}
}

// HandleRecentQueries returns individual query rows for admin inspection.
// GET /admin/queries?days=30&limit=50
func (h *AdminHandler) HandleRecentQueries(w http.ResponseWriter, r *http.Request) {
	days := queryInt(r, "days", 30)
	limit := queryInt(r, "limit", 50)

	queries, err := h.analyticsRepo.RecentQueries(r.Context(), days, limit)
	if err != nil {
		slog.Error("Recent queries failed", "error", err)
		jsonError(w, "db_error", "Failed to get recent queries", http.StatusInternalServerError)
		return
	}
	if queries == nil {
		queries = []domain.QueryRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(queries)
}

// ─── Document Endpoints ─────────────────────────────────────────────────────

// HandleListDocuments returns all knowledge base documents.
// GET /admin/documents
func (h *AdminHandler) HandleListDocuments(w http.ResponseWriter, r *http.Request) {
	docs, err := h.documentRepo.List(r.Context())
	if err != nil {
		slog.Error("List documents failed", "error", err)
		jsonError(w, "db_error", "Failed to list documents", http.StatusInternalServerError)
		return
	}

	if docs == nil {
		docs = []domain.DocumentRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(docs)
}

// HandleDownloadDocument serves the raw document file.
// GET /admin/documents/{id}/download
func (h *AdminHandler) HandleDownloadDocument(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")
	doc, err := h.documentRepo.GetByID(r.Context(), docID)
	if err != nil {
		jsonError(w, "not_found", "Document not found", http.StatusNotFound)
		return
	}

	ext := filepath.Ext(doc.Filename)
	filePath := filepath.Join(".", "data", "documents", docID+strings.ToLower(ext))

	// Ensure file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		jsonError(w, "file_missing", "The raw file is not available on the server", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", `inline; filename="`+doc.Filename+`"`)
	http.ServeFile(w, r, filePath)
}

// HandleRenameDocument updates a document's filename.
// PATCH /admin/documents/{id}/rename
func (h *AdminHandler) HandleRenameDocument(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")

	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" {
		jsonError(w, "bad_request", "Valid filename is required", http.StatusBadRequest)
		return
	}

	// Rename in SQLite
	if err := h.documentRepo.Rename(r.Context(), docID, req.Filename); err != nil {
		slog.Error("Rename document failed", "error", err)
		jsonError(w, "db_error", "Failed to rename document", http.StatusInternalServerError)
		return
	}

	// Rename payload in Qdrant
	if h.vectorStore != nil {
		if err := h.vectorStore.RenameDocumentPayload(r.Context(), docID, req.Filename); err != nil {
			slog.Error("Qdrant rename payload failed", "error", err)
		}
	}

	if h.auditRepo != nil {
		adminEmail := "system"
		if email, ok := r.Context().Value("admin_email").(string); ok {
			adminEmail = email
		}
		_ = h.auditRepo.Record(r.Context(), domain.AuditEntry{
			AdminEmail: adminEmail,
			Action:     "rename_document",
			Target:     fmt.Sprintf("ID: %s, New Name: %s", docID, req.Filename),
			IP:         realIP(r),
		})
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ─── Audit Log Endpoint ─────────────────────────────────────────────────────

// HandleAuditLog returns paginated admin audit entries.
// GET /admin/audit?offset=0&limit=50
func (h *AdminHandler) HandleAuditLog(w http.ResponseWriter, r *http.Request) {
	offset := queryInt(r, "offset", 0)
	limit := queryInt(r, "limit", 50)

	entries, total, err := h.auditRepo.List(r.Context(), offset, limit)
	if err != nil {
		slog.Error("Audit log failed", "error", err)
		jsonError(w, "db_error", "Failed to get audit log", http.StatusInternalServerError)
		return
	}

	if entries == nil {
		entries = []domain.AuditEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"entries": entries,
		"total":   total,
		"offset":  offset,
		"limit":   limit,
	})
}

// ─── Prompt Variants Endpoints (A/B Testing) ────────────────────────────────

// HandleListPrompts returns all prompt variants.
// GET /admin/prompts
func (h *AdminHandler) HandleListPrompts(w http.ResponseWriter, r *http.Request) {
	variants, err := h.promptRepo.List(r.Context())
	if err != nil {
		jsonError(w, "db_error", "Failed to list prompt variants", http.StatusInternalServerError)
		return
	}
	if variants == nil {
		variants = []domain.PromptVariant{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(variants)
}

// HandleCreatePrompt creates a new prompt variant.
// POST /admin/prompts
func (h *AdminHandler) HandleCreatePrompt(w http.ResponseWriter, r *http.Request) {
	var variant domain.PromptVariant
	if err := json.NewDecoder(r.Body).Decode(&variant); err != nil {
		jsonError(w, "invalid_request", "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if variant.Name == "" || variant.PromptText == "" {
		jsonError(w, "validation_error", "name and prompt_text are required", http.StatusBadRequest)
		return
	}

	if variant.Language == "" {
		variant.Language = domain.LangUk
	}

	if err := h.promptRepo.Create(r.Context(), &variant); err != nil {
		jsonError(w, "db_error", "Failed to create prompt variant", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(variant)
}

// HandleTogglePromptActive toggles is_active for a prompt.
// PATCH /admin/prompts/{id}/active
func (h *AdminHandler) HandleTogglePromptActive(w http.ResponseWriter, r *http.Request) {
	promptID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid_id", "Invalid prompt ID", http.StatusBadRequest)
		return
	}

	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid_request", "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := h.promptRepo.SetActive(r.Context(), promptID, req.IsActive); err != nil {
		jsonError(w, "db_error", "Failed to update prompt status", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleUpdatePrompt updates the prompt_text.
// PATCH /admin/prompts/{id}
func (h *AdminHandler) HandleUpdatePrompt(w http.ResponseWriter, r *http.Request) {
	promptID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid_id", "Invalid prompt ID", http.StatusBadRequest)
		return
	}

	var req struct {
		PromptText string `json:"prompt_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PromptText == "" {
		jsonError(w, "invalid_request", "Invalid JSON body or empty text", http.StatusBadRequest)
		return
	}

	if err := h.promptRepo.Update(r.Context(), promptID, req.PromptText); err != nil {
		jsonError(w, "db_error", "Failed to update prompt", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleDeletePrompt deletes a prompt completely.
// DELETE /admin/prompts/{id}
func (h *AdminHandler) HandleDeletePrompt(w http.ResponseWriter, r *http.Request) {
	promptID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid_id", "Invalid prompt ID", http.StatusBadRequest)
		return
	}

	if err := h.promptRepo.Delete(r.Context(), promptID); err != nil {
		jsonError(w, "db_error", "Failed to delete prompt", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ─── Suggestions Endpoints ──────────────────────────────────────────────────

// HandleListSuggestions returns suggested questions for a language.
// GET /admin/suggestions?lang=uk&limit=5
// Also used publicly via /api/v1/suggestions
func (h *AdminHandler) HandleListSuggestions(w http.ResponseWriter, r *http.Request) {
	lang := domain.Language(r.URL.Query().Get("lang"))
	if lang != domain.LangUk && lang != domain.LangEn {
		lang = domain.LangUk
	}
	limit := queryInt(r, "limit", 5)

	questions, err := h.suggestRepo.List(r.Context(), lang, limit)
	if err != nil {
		jsonError(w, "db_error", "Failed to list suggestions", http.StatusInternalServerError)
		return
	}
	if questions == nil {
		questions = []domain.SuggestedQuestion{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(questions)
}

// HandleCreateSuggestion creates a new suggested question.
// POST /admin/suggestions
func (h *AdminHandler) HandleCreateSuggestion(w http.ResponseWriter, r *http.Request) {
	var q domain.SuggestedQuestion
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		jsonError(w, "invalid_request", "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if q.Question == "" {
		jsonError(w, "validation_error", "question is required", http.StatusBadRequest)
		return
	}

	if q.Language == "" {
		q.Language = domain.LangUk
	}

	if err := h.suggestRepo.Upsert(r.Context(), &q); err != nil {
		jsonError(w, "db_error", "Failed to create suggestion", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(q)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}
