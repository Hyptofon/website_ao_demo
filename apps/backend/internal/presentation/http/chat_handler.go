package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"university-chatbot/backend/internal/application/features/chat/commands"
	"university-chatbot/backend/internal/application/features/chat/queries"
	"university-chatbot/backend/internal/application/features/chat/validators"
	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/security"
)

// ChatHandler handles chat-related HTTP endpoints.
type ChatHandler struct {
	askBot       *queries.AskBotHandler
	feedback     *commands.SubmitFeedbackHandler
	rateLimiter  *security.RateLimiter
	offTopic     *security.OffTopicFilter
	validator    *validators.AskBotValidator
}

// NewChatHandler constructs the handler.
func NewChatHandler(
	askBot *queries.AskBotHandler,
	feedback *commands.SubmitFeedbackHandler,
	rl *security.RateLimiter,
	otf *security.OffTopicFilter,
) *ChatHandler {
	return &ChatHandler{
		askBot:      askBot,
		feedback:    feedback,
		rateLimiter: rl,
		offTopic:    otf,
		validator:   &validators.AskBotValidator{},
	}
}

// ─── POST /api/v1/chat/stream ─────────────────────────────────────────────────
// This endpoint handles both initiating the chat and streaming the response.
// It uses SSE (Server-Sent Events) protocol.

func (h *ChatHandler) StreamChat(w http.ResponseWriter, r *http.Request) {
	// --- SSE headers ---
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// --- Parse request body ---
	var req domain.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sseError(w, flusher, "invalid_request", "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// --- Validate input ---
	if err := h.validator.Validate(&req); err != nil {
		sseError(w, flusher, "validation_error", err.Error(), http.StatusBadRequest)
		return
	}

	// --- Rate limiting ---
	ip := realIP(r)
	allowed, retryAfter := h.rateLimiter.Allow(ip, 1)
	if !allowed {
		secs := int(retryAfter.Seconds())
		msg := fmt.Sprintf("Ви надіслали забагато запитань. Зачекайте %d сек.", secs)
		if req.Language == domain.LangEn {
			msg = fmt.Sprintf("You have sent too many requests. Please wait %d seconds.", secs)
		}
		sseError(w, flusher, "rate_limit_exceeded", msg, http.StatusTooManyRequests)
		return
	}

	// --- Off-topic filter (Stage 1: keyword-based, zero latency) ---
	if h.offTopic.IsOffTopic(req.Message) {
		// Apply penalty: this counts as 3 normal requests
		h.rateLimiter.Allow(ip, 2) // +2 more on top of the 1 already consumed

		offTopicMsg := security.OffTopicResponseUA
		if req.Language == domain.LangEn {
			offTopicMsg = security.OffTopicResponseEN
		}

		// Stream it as a normal response so the UI renders it smoothly
		writeSSEToken(w, flusher, offTopicMsg)
		writeSources(w, flusher, nil)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// --- Execute RAG pipeline with streaming ---
	result, err := h.askBot.Handle(ctx, queries.AskBotQuery{Request: &req}, &sseWriter{w: w, flusher: flusher})
	if err != nil {
		log.Printf("[ERROR] RAG pipeline: %v", err)
		sseError(w, flusher, "internal_error", "Failed to generate response", http.StatusInternalServerError)
		return
	}

	// --- Send sources after the answer ---
	writeSources(w, flusher, result.Sources)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
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

// writeSSEToken sends a single text token as an SSE data event.
func writeSSEToken(w http.ResponseWriter, f http.Flusher, token string) {
	token = strings.ReplaceAll(token, "\n", "\\n")
	fmt.Fprintf(w, "data: %s\n\n", token)
	f.Flush()
}

// writeSources sends sources as a JSON SSE event.
func writeSources(w http.ResponseWriter, f http.Flusher, sources []domain.Source) {
	if sources == nil {
		sources = []domain.Source{}
	}
	b, _ := json.Marshal(sources)
	fmt.Fprintf(w, "event: sources\ndata: %s\n\n", b)
	f.Flush()
}

// sseError sends an error as an SSE event and stops the stream.
func sseError(w http.ResponseWriter, f http.Flusher, code, msg string, status int) {
	w.WriteHeader(status)
	b, _ := json.Marshal(map[string]string{"error": code, "message": msg})
	fmt.Fprintf(w, "event: error\ndata: %s\n\n", b)
	f.Flush()
}

// jsonError writes a standard JSON error response.
func jsonError(w http.ResponseWriter, code, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}

// realIP extracts the real client IP, respecting X-Real-IP/X-Forwarded-For headers.
func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.SplitN(ip, ",", 2)[0]
	}
	// Strip port from RemoteAddr
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		return addr[:idx]
	}
	return addr
}
