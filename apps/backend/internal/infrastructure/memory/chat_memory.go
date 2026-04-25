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
	mu       sync.Mutex
	History  []domain.Message
	LastSeen time.Time
}

// NewChatMemory creates a new in-memory conversation store and starts a background
// goroutine to clean up expired sessions.
// The ctx parameter controls the lifecycle of the cleanup goroutine:
// when ctx is cancelled (e.g. on SIGTERM) the goroutine exits cleanly.
func NewChatMemory(ctx context.Context, ttl time.Duration) *ChatMemory {
	cm := &ChatMemory{
		ttl: ttl,
	}

	// Start context-aware background cleanup.
	// Previously used time.Sleep which cannot be interrupted on shutdown.
	// Now uses ticker + select so the goroutine exits when serverCtx is cancelled.
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cm.cleanup()
			}
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
	data.mu.Lock()
	defer data.mu.Unlock()

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
	
	data.mu.Lock()
	defer data.mu.Unlock()

	// Limit history length to avoid infinite growth (e.g. max 50).
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
