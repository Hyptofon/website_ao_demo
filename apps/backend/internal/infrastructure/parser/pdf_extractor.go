package parser

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/genai"
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

// ExtractText reads a PDF file and uses Gemini to extract all text content.
func (p *PDFExtractor) ExtractText(ctx context.Context, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read pdf %q: %w", path, err)
	}

	pdfPart := genai.NewPartFromBytes(data, "application/pdf")
	promptPart := genai.NewPartFromText(`Витягни ВЕСЬ текстовий вміст з цього PDF документа.
Вимоги:
- Поверни ТІЛЬКИ чистий текст без форматування markdown
- Збережи структуру: заголовки, абзаци, списки, таблиці
- Таблиці представ як текст з розділювачами " | " між колонками
- НЕ додавай коментарі, пояснення чи анотації
- НЕ пропускай жодного тексту
- Збережи мову оригіналу (українська/англійська)`)

	resp, err := p.client.Models.GenerateContent(
		ctx,
		"gemini-3-flash-preview",
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
		return "", fmt.Errorf("gemini extract pdf: %w", err)
	}

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

	return text, nil
}
