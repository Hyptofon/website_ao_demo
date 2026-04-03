package http

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/chunker"
	"university-chatbot/backend/internal/infrastructure/parser"
)

// IndexHandler handles document indexing for the admin CLI / seeding.
type IndexHandler struct {
	vectorStore  domain.VectorStore
	chunker      *chunker.Chunker
	pdfExtractor *parser.PDFExtractor
}

// NewIndexHandler constructs the handler.
func NewIndexHandler(vs domain.VectorStore, c *chunker.Chunker, pe *parser.PDFExtractor) *IndexHandler {
	return &IndexHandler{vectorStore: vs, chunker: c, pdfExtractor: pe}
}

// IndexDocumentsFromDir reads all supported files from dir and indexes them into Qdrant.
// Supported formats: .txt, .pdf, .docx, .xlsx
func (h *IndexHandler) IndexDocumentsFromDir(ctx context.Context, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %q: %w", dir, err)
	}

	var indexed, skipped, failed int

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !parser.IsSupported(entry.Name()) {
			log.Printf("  [skip] %s — unsupported format %q", entry.Name(), ext)
			skipped++
			continue
		}

		path := filepath.Join(dir, entry.Name())

		// Extract text based on file type
		var text string
		var extractErr error

		if ext == ".pdf" {
			log.Printf("  [pdf]  %s — extracting via Gemini...", entry.Name())
			text, extractErr = h.pdfExtractor.ExtractText(ctx, path)
		} else {
			text, extractErr = parser.ExtractText(path)
		}

		if extractErr != nil {
			log.Printf("  [FAIL] %s — %v", entry.Name(), extractErr)
			failed++
			continue
		}

		if strings.TrimSpace(text) == "" {
			log.Printf("  [skip] %s — empty content after extraction", entry.Name())
			skipped++
			continue
		}

		// Generate deterministic document ID
		docIDRaw := fmt.Sprintf("%s_%s", entry.Name(), time.Now().Format("2006-01-02"))
		docIDHash := sha256.Sum256([]byte(docIDRaw))
		docID := fmt.Sprintf("%x", docIDHash[:8])

		// Detect language from filename: *_en.* → en, else uk
		lang := "uk"
		nameWithoutExt := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if strings.HasSuffix(strings.ToLower(nameWithoutExt), "_en") {
			lang = "en"
		}

		chunks := h.chunker.Chunk(docID, entry.Name(), "document", lang, text)
		if len(chunks) == 0 {
			log.Printf("  [skip] %s — no chunks extracted (text too short?)", entry.Name())
			skipped++
			continue
		}

		if err := h.vectorStore.UpsertChunks(ctx, chunks); err != nil {
			log.Printf("  [FAIL] %s — upsert error: %v", entry.Name(), err)
			failed++
			continue
		}

		log.Printf("  [ok]   %s → %d chunks (%d chars extracted)", entry.Name(), len(chunks), len(text))
		indexed++
	}

	log.Printf("Indexing summary: %d indexed, %d skipped, %d failed", indexed, skipped, failed)
	return nil
}

// AdminUploadHandler handles POST /admin/documents/upload
func AdminUploadHandler(vs domain.VectorStore, c *chunker.Chunker, pe *parser.PDFExtractor) http.HandlerFunc {
	ih := &IndexHandler{vectorStore: vs, chunker: c, pdfExtractor: pe}
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
		if !parser.IsSupported(header.Filename) {
			jsonError(w, "invalid_type",
				fmt.Sprintf("Unsupported format. Supported: %s", supportedExtList()),
				http.StatusBadRequest)
			return
		}

		data, err := io.ReadAll(file)
		if err != nil {
			jsonError(w, "read_error", "Cannot read file", http.StatusInternalServerError)
			return
		}

		// For uploaded files, save to temp and parse
		ext := strings.ToLower(filepath.Ext(header.Filename))
		var text string

		if ext == ".txt" {
			text = string(data)
		} else {
			// Save to temp file for parser
			tmpFile, err := os.CreateTemp("", "upload-*"+ext)
			if err != nil {
				jsonError(w, "temp_error", "Cannot create temp file", http.StatusInternalServerError)
				return
			}
			defer os.Remove(tmpFile.Name())
			defer tmpFile.Close()

			if _, err := tmpFile.Write(data); err != nil {
				jsonError(w, "write_error", "Cannot write temp file", http.StatusInternalServerError)
				return
			}
			tmpFile.Close()

			if ext == ".pdf" {
				text, err = ih.pdfExtractor.ExtractText(r.Context(), tmpFile.Name())
			} else {
				text, err = parser.ExtractText(tmpFile.Name())
			}
			if err != nil {
				jsonError(w, "parse_error", fmt.Sprintf("Cannot parse file: %v", err), http.StatusBadRequest)
				return
			}
		}

		docIDHash := sha256.Sum256([]byte(fmt.Sprintf("%s_%d", header.Filename, time.Now().UnixNano())))
		docID := fmt.Sprintf("%x", docIDHash[:8])
		chunks := ih.chunker.Chunk(docID, header.Filename, "document", "uk", text)

		if err := vs.UpsertChunks(r.Context(), chunks); err != nil {
			jsonError(w, "index_error", "Failed to index document", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":      "indexed",
			"filename":    header.Filename,
			"chunk_count": len(chunks),
			"chars":       len(text),
		})
	}
}

func supportedExtList() string {
	exts := make([]string, 0, len(parser.SupportedExtensions))
	for ext := range parser.SupportedExtensions {
		exts = append(exts, ext)
	}
	return strings.Join(exts, ", ")
}
