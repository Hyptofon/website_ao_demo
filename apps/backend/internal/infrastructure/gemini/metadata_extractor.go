package gemini

import (
	"context"
	"fmt"
	"log"
	"strings"

	"google.golang.org/genai"
	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/utils"
)

// ─── Structured Output: Document Metadata Extraction ─────────────────────────
// Inspired by python_service-dev/core/services/rag_metadata_extractor.py
// Uses Gemini JSON mode to auto-detect language, doc_type, and summary.

// DocumentMetadata holds automatically extracted document metadata.
type DocumentMetadata struct {
	Language string `json:"language"` // "uk" or "en"
	DocType  string `json:"doc_type"` // "rules", "syllabus", "schedule", "faq", "order", "general"
	Summary  string `json:"summary"`  // 1-2 sentence summary
}

// MetadataExtractor uses Gemini to analyze document content and extract metadata.
type MetadataExtractor struct {
	client *genai.Client
}

// NewMetadataExtractor creates a new MetadataExtractor.
func NewMetadataExtractor(client *genai.Client) *MetadataExtractor {
	return &MetadataExtractor{client: client}
}

// ExtractMetadata analyzes a text excerpt and returns structured metadata.
// It takes the first ~2000 chars of the document to minimize token usage.
func (m *MetadataExtractor) ExtractMetadata(ctx context.Context, text string) (*DocumentMetadata, error) {
	// Take only a snippet to minimize token costs
	excerpt := text
	if len(excerpt) > 2000 {
		excerpt = excerpt[:2000]
	}

	prompt := fmt.Sprintf(domain.MetadataExtractionPrompt, excerpt)

	resp, err := withRetry(ctx, "metadata-extract", defaultMaxRetries, func() (*genai.GenerateContentResponse, error) {
		return m.client.Models.GenerateContent(
			ctx,
			modelGenerate,
			[]*genai.Content{genai.NewContentFromText(prompt, genai.RoleUser)},
			&genai.GenerateContentConfig{
				MaxOutputTokens:  256,
				ResponseMIMEType: "application/json",
			},
		)
	})
	if err != nil {
		return nil, fmt.Errorf("gemini metadata extract: %w", err)
	}

	// Extract text from response
	var rawJSON strings.Builder
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				rawJSON.WriteString(part.Text)
			}
		}
	}

	if rawJSON.Len() == 0 {
		return nil, fmt.Errorf("gemini returned empty response for metadata extraction")
	}

	// Parse with sanitization
	var meta DocumentMetadata
	if err := utils.SafeJSONUnmarshal([]byte(rawJSON.String()), &meta); err != nil {
		log.Printf("[WARN] Metadata extraction JSON parse failed, using defaults: %v", err)
		return defaultMetadata(text), nil
	}

	// Validate and normalize
	meta.Language = normalizeLanguage(meta.Language)
	meta.DocType = normalizeDocType(meta.DocType)

	return &meta, nil
}

// defaultMetadata returns sensible defaults when extraction fails.
func defaultMetadata(text string) *DocumentMetadata {
	lang := "uk"
	// Simple heuristic: if more Latin chars than Cyrillic, it's likely English
	cyrillic, latin := 0, 0
	for _, r := range text {
		if r >= 'а' && r <= 'я' || r >= 'А' && r <= 'Я' || r == 'і' || r == 'ї' || r == 'є' || r == 'ґ' {
			cyrillic++
		} else if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			latin++
		}
	}
	if latin > cyrillic {
		lang = "en"
	}

	return &DocumentMetadata{
		Language: lang,
		DocType:  "general",
		Summary:  "",
	}
}

func normalizeLanguage(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "en" || lang == "english" {
		return "en"
	}
	return "uk"
}

func normalizeDocType(dt string) string {
	dt = strings.ToLower(strings.TrimSpace(dt))
	validTypes := map[string]bool{
		"rules": true, "syllabus": true, "schedule": true,
		"faq": true, "order": true, "general": true,
	}
	if validTypes[dt] {
		return dt
	}
	return "general"
}
