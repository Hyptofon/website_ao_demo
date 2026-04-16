package parser

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"

	"university-chatbot/backend/internal/domain"
)

// PDFExtractor uses the Gemini API to extract text from PDF files.
// This approach handles scanned PDFs, complex layouts, tables, and
// multi-column documents far better than any free Go library.
type PDFExtractor struct {
	client *genai.Client
}

// NewPDFExtractor creates a new PDF text extractor powered by Gemini.
func NewPDFExtractor(client *genai.Client) *PDFExtractor {
	return &PDFExtractor{client: client}
}

const pdfMaxRetries = 5

// ExtractText reads a PDF file and uses Gemini to extract all text content.
// Wrapped with retry logic for transient API errors (503, 429).
// Fatal errors (400 INVALID_ARGUMENT) are detected immediately without retry.
func (p *PDFExtractor) ExtractText(ctx context.Context, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read pdf %q: %w", path, err)
	}

	pdfPart := genai.NewPartFromBytes(data, "application/pdf")
	promptPart := genai.NewPartFromText(domain.PDFExtractionPrompt)

	var lastErr error
	for attempt := 1; attempt <= pdfMaxRetries; attempt++ {
		resp, err := p.client.Models.GenerateContent(
			ctx,
			"gemini-2.5-flash",
			[]*genai.Content{
				{
					Role:  "user",
					Parts: []*genai.Part{pdfPart, promptPart},
				},
			},
			&genai.GenerateContentConfig{
				MaxOutputTokens: 65536,
			},
		)
		if err != nil {
			lastErr = err

			// Fast-fail on fatal (non-retryable) errors — no point burning time
			// retrying an INVALID_ARGUMENT from Gemini.
			if isFatalPDFError(err) {
				return "", friendlyPDFError(err, path)
			}

			if !isTransientError(err) || attempt == pdfMaxRetries {
				break
			}
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			slog.Warn("PDF extraction retry",
				"attempt", attempt,
				"max", pdfMaxRetries,
				"backoff", backoff,
				"error", err,
				"path", path,
			)
			select {
			case <-ctx.Done():
				return "", fmt.Errorf("pdf extract cancelled: %w", ctx.Err())
			case <-time.After(backoff):
			}
			continue
		}

		// Parse response
		var result strings.Builder
		for _, cand := range resp.Candidates {
			if cand.Content == nil {
				continue
			}
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					result.WriteString(part.Text)
				}
			}
		}

		text := strings.TrimSpace(result.String())
		if text == "" {
			return "", fmt.Errorf("gemini returned no text for %q", path)
		}

		if attempt > 1 {
			slog.Info("PDF extraction succeeded on retry", "attempt", attempt, "path", path)
		}
		return text, nil
	}

	return "", friendlyTransientError(pdfMaxRetries, lastErr)
}

// friendlyTransientError formats a retry-exhausted error into an admin-readable message.
func friendlyTransientError(attempts int, err error) error {
	s := ""
	if err != nil {
		s = err.Error()
	}
	switch {
	case strings.Contains(s, "503") || strings.Contains(s, "UNAVAILABLE") || strings.Contains(s, "high demand"):
		return fmt.Errorf(
			"Gemini API тимчасово перевантажений (503). "+
				"Файл у форматі PDF правильний, але сервери AI зараз недоступні. "+
				"Спробуйте завантажити файл ще раз через 2-3 хвилини",
		)
	case strings.Contains(s, "429") || strings.Contains(s, "RESOURCE_EXHAUSTED"):
		return fmt.Errorf(
			"Перевищено ліміт запитів до Gemini API (429). "+
				"Зачекайте кілька хвилин та спробуйте знову",
		)
	default:
		return fmt.Errorf("PDF не вдалось опрацювати після %d спроб. Деталі: %v", attempts, err)
	}
}

// INVALID_ARGUMENT (400) means the file itself is malformed/unsupported.
func isFatalPDFError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "400") ||
		strings.Contains(s, "INVALID_ARGUMENT") ||
		strings.Contains(s, "PERMISSION_DENIED") ||
		strings.Contains(s, "403")
}

// friendlyPDFError maps Gemini API errors to admin-readable messages.
func friendlyPDFError(err error, path string) error {
	s := err.Error()
	switch {
	case strings.Contains(s, "no pages") || strings.Contains(s, "no_pages"):
		return fmt.Errorf(
			"PDF файл %q не містить жодної сторінки. "+
				"Можливі причини: файл пошкоджений, зашифрований паролем, або це не справжній PDF. "+
				"Спробуйте відкрити файл локально та переконатись що він читабельний",
			fileBasename(path),
		)
	case strings.Contains(s, "PERMISSION_DENIED") || strings.Contains(s, "403"):
		return fmt.Errorf(
			"Gemini API відмовив у доступі до файлу %q (403). "+
				"Перевірте GEMINI_API_KEY та дозволи",
			fileBasename(path),
		)
	default:
		return fmt.Errorf(
			"неможливо обробити PDF %q: файл може бути зашифрований, пошкоджений або у непідтримуваному форматі. "+
				"Деталі: %v",
			fileBasename(path), err,
		)
	}
}

func fileBasename(path string) string {
	parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

// isTransientError checks if an error is worth retrying (503, 429, etc.).
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	transientMarkers := []string{
		"503", "429", "UNAVAILABLE", "RESOURCE_EXHAUSTED",
		"DEADLINE_EXCEEDED", "connection reset", "connection refused", "EOF",
	}
	for _, marker := range transientMarkers {
		if strings.Contains(errStr, marker) {
			return true
		}
	}
	return false
}
