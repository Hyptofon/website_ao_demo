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

const pdfMaxRetries = 3

// ExtractText reads a PDF file and uses Gemini to extract all text content.
// Wrapped with retry logic for transient API errors (503, 429).
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

	return "", fmt.Errorf("gemini extract pdf after %d retries: %w", pdfMaxRetries, lastErr)
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
