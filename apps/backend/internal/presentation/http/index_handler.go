package http

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/chunker"
)

// IndexHandler handles document indexing for the admin CLI / seeding.
type IndexHandler struct {
	vectorStore domain.VectorStore
	chunker     *chunker.Chunker
}

// NewIndexHandler constructs the handler.
func NewIndexHandler(vs domain.VectorStore, c *chunker.Chunker) *IndexHandler {
	return &IndexHandler{vectorStore: vs, chunker: c}
}

// IndexDocumentsFromDir reads all .txt files from dir and indexes them into Qdrant.
func (h *IndexHandler) IndexDocumentsFromDir(ctx context.Context, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %q: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".txt" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %q: %w", path, err)
		}

		docIDRaw := fmt.Sprintf("%s_%s", entry.Name(), time.Now().Format("2006-01-02"))
		docIDHash := sha256.Sum256([]byte(docIDRaw))
		docID := fmt.Sprintf("%x", docIDHash[:8])

		// Detect language from filename: *_en.txt → en, else uk
		lang := "uk"
		if len(entry.Name()) > 7 && entry.Name()[len(entry.Name())-7:len(entry.Name())-4] == "_en" {
			lang = "en"
		}

		chunks := h.chunker.Chunk(docID, entry.Name(), "document", lang, string(data))
		if len(chunks) == 0 {
			fmt.Printf("  [skip] %s — no chunks extracted\n", entry.Name())
			continue
		}

		if err := h.vectorStore.UpsertChunks(ctx, chunks); err != nil {
			return fmt.Errorf("upsert %q: %w", entry.Name(), err)
		}
		fmt.Printf("  [ok] %s → %d chunks\n", entry.Name(), len(chunks))
	}
	return nil
}

// AdminUploadHandler handles POST /admin/documents/upload
func AdminUploadHandler(vs domain.VectorStore, c *chunker.Chunker) http.HandlerFunc {
	ih := &IndexHandler{vectorStore: vs, chunker: c}
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(50 << 20); err != nil {
			jsonError(w, "invalid_form", "Cannot parse multipart form", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			jsonError(w, "missing_file", "No file in request", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Validate extension
		ext := filepath.Ext(header.Filename)
		if ext != ".txt" && ext != ".pdf" && ext != ".docx" {
			jsonError(w, "invalid_type", "Only .txt files supported in Phase 1", http.StatusBadRequest)
			return
		}

		data, err := io.ReadAll(file)
		if err != nil {
			jsonError(w, "read_error", "Cannot read file", http.StatusInternalServerError)
			return
		}

		docID := fmt.Sprintf("%x%d", header.Filename, time.Now().UnixNano())
		chunks := ih.chunker.Chunk(docID, header.Filename, "document", "uk", string(data))

		if err := vs.UpsertChunks(r.Context(), chunks); err != nil {
			jsonError(w, "index_error", "Failed to index document", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":      "indexed",
			"filename":    header.Filename,
			"chunk_count": len(chunks),
		})
	}
}
