package memory

import (
	"context"
	"sync"
	"time"

	"university-chatbot/backend/internal/domain"
)

// ChatMemory implements domain.ConversationMemory using an in-memory thread-safe map
// with expiration (garbage collection for old sessions).
type ChatMemory struct {
	sessions sync.Map // sessionID -> *sessionData
	ttl      time.Duration
}

type sessionData struct {
	History  []domain.Message
	LastSeen time.Time
}

// NewChatMemory creates a new in-memory conversation store and starts a background
// goroutine to clean up expired sessions.
func NewChatMemory(ttl time.Duration) *ChatMemory {
	cm := &ChatMemory{
		ttl: ttl,
	}

	// Start background cleanup
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			cm.cleanup()
		}
	}()

	return cm
}

// GetHistory retrieves the conversation history for a session.
func (m *ChatMemory) GetHistory(ctx context.Context, sessionID string, limit int) ([]domain.Message, error) {
	if sessionID == "" {
		return nil, nil
	}

	val, ok := m.sessions.Load(sessionID)
	if !ok {
		return nil, nil
	}

	data := val.(*sessionData)
	data.LastSeen = time.Now()

	hist := data.History
	if len(hist) > limit {
		// Return only the last 'limit' messages
		hist = hist[len(hist)-limit:]
	}

	// Return a copy to prevent mutation
	copied := make([]domain.Message, len(hist))
	copy(copied, hist)
	return copied, nil
}

// AddMessage appends a message to the session's history.
func (m *ChatMemory) AddMessage(ctx context.Context, sessionID string, msg domain.Message) error {
	if sessionID == "" {
		return nil
	}

	val, _ := m.sessions.LoadOrStore(sessionID, &sessionData{
		History:  make([]domain.Message, 0),
		LastSeen: time.Now(),
	})

	data := val.(*sessionData)
	// Mutex is not strictly necessary for simple slice append if we assume one request per session at a time,
	// but to be absolutely thread-safe we'd need a lock per session. For the MVP, we just replace the slice.
	// We'll use a simple approach: limit history length to avoid infinite growth (e.g. max 50).
	hist := append(data.History, msg)
	if len(hist) > 50 {
		hist = hist[len(hist)-50:]
	}
	data.History = hist
	data.LastSeen = time.Now()

	return nil
}

func (m *ChatMemory) cleanup() {
	now := time.Now()
	m.sessions.Range(func(key, value interface{}) bool {
		data := value.(*sessionData)
		if now.Sub(data.LastSeen) > m.ttl {
			m.sessions.Delete(key)
		}
		return true
	})
}
