package chunker

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode"

	"university-chatbot/backend/internal/domain"
)

const (
	// Target chunk size in approximate tokens (1 token ≈ 4 chars for Ukrainian).
	TargetChunkChars = 1400 // ~350 tokens
	OverlapChars     = 200  // ~50 tokens overlap
	MinChunkChars    = 100  // discard very small chunks
)

// Chunker splits plain text into overlapping chunks suitable for embedding.
type Chunker struct{}

// NewChunker returns a Chunker instance.
func NewChunker() *Chunker { return &Chunker{} }

// Chunk splits text into overlapping chunks and attaches metadata.
func (c *Chunker) Chunk(documentID, documentName, docType, language, text string) []domain.Chunk {
	// Normalize whitespace
	text = strings.Join(strings.FieldsFunc(text, func(r rune) bool {
		return r == '\r'
	}), "")

	// Split into paragraphs first, then merge to target size
	paragraphs := splitParagraphs(text)
	chunks := buildChunks(paragraphs, TargetChunkChars, OverlapChars)

	result := make([]domain.Chunk, 0, len(chunks))
	for i, chunkText := range chunks {
		chunkText = strings.TrimSpace(chunkText)
		if len(chunkText) < MinChunkChars {
			continue
		}

		// Stable deterministic ID
		hash := sha256.Sum256([]byte(documentID + chunkText))
		id := fmt.Sprintf("%x", hash[:16])

		result = append(result, domain.Chunk{
			ID:           id,
			DocumentID:   documentID,
			DocumentName: documentName,
			Text:         chunkText,
			PageNumber:   i + 1, // approximate "page" = chunk index
			Metadata: map[string]string{
				"doc_type": docType,
				"language": language,
			},
		})
	}
	return result
}

// splitParagraphs splits text on double newlines (paragraph boundaries).
func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	var out []string
	for _, p := range raw {
		p = strings.TrimFunc(p, unicode.IsSpace)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// buildChunks merges paragraphs into target-size chunks with overlap.
func buildChunks(paragraphs []string, targetSize, overlapSize int) []string {
	var chunks []string
	var current strings.Builder

	flush := func() {
		if current.Len() > 0 {
			chunks = append(chunks, current.String())
		}
	}

	for _, para := range paragraphs {
		// If adding this paragraph would exceed target, flush and start overlap.
		if current.Len()+len(para)+2 > targetSize && current.Len() > 0 {
			flush()

			// Carry overlap: take last `overlapSize` chars of current chunk.
			overlap := current.String()
			if len(overlap) > overlapSize {
				overlap = overlap[len(overlap)-overlapSize:]
				// Trim to word boundary.
				if idx := strings.Index(overlap, " "); idx >= 0 {
					overlap = overlap[idx+1:]
				}
			}
			current.Reset()
			current.WriteString(overlap)
			if current.Len() > 0 {
				current.WriteString("\n\n")
			}
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(para)
	}
	flush()
	return chunks
}
