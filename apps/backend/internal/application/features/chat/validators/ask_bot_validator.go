package validators

import (
	"strings"
	"unicode/utf8"

	"university-chatbot/backend/internal/domain"
)

const maxQueryLength = 500

// ValidationError represents a list of field-level validation failures.
type ValidationError struct {
	Errors []FieldError `json:"errors"`
}

func (e *ValidationError) Error() string {
	msgs := make([]string, 0, len(e.Errors))
	for _, fe := range e.Errors {
		msgs = append(msgs, fe.Field+": "+fe.Message)
	}
	return strings.Join(msgs, "; ")
}

// FieldError represents a single validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// AskBotValidator validates an incoming chat request before it reaches the handler.
type AskBotValidator struct{}

// Validate returns a *ValidationError if the request is invalid, otherwise nil.
func (v *AskBotValidator) Validate(req *domain.ChatRequest) error {
	var errs []FieldError

	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		errs = append(errs, FieldError{Field: "message", Message: "must not be empty"})
	} else if utf8.RuneCountInString(msg) > maxQueryLength {
		errs = append(errs, FieldError{
			Field:   "message",
			Message: "must not exceed 500 characters",
		})
	}

	if req.Language != domain.LangUk && req.Language != domain.LangEn {
		// Default to Ukrainian instead of failing
		req.Language = domain.LangUk
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}
