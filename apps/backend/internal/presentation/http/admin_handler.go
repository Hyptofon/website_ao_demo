package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

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
	allowedEmails []string
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
	allowedEmails []string,
) *AdminHandler {
	return &AdminHandler{
		oauthSvc:      oauthSvc,
		jwtSvc:        jwtSvc,
		analyticsRepo: analyticsRepo,
		auditRepo:     auditRepo,
		documentRepo:  documentRepo,
		promptRepo:    promptRepo,
		suggestRepo:   suggestRepo,
		allowedEmails: allowedEmails,
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

	// Record login in audit log
	if h.auditRepo != nil {
		go func() {
			_ = h.auditRepo.Record(r.Context(), domain.AuditEntry{
				AdminEmail: userInfo.Email,
				Action:     domain.ActionLogin,
				Target:     "oauth",
				IP:         realIP(r),
			})
		}()
	}

	slog.Info("Admin login successful", "email", userInfo.Email)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stateResponse{
		Token: token,
		Email: userInfo.Email,
		Name:  userInfo.Name,
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
