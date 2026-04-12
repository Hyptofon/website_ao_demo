package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"

	"university-chatbot/backend/internal/application/features/chat/commands"
	"university-chatbot/backend/internal/application/features/chat/queries"
	"university-chatbot/backend/internal/application/features/chat/validators"
	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/security"
)

// AskBotUseCase defines the interface for RAG chat processing.
type AskBotUseCase interface {
	Handle(ctx context.Context, q queries.AskBotQuery, w io.Writer) (*queries.AskBotResult, error)
}

// ChatHandler handles chat-related HTTP endpoints.
type ChatHandler struct {
	askBot    AskBotUseCase
	feedback  *commands.SubmitFeedbackHandler
	rateBan   func(ip string) // Functional injection for ban capability
	offTopic  *security.OffTopicFilter
	validator *validators.AskBotValidator
}

// NewChatHandler constructs the handler.
func NewChatHandler(
	askBot AskBotUseCase,
	feedback *commands.SubmitFeedbackHandler,
	banFunc func(ip string),
	otf *security.OffTopicFilter,
) *ChatHandler {
	return &ChatHandler{
		askBot:    askBot,
		feedback:  feedback,
		rateBan:   banFunc,
		offTopic:  otf,
		validator: validators.NewAskBotValidator(),
	}
}

// ─── POST /api/v1/chat/stream ─────────────────────────────────────────────────

func (h *ChatHandler) StreamChat(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetReqID(r.Context())

	// 1. Setup SSE headers and Flusher
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// 2. Parse and Validate Request
	var req domain.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sseError(w, flusher, "invalid_request", "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := h.validator.Validate(&req); err != nil {
		sseError(w, flusher, "validation_error", err.Error(), http.StatusBadRequest)
		return
	}

	// 3. Off-topic filter with side-effect (Ban/Penalty)
	if h.offTopic.IsOffTopic(req.Message) {
		h.rateBan(realIP(r)) // Apply penalty ban 

		offTopicMsg := domain.OffTopicResponseUA
		if req.Language == domain.LangEn {
			offTopicMsg = domain.OffTopicResponseEN
		}

		writeSSEToken(w, flusher, offTopicMsg)
		writeSources(w, flusher, nil)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	// 4. Execute RAG pipeline
	ctx := r.Context()

	result, err := h.askBot.Handle(ctx, queries.AskBotQuery{Request: &req}, &sseWriter{w: w, flusher: flusher})
	if err != nil {
		slog.Error("RAG pipeline error",
			"request_id", reqID,
			"session_id", req.SessionID,
			"error", err,
		)
		h.handleRAGError(err, &req, w, flusher)
		return
	}

	// 5. Send query hash so the frontend can submit feedback with the correct key
	writeSSEMeta(w, flusher, result.QueryHash)

	// 6. Send final sources
	writeSources(w, flusher, result.Sources)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (h *ChatHandler) handleRAGError(err error, req *domain.ChatRequest, w http.ResponseWriter, flusher http.Flusher) {
	msg := domain.RAGErrorResponseEN
	if req.Language == domain.LangUk {
		msg = domain.RAGErrorResponseUA
	}

	if strings.Contains(err.Error(), "503") || strings.Contains(err.Error(), "UNAVAILABLE") || strings.Contains(err.Error(), "429") {
		msg = domain.OverloadResponseEN
		if req.Language == domain.LangUk {
			msg = domain.OverloadResponseUA
		}
	}

	sseError(w, flusher, "internal_error", msg, http.StatusInternalServerError)
}

// ─── POST /api/v1/feedback ────────────────────────────────────────────────────

func (h *ChatHandler) SubmitFeedback(w http.ResponseWriter, r *http.Request) {
	var cmd commands.SubmitFeedbackCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		jsonError(w, "invalid_request", "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := h.feedback.Handle(r.Context(), cmd); err != nil {
		jsonError(w, "validation_error", err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

type sseWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (sw *sseWriter) Write(p []byte) (n int, err error) {
	n, err = sw.w.Write(p)
	sw.flusher.Flush()
	return
}

func writeSSEToken(w http.ResponseWriter, f http.Flusher, token string) {
	token = strings.ReplaceAll(token, "\n", "\\n")
	fmt.Fprintf(w, "data: %s\n\n", token)
	f.Flush()
}

func writeSources(w http.ResponseWriter, f http.Flusher, sources []domain.Source) {
	if sources == nil {
		sources = []domain.Source{}
	}
	b, _ := json.Marshal(sources)
	fmt.Fprintf(w, "event: sources\ndata: %s\n\n", b)
	f.Flush()
}

func writeSSEMeta(w http.ResponseWriter, f http.Flusher, queryHash string) {
	b, _ := json.Marshal(map[string]string{"query_hash": queryHash})
	fmt.Fprintf(w, "event: meta\ndata: %s\n\n", b)
	f.Flush()
}

func sseError(w http.ResponseWriter, f http.Flusher, code, msg string, status int) {
	w.WriteHeader(status)
	b, _ := json.Marshal(map[string]string{"error": code, "message": msg})
	fmt.Fprintf(w, "event: error\ndata: %s\n\n", b)
	f.Flush()
}

func jsonError(w http.ResponseWriter, code, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}

func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.SplitN(ip, ",", 2)[0]
	}
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		return addr[:idx]
	}
	return addr
}
