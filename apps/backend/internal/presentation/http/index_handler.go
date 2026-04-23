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
	documentRepo  domain.DocumentRepo 
	auditRepo     domain.AuditRepo   
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
		slog.Error("Upload: ParseMultipartForm failed", "error", err)
		jsonError(w, "invalid_form", "Cannot parse multipart form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		slog.Error("Upload: FormFile 'file' missing", "error", err)
		jsonError(w, "missing_file", "No file in request", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !parser.IsSupported(header.Filename) {
		slog.Error("Upload: Unsupported format", "filename", header.Filename)
		jsonError(w, "invalid_type", fmt.Sprintf("Unsupported format. Supported: %s", supportedExtList()), http.StatusBadRequest)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		slog.Error("Upload: ReadAll file failed", "error", err)
		jsonError(w, "read_error", "Cannot read file", http.StatusInternalServerError)
		return
	}

	// TZ §3.5 / §6.1: MIME-type validation — check actual file content, not just extension.
	// http.DetectContentType reads the first 512 bytes to determine the real MIME type.
	if !validateMIMEType(data, header.Filename) {
		slog.Warn("Upload: MIME type mismatch", "filename", header.Filename)
		jsonError(w, "invalid_mime", "File content does not match its extension", http.StatusBadRequest)
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
		slog.Error("Upload: CreateJob failed", "error", err, "jobID", jobID)
		jsonError(w, "db_error", "Cannot create job record", http.StatusInternalServerError)
		return
	}

	// Save to permanent storage for viewing and background worker
	ext := strings.ToLower(filepath.Ext(header.Filename))
	docsDir := filepath.Join(".", "data", "documents")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		slog.Error("Upload: MkdirAll docsDir failed", "error", err)
		h.jobsRepo.UpdateJobStatus(r.Context(), jobID, domain.JobStatusFailed, err)
		jsonError(w, "fs_error", "Cannot create documents directory", http.StatusInternalServerError)
		return
	}

	savedFilepath := filepath.Join(docsDir, jobID+ext)
	outFile, err := os.Create(savedFilepath)
	if err != nil {
		slog.Error("Upload: Create file failed", "error", err)
		h.jobsRepo.UpdateJobStatus(r.Context(), jobID, domain.JobStatusFailed, err)
		jsonError(w, "fs_error", "Cannot create permanent file", http.StatusInternalServerError)
		return
	}
	defer outFile.Close()

	if _, err := outFile.Write(data); err != nil {
		h.jobsRepo.UpdateJobStatus(r.Context(), jobID, domain.JobStatusFailed, err)
		jsonError(w, "write_error", "Cannot write permanent file", http.StatusInternalServerError)
		return
	}



	// Spin background worker — pass admin email from context for audit trail
	adminEmail := AdminEmailFromCtx(r.Context())
	go h.processBackgroundUpload(jobID, header.Filename, savedFilepath, adminEmail)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202 Accepted
	json.NewEncoder(w).Encode(map[string]string{
		"status": "accepted",
		"job_id": jobID,
	})
}

func (h *IndexHandler) processBackgroundUpload(jobID, originalFilename, filePath, adminEmail string) {
	ctx := context.Background()
	_ = h.jobsRepo.UpdateJobStatus(ctx, jobID, domain.JobStatusProcessing, nil)
	_ = h.jobsRepo.UpdateProgress(ctx, jobID, 10, "Starting text extraction...")

	var text string
	var err error

	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".txt" {
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			err = readErr
		} else {
			text = string(data)
		}
	} else if ext == ".pdf" {
		_ = h.jobsRepo.UpdateProgress(ctx, jobID, 15, "Extracting text from PDF via Gemini...")
		text, err = h.pdfExtractor.ExtractText(ctx, filePath)
	} else {
		text, err = parser.ExtractText(filePath)
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

	// Phase 2: Track document in SQLite for admin panel
	if h.documentRepo != nil {
		_ = h.documentRepo.Create(ctx, &domain.DocumentRecord{
			ID:         jobID,
			Filename:   originalFilename,
			DocType:    docType,
			Language:   domain.Language(lang),
			ChunkCount: len(chunks),
			UploadedBy: adminEmail,
		})
	}
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
// Removes all chunks from the vector store and the document record from SQLite.
func (h *IndexHandler) HandleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	documentID := chi.URLParam(r, "document_id")
	if documentID == "" {
		jsonError(w, "missing_document_id", "document_id missing in URL", http.StatusBadRequest)
		return
	}

	adminEmail := AdminEmailFromCtx(r.Context())
	slog.Info("Deleting document", "document_id", documentID, "admin", adminEmail)

	if err := h.vectorStore.DeleteByDocumentID(r.Context(), documentID); err != nil {
		slog.Error("Failed to delete document from Qdrant", "document_id", documentID, "error", err)
		jsonError(w, "delete_error", "Failed to delete document chunks", http.StatusInternalServerError)
		return
	}

	// Phase 2: Remove from SQLite document registry
	if h.documentRepo != nil {
		if err := h.documentRepo.Delete(r.Context(), documentID); err != nil {
			slog.Warn("Document not in SQLite registry (CLI-indexed?)", "document_id", documentID)
		}
	}

	slog.Info("Document deleted successfully", "document_id", documentID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "deleted",
		"document_id": documentID,
	})
}

// HandleReindexDocument handles POST /admin/documents/{document_id}/reindex.
// Re-indexes a single document by deleting its existing chunks and re-processing the stored file.
// TZ §3.3 / §4.2: re-indexation endpoint.
func (h *IndexHandler) HandleReindexDocument(w http.ResponseWriter, r *http.Request) {
	documentID := chi.URLParam(r, "document_id")
	if documentID == "" {
		jsonError(w, "missing_document_id", "document_id missing in URL", http.StatusBadRequest)
		return
	}

	// Look up the document record to get filename and extension
	if h.documentRepo == nil {
		jsonError(w, "not_configured", "Document repository not configured", http.StatusInternalServerError)
		return
	}

	doc, err := h.documentRepo.GetByID(r.Context(), documentID)
	if err != nil {
		jsonError(w, "not_found", "Document not found", http.StatusNotFound)
		return
	}

	// Find the stored file on disk
	ext := strings.ToLower(filepath.Ext(doc.Filename))
	filePath := filepath.Join(".", "data", "documents", documentID+ext)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		jsonError(w, "file_missing", "Raw file not found on disk, cannot re-index", http.StatusNotFound)
		return
	}

	// Delete existing chunks from Qdrant
	if err := h.vectorStore.DeleteByDocumentID(r.Context(), documentID); err != nil {
		slog.Error("Reindex: failed to delete old chunks", "document_id", documentID, "error", err)
		jsonError(w, "delete_error", "Failed to delete old chunks", http.StatusInternalServerError)
		return
	}

	adminEmail := AdminEmailFromCtx(r.Context())

	// Spin background re-indexing
	go h.processBackgroundUpload(documentID, doc.Filename, filePath, adminEmail)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "reindex_started",
		"document_id": documentID,
	})
}

// ── MIME-type validation ─────────────────────────────────────────────────────
// TZ §3.5 / §6.1: Validate actual file content, not just extension.

// allowedMIMEs maps file extensions to their expected MIME types.
var allowedMIMEs = map[string][]string{
	".txt":  {"text/plain"},
	".pdf":  {"application/pdf"},
	".docx": {"application/zip", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/octet-stream"},
	".xlsx": {"application/zip", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "application/octet-stream"},
}

// validateMIMEType checks that the file content matches the extension's expected MIME type.
func validateMIMEType(data []byte, filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	allowed, ok := allowedMIMEs[ext]
	if !ok {
		return false
	}

	detected := http.DetectContentType(data)
	for _, mime := range allowed {
		if strings.HasPrefix(detected, mime) {
			return true
		}
	}

	slog.Warn("MIME type mismatch", "filename", filename, "extension", ext, "detected", detected)
	return false
}
