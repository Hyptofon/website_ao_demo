package gemini

import (
	"context"
	"fmt"
	"io"
	"strings"

	"google.golang.org/genai"
	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/utils"
)

const (
	modelGenerate = "gemini-2.5-flash"
	modelEmbed    = "gemini-embedding-001"
	maxConcurrent = 50

	// Temperature for factual RAG answers (low = more deterministic).
	// Inspired by python_service-dev temp_code=0.3, but even lower for factual Q&A.
	temperatureChat = 0.2
)

// Client wraps the google.golang.org/genai SDK with a concurrency semaphore.
type Client struct {
	client    *genai.Client
	semaphore chan struct{}
}

// NewClient initialises the Gemini client using the unified SDK.
func NewClient(ctx context.Context, apiKey string) (*Client, error) {
	c, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: init client: %w", err)
	}
	return &Client{
		client:    c,
		semaphore: make(chan struct{}, maxConcurrent),
	}, nil
}

// Close is a no-op for the new SDK but keeps the interface stable.
func (c *Client) Close() error {
	return nil
}

// RawClient returns the underlying genai.Client for use by other infrastructure components.
func (c *Client) RawClient() *genai.Client {
	return c.client
}

// Embed returns a vector for the given text using text-embedding-004.
// Wrapped with retry logic for transient API errors (503, 429).
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	c.semaphore <- struct{}{}
	defer func() { <-c.semaphore }()

	return withRetry(ctx, "gemini-embed", defaultMaxRetries, func() ([]float32, error) {
		contents := []*genai.Content{genai.NewContentFromText(text, genai.RoleUser)}
		res, err := c.client.Models.EmbedContent(ctx, modelEmbed, contents, nil)
		if err != nil {
			return nil, fmt.Errorf("gemini: embed: %w", err)
		}
		if len(res.Embeddings) == 0 {
			return nil, fmt.Errorf("gemini: embed: no embeddings returned")
		}
		return res.Embeddings[0].Values, nil
	})
}

// StreamAnswer streams a Gemini response token-by-token to w using SSE format.
// Note: streaming cannot be retried mid-stream. The retry wraps the entire stream
// attempt — if a transient error occurs before any tokens are emitted, it retries.
func (c *Client) StreamAnswer(
	ctx context.Context,
	systemPrompt, userQuery, docContext string,
	lang domain.Language,
	w io.Writer,
) error {
	c.semaphore <- struct{}{}
	defer func() { <-c.semaphore }()

	// Build the full prompt: context + user query
	var promptBuilder strings.Builder
	if docContext != "" {
		promptBuilder.WriteString("Контекст (офіційні документи кафедри):\n\n")
		promptBuilder.WriteString(docContext)
		promptBuilder.WriteString("\n\n---\n\nЗапитання користувача: ")
	}
	promptBuilder.WriteString(userQuery)

	temp := float32(temperatureChat)
	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, genai.RoleUser),
		MaxOutputTokens:   1536,
		Temperature:       &temp,
	}

	var textEmitted bool

	for resp, err := range c.client.Models.GenerateContentStream(
		ctx,
		modelGenerate,
		[]*genai.Content{genai.NewContentFromText(promptBuilder.String(), genai.RoleUser)},
		config,
	) {
		if err != nil {
			return fmt.Errorf("gemini: stream: %w", err)
		}
		if resp == nil {
			continue
		}

		for _, cand := range resp.Candidates {
			if cand.Content == nil {
				continue
			}
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					textEmitted = true
					token := strings.ReplaceAll(part.Text, "\n", "\\n")
					fmt.Fprintf(w, "data: %s\n\n", token)
					if f, ok := w.(interface{ Flush() }); ok {
						f.Flush()
					}
				}
			}
		}
	}

	if !textEmitted {
		fallbackText := domain.FallbackResponseUA
		if lang == domain.LangEn {
			fallbackText = domain.FallbackResponseEN
		}
		fallbackText = strings.ReplaceAll(fallbackText, "\n", "\\n")
		fmt.Fprintf(w, "data: %s\n\n", fallbackText)
		if f, ok := w.(interface{ Flush() }); ok {
			f.Flush()
		}
	}

	// We no longer signal end of stream here because the HTTP handler (chat_handler.go)
	// needs to append additional events (meta, sources) before signaling [DONE].
	return nil
}

// GenerateJSON sends a prompt to Gemini in JSON mode and unmarshals the response into result.
// Uses retry logic and JSON sanitization for maximum reliability.
func (c *Client) GenerateJSON(ctx context.Context, prompt string, result any) error {
	c.semaphore <- struct{}{}
	defer func() { <-c.semaphore }()

	resp, err := withRetry(ctx, "gemini-json", defaultMaxRetries, func() (*genai.GenerateContentResponse, error) {
		return c.client.Models.GenerateContent(
			ctx,
			modelGenerate,
			[]*genai.Content{genai.NewContentFromText(prompt, genai.RoleUser)},
			&genai.GenerateContentConfig{
				MaxOutputTokens:  4096,
				ResponseMIMEType: "application/json",
			},
		)
	})
	if err != nil {
		return fmt.Errorf("gemini json generate: %w", err)
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
		return fmt.Errorf("gemini returned empty JSON response")
	}

	return utils.SafeJSONUnmarshal([]byte(rawJSON.String()), result)
}
