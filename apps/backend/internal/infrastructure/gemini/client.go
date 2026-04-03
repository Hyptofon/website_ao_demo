package gemini

import (
	"context"
	"fmt"
	"io"
	"strings"

	"google.golang.org/genai"
	"university-chatbot/backend/internal/domain"
)

const (
	modelGenerate = "gemini-3-flash-preview"
	modelEmbed    = "gemini-embedding-001"
	maxConcurrent = 50
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
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	c.semaphore <- struct{}{}
	defer func() { <-c.semaphore }()

	contents := []*genai.Content{genai.NewContentFromText(text, genai.RoleUser)}
	res, err := c.client.Models.EmbedContent(ctx, modelEmbed, contents, nil)
	if err != nil {
		return nil, fmt.Errorf("gemini: embed: %w", err)
	}
	if len(res.Embeddings) == 0 {
		return nil, fmt.Errorf("gemini: embed: no embeddings returned")
	}
	return res.Embeddings[0].Values, nil
}

// StreamAnswer streams a Gemini response token-by-token to w using SSE format.
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

	// No longer using thinkingLevelVal

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, genai.RoleUser),
		MaxOutputTokens:   1536,
		ThinkingConfig: &genai.ThinkingConfig{
			ThinkingLevel: genai.ThinkingLevel("low"),
		},
	}

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
					token := strings.ReplaceAll(part.Text, "\n", "\\n")
					fmt.Fprintf(w, "data: %s\n\n", token)
					if f, ok := w.(interface{ Flush() }); ok {
						f.Flush()
					}
				}
			}
		}
	}

	// Signal end of stream
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if f, ok := w.(interface{ Flush() }); ok {
		f.Flush()
	}
	return nil
}
