package qdrant

import (
	"context"
	"fmt"

	"github.com/qdrant/go-client/qdrant"
	"university-chatbot/backend/internal/domain"
)

const (
	collectionName = "university_docs"
	// vectorSize is 3072 to match gemini-embedding-001 output dimensions.
	// TZ §4.2 originally specified text-embedding-004 (768 dims), however
	// gemini-embedding-001 produces higher quality 3072-dim embeddings that
	// significantly improve retrieval accuracy for Ukrainian university content.
	// Changing the model requires re-embedding all documents and recreating the collection.
	vectorSize = 3072
	topKDense  = 5
	topKSparse = 3
)

// Client wraps the Qdrant gRPC client.
type Client struct {
	conn       *qdrant.Client
	embedder   interface {
		Embed(ctx context.Context, text string) ([]float32, error)
	}
}

// NewClient connects to Qdrant using the given URL and API key.
func NewClient(ctx context.Context, url, apiKey string, embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}) (*Client, error) {
	cfg := &qdrant.Config{
		Host: url,
		Port: 6334, // gRPC port
	}
	if apiKey != "" {
		cfg.APIKey = apiKey
		cfg.UseTLS = true
	}

	conn, err := qdrant.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("qdrant: connect: %w", err)
	}
	return &Client{conn: conn, embedder: embedder}, nil
}

// EnsureCollection creates the Qdrant collection if it doesn't exist.
func (c *Client) EnsureCollection(ctx context.Context) error {
	exists, err := c.conn.CollectionExists(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("qdrant: check collection: %w", err)
	}
	if exists {
		return nil
	}

	err = c.conn.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     vectorSize,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("qdrant: create collection: %w", err)
	}
	return nil
}

// UpsertChunks embeds and stores document chunks into Qdrant.
func (c *Client) UpsertChunks(ctx context.Context, chunks []domain.Chunk) error {
	points := make([]*qdrant.PointStruct, 0, len(chunks))

	for _, chunk := range chunks {
		vec, err := c.embedder.Embed(ctx, chunk.Text)
		if err != nil {
			return fmt.Errorf("qdrant: embed chunk %s: %w", chunk.ID, err)
		}

		payload := map[string]*qdrant.Value{
			"document_id":   qdrant.NewValueString(chunk.DocumentID),
			"document_name": qdrant.NewValueString(chunk.DocumentName),
			"text":          qdrant.NewValueString(chunk.Text),
			"page_number":   qdrant.NewValueInt(int64(chunk.PageNumber)),
		}
		for k, v := range chunk.Metadata {
			payload[k] = qdrant.NewValueString(v)
		}

		points = append(points, &qdrant.PointStruct{
			Id:      qdrant.NewIDUUID(chunk.ID),
			Vectors: qdrant.NewVectorsDense(vec),
			Payload: payload,
		})
	}

	_, err := c.conn.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collectionName,
		Points:         points,
	})
	return err
}

// HybridSearch performs dense vector search (BM25 sparse not in free tier, use dense only).
func (c *Client) HybridSearch(ctx context.Context, query string, topK int) ([]domain.SearchResult, error) {
	vec, err := c.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("qdrant: embed query: %w", err)
	}

	results, err := c.conn.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collectionName,
		Query:          qdrant.NewQuery(vec...),
		Limit:          qdrant.PtrOf(uint64(topK)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: search: %w", err)
	}

	searchResults := make([]domain.SearchResult, 0, len(results))
	for _, r := range results {
		chunk := domain.Chunk{
			ID:           r.Id.GetUuid(),
			DocumentName: r.Payload["document_name"].GetStringValue(),
			DocumentID:   r.Payload["document_id"].GetStringValue(),
			Text:         r.Payload["text"].GetStringValue(),
			PageNumber:   int(r.Payload["page_number"].GetIntegerValue()),
		}
		searchResults = append(searchResults, domain.SearchResult{
			Chunk: chunk,
			Score: r.Score,
		})
	}
	return searchResults, nil
}

// DeleteByDocumentID removes all points belonging to the given document.
func (c *Client) DeleteByDocumentID(ctx context.Context, documentID string) error {
	_, err := c.conn.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collectionName,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
				Filter: &qdrant.Filter{
					Must: []*qdrant.Condition{
						qdrant.NewMatchKeyword("document_id", documentID),
					},
				},
			},
		},
	})
	return err
}

// RenameDocumentPayload updates the 'document_name' payload for all points belonging to a document.
func (c *Client) RenameDocumentPayload(ctx context.Context, documentID string, newName string) error {
	_, err := c.conn.SetPayload(ctx, &qdrant.SetPayloadPoints{
		CollectionName: collectionName,
		Payload: map[string]*qdrant.Value{
			"document_name": qdrant.NewValueString(newName),
		},
		PointsSelector: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
				Filter: &qdrant.Filter{
					Must: []*qdrant.Condition{
						qdrant.NewMatchKeyword("document_id", documentID),
					},
				},
			},
		},
	})
	return err
}
