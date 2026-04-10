package validators

import (
	"testing"

	"university-chatbot/backend/internal/domain"
)

func TestValidate_ValidRequest(t *testing.T) {
	v := NewAskBotValidator()

	req := &domain.ChatRequest{
		SessionID: "sess-123",
		Message:   "Які документи потрібні для вступу?",
		Language:  domain.LangUk,
	}
	if err := v.Validate(req); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidate_EmptyMessage(t *testing.T) {
	v := NewAskBotValidator()

	req := &domain.ChatRequest{
		SessionID: "sess-123",
		Message:   "",
		Language:  domain.LangUk,
	}
	if err := v.Validate(req); err == nil {
		t.Error("expected validation error for empty message")
	}
}

func TestValidate_TooLongMessage(t *testing.T) {
	v := NewAskBotValidator()

	// 501 characters should exceed the max=500 limit
	longMsg := ""
	for i := 0; i < 501; i++ {
		longMsg += "a"
	}

	req := &domain.ChatRequest{
		SessionID: "sess-123",
		Message:   longMsg,
		Language:  domain.LangUk,
	}
	if err := v.Validate(req); err == nil {
		t.Error("expected validation error for message > 500 chars")
	}
}

func TestValidate_EmptySessionID(t *testing.T) {
	v := NewAskBotValidator()

	req := &domain.ChatRequest{
		SessionID: "",
		Message:   "test",
		Language:  domain.LangUk,
	}
	if err := v.Validate(req); err == nil {
		t.Error("expected validation error for empty session_id")
	}
}

func TestValidate_UnsupportedLanguageFallback(t *testing.T) {
	v := NewAskBotValidator()

	req := &domain.ChatRequest{
		SessionID: "sess-123",
		Message:   "test message",
		Language:  domain.Language("fr"),
	}
	err := v.Validate(req)
	if err != nil {
		t.Errorf("expected no error after language fallback, got %v", err)
	}
	if req.Language != domain.LangUk {
		t.Errorf("expected language fallback to 'uk', got %s", req.Language)
	}
}

func TestValidate_EnglishLanguage(t *testing.T) {
	v := NewAskBotValidator()

	req := &domain.ChatRequest{
		SessionID: "sess-456",
		Message:   "What courses are available?",
		Language:  domain.LangEn,
	}
	if err := v.Validate(req); err != nil {
		t.Errorf("expected no error for English request, got %v", err)
	}
}

func TestValidationError_ErrorString(t *testing.T) {
	ve := &ValidationError{
		Errors: []FieldError{
			{Field: "message", Message: "required"},
			{Field: "session_id", Message: "required"},
		},
	}
	errStr := ve.Error()
	if errStr == "" {
		t.Error("expected non-empty error string")
	}
}
