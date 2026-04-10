package http

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"university-chatbot/backend/internal/domain"
	"university-chatbot/backend/internal/infrastructure/chunker"
	"university-chatbot/backend/internal/infrastructure/gemini"
	"university-chatbot/backend/internal/infrastructure/parser"
	"university-chatbot/backend/internal/infrastructure/sqlite"
)

// IndexHandler handles document indexing for the admin CLI / seeding.
type IndexHandler struct {
	vectorStore   domain.VectorStore
	chunker       *chunker.Chunker
	pdfExtractor  *parser.PDFExtractor
	jobsRepo      *sqlite.JobRepository
	metaExtractor *gemini.MetadataExtractor
}

// NewIndexHandler constructs the handler.
func NewIndexHandler(vs domain.VectorStore, c *chunker.Chunker, pe *parser.PDFExtractor, jobs *sqlite.JobRepository, me *gemini.MetadataExtractor) *IndexHandler {
	return &IndexHandler{vectorStore: vs, chunker: c, pdfExtractor: pe, jobsRepo: jobs, metaExtractor: me}
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

		docIDRaw := fmt.Sprintf("%s_%s", entry.Name(), time.Now().Format("2006-01-02"))
		docIDHash := sha256.Sum256([]byte(docIDRaw))
		docID := fmt.Sprintf("%x", docIDHash[:8])

		lang := "uk"
		nameWithoutExt := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if strings.HasSuffix(strings.ToLower(nameWithoutExt), "_en") {
			lang = "en"
		}

		chunks := h.chunker.Chunk(docID, entry.Name(), "document", lang, text)
		if len(chunks) == 0 {
			log.Printf("  [skip] %s — no chunks extracted", entry.Name())
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

// HandleAdminUpload handles POST /admin/documents/upload asynchronously.
func (h *IndexHandler) HandleAdminUpload(w http.ResponseWriter, r *http.Request) {
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

	if !parser.IsSupported(header.Filename) {
		jsonError(w, "invalid_type", fmt.Sprintf("Unsupported format. Supported: %s", supportedExtList()), http.StatusBadRequest)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		jsonError(w, "read_error", "Cannot read file", http.StatusInternalServerError)
		return
	}

	// Create job record
	jobIDHash := sha256.Sum256([]byte(fmt.Sprintf("%s_%d", header.Filename, time.Now().UnixNano())))
	jobID := fmt.Sprintf("%x", jobIDHash[:8])

	job := &domain.UploadJob{
		ID:       jobID,
		Filename: header.Filename,
		Status:   domain.JobStatusPending,
	}

	if err := h.jobsRepo.CreateJob(r.Context(), job); err != nil {
		jsonError(w, "db_error", "Cannot create job record", http.StatusInternalServerError)
		return
	}

	// Save to temp file for the background worker
	ext := strings.ToLower(filepath.Ext(header.Filename))
	tmpFile, err := os.CreateTemp("", "upload-"+jobID+"-*"+ext)
	if err != nil {
		h.jobsRepo.UpdateJobStatus(r.Context(), jobID, domain.JobStatusFailed, err)
		jsonError(w, "temp_error", "Cannot create temp file", http.StatusInternalServerError)
		return
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(data); err != nil {
		h.jobsRepo.UpdateJobStatus(r.Context(), jobID, domain.JobStatusFailed, err)
		jsonError(w, "write_error", "Cannot write temp file", http.StatusInternalServerError)
		return
	}

	filepath := tmpFile.Name()

	// Spin background worker
	go h.processBackgroundUpload(jobID, header.Filename, filepath)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202 Accepted
	json.NewEncoder(w).Encode(map[string]string{
		"status": "accepted",
		"job_id": jobID,
	})
}

func (h *IndexHandler) processBackgroundUpload(jobID, originalFilename, filepath string) {
	ctx := context.Background()
	_ = h.jobsRepo.UpdateJobStatus(ctx, jobID, domain.JobStatusProcessing, nil)
	_ = h.jobsRepo.UpdateProgress(ctx, jobID, 10, "Starting text extraction...")

	// Ensure file cleanup after processing
	defer os.Remove(filepath)

	var text string
	var err error

	ext := strings.ToLower(filepath[strings.LastIndex(filepath, "."):])
	if ext == ".txt" {
		data, readErr := os.ReadFile(filepath)
		if readErr != nil {
			err = readErr
		} else {
			text = string(data)
		}
	} else if ext == ".pdf" {
		_ = h.jobsRepo.UpdateProgress(ctx, jobID, 15, "Extracting text from PDF via Gemini...")
		text, err = h.pdfExtractor.ExtractText(ctx, filepath)
	} else {
		text, err = parser.ExtractText(filepath)
	}

	if err != nil {
		_ = h.jobsRepo.UpdateJobStatus(ctx, jobID, domain.JobStatusFailed, fmt.Errorf("extraction error: %w", err))
		return
	}

	_ = h.jobsRepo.UpdateProgress(ctx, jobID, 30, "Text extracted, detecting metadata...")

	// --- Auto-detect document metadata via Gemini Structured Output ---
	lang := "uk"
	docType := "general"
	if h.metaExtractor != nil {
		meta, metaErr := h.metaExtractor.ExtractMetadata(ctx, text)
		if metaErr != nil {
			log.Printf("[WARN] Metadata extraction failed for %s, using defaults: %v", originalFilename, metaErr)
		} else {
			lang = meta.Language
			docType = meta.DocType
			log.Printf("[INFO] Detected metadata for %s: lang=%s, type=%s", originalFilename, lang, docType)
		}
	}

	_ = h.jobsRepo.UpdateProgress(ctx, jobID, 50, "Splitting document into chunks...")

	chunks := h.chunker.Chunk(jobID, originalFilename, docType, lang, text)
	if len(chunks) == 0 {
		_ = h.jobsRepo.UpdateJobStatus(ctx, jobID, domain.JobStatusFailed, fmt.Errorf("no chunks extracted, file might be empty"))
		return
	}

	_ = h.jobsRepo.UpdateProgress(ctx, jobID, 60, fmt.Sprintf("Generating embeddings for %d chunks...", len(chunks)))

	if err := h.vectorStore.UpsertChunks(ctx, chunks); err != nil {
		_ = h.jobsRepo.UpdateJobStatus(ctx, jobID, domain.JobStatusFailed, fmt.Errorf("qdrant error: %w", err))
		return
	}

	_ = h.jobsRepo.UpdateChunksCount(ctx, jobID, len(chunks))
	_ = h.jobsRepo.UpdateProgress(ctx, jobID, 100, "Indexing completed")
	_ = h.jobsRepo.UpdateJobStatus(ctx, jobID, domain.JobStatusCompleted, nil)
}

// GetJobStatus handles GET /admin/documents/jobs/{job_id}
func (h *IndexHandler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	if jobID == "" {
		jsonError(w, "missing_job_id", "job_id missing in URL", http.StatusBadRequest)
		return
	}

	job, err := h.jobsRepo.GetJob(r.Context(), jobID)
	if err == sqlite.ErrJobNotFound {
		jsonError(w, "not_found", "job not found", http.StatusNotFound)
		return
	} else if err != nil {
		jsonError(w, "db_error", "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(job)
}

func supportedExtList() string {
	exts := make([]string, 0, len(parser.SupportedExtensions))
	for ext := range parser.SupportedExtensions {
		exts = append(exts, ext)
	}
	return strings.Join(exts, ", ")
}

// HandleDeleteDocument handles DELETE /admin/documents/{document_id}.
// Removes all chunks belonging to a document from the vector store.
func (h *IndexHandler) HandleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	documentID := chi.URLParam(r, "document_id")
	if documentID == "" {
		jsonError(w, "missing_document_id", "document_id missing in URL", http.StatusBadRequest)
		return
	}

	slog.Info("Deleting document from vector store", "document_id", documentID)

	if err := h.vectorStore.DeleteByDocumentID(r.Context(), documentID); err != nil {
		slog.Error("Failed to delete document", "document_id", documentID, "error", err)
		jsonError(w, "delete_error", "Failed to delete document chunks", http.StatusInternalServerError)
		return
	}

	slog.Info("Document deleted successfully", "document_id", documentID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "deleted",
		"document_id": documentID,
	})
}
