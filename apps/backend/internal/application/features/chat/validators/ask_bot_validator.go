package validators

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/security"
)

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

// AskBotValidator validates an incoming chat request.
type AskBotValidator struct {
	validate *validator.Validate
}

func NewAskBotValidator() *AskBotValidator {
	return &AskBotValidator{validate: validator.New()}
}

// Validate uses go-playground/validator to enforce DTO tags.
func (v *AskBotValidator) Validate(req *domain.ChatRequest) error {
	// Fallback logic for unsupported languages before validation
	if req.Language != domain.LangUk && req.Language != domain.LangEn {
		req.Language = domain.LangUk
	}

	// TZ §3.5 / §6.1: XSS prevention — reject malicious HTML/JS payloads.
	if security.DetectXSSPayload(req.Message) {
		return fmt.Errorf("message contains potentially malicious content")
	}

	// Sanitize HTML entities in the message to prevent stored XSS.
	req.Message = security.SanitizeInput(req.Message)

	err := v.validate.Struct(req)
	if err != nil {
		var errs []FieldError
		for _, err := range err.(validator.ValidationErrors) {
			errs = append(errs, FieldError{
				Field:   strings.ToLower(err.Field()),
				Message: err.Tag(),
			})
		}
		return &ValidationError{Errors: errs}
	}
	return nil
}
