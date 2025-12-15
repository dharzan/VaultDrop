package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/dharsanguruparan/VaultDrop/internal/config"
	"github.com/dharsanguruparan/VaultDrop/internal/queue"
	"github.com/dharsanguruparan/VaultDrop/internal/repository"
	"github.com/dharsanguruparan/VaultDrop/internal/s3storage"
)

// Server exposes HTTP endpoints for uploads and document visibility.
type Server struct {
	cfg    *config.Config
	repo   *repository.DocumentRepository
	store  *s3storage.Storage
	queue  *asynq.Client
	server *http.Server
	once   sync.Once
}

// New constructs a Server.
func New(cfg *config.Config, repo *repository.DocumentRepository, store *s3storage.Storage, queueClient *asynq.Client) *Server {
	return &Server{
		cfg:   cfg,
		repo:  repo,
		store: store,
		queue: queueClient,
	}
}

// Run starts the HTTP server and blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	s.once.Do(func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", s.handleHealth)
		mux.HandleFunc("/documents", s.handleDocuments)
		mux.HandleFunc("/documents/", s.handleDocumentRoute)
		s.server = &http.Server{
			Addr:    s.cfg.Address,
			Handler: corsMiddleware(loggingMiddleware(mux)),
		}
	})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
	}()
	log.Printf("api listening on %s", s.cfg.Address)
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDocuments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleUpload(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDocumentRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/documents/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		s.handleDocument(w, r, id)
		return
	}
	switch parts[1] {
	case "text":
		s.handleDocumentText(w, r, id)
	case "processed-url":
		s.handleProcessedURL(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleDocument(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	doc, err := s.repo.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	respondJSON(w, http.StatusOK, doc)
}

func (s *Server) handleDocumentText(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	doc, err := s.repo.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	if doc.Status != repository.StatusCompleted || doc.Content == "" {
		http.Error(w, "document not processed", http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, doc.Content)
}

func (s *Server) handleProcessedURL(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	doc, err := s.repo.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "document not found", http.StatusNotFound)
		return
	}
	if doc.ProcessedKey == nil {
		http.Error(w, "processed artifact unavailable", http.StatusNotFound)
		return
	}
	url, err := s.store.PresignProcessedURL(r.Context(), *doc.ProcessedKey, int64(s.cfg.SignedURLTTL.Seconds()))
	if err != nil {
		http.Error(w, "failed to generate url", http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"url": url})
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxFileSize+1024)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "expecting multipart form", http.StatusBadRequest)
		return
	}
	part, err := nextFilePart(mr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer part.Close()
	tmp, err := s.persistTemp(part)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer os.Remove(tmp.path)
	defer tmp.f.Close()
	if tmp.contentType != "application/pdf" {
		http.Error(w, "only PDF files supported", http.StatusBadRequest)
		return
	}
	docID := uuid.NewString()
	objectKey := fmt.Sprintf("uploads/%s/%s", docID, filepath.Base(tmp.filename))
	if err := s.uploadToStorage(ctx, objectKey, tmp); err != nil {
		log.Printf("upload to storage failed: %v", err)
		http.Error(w, "failed to store file", http.StatusInternalServerError)
		return
	}
	doc := &repository.Document{
		ID:        docID,
		FileName:  tmp.filename,
		ObjectKey: objectKey,
	}
	if err := s.repo.Create(ctx, doc); err != nil {
		http.Error(w, "failed to store metadata", http.StatusInternalServerError)
		return
	}
	payload := queue.ExtractPayload{
		DocumentID: docID,
		ObjectKey:  objectKey,
		FileName:   tmp.filename,
	}
	if err := queue.EnqueueExtract(ctx, s.queue, payload); err != nil {
		http.Error(w, "failed to queue job", http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusAccepted, map[string]string{
		"id":     docID,
		"status": string(repository.StatusQueued),
	})
}

type tempUpload struct {
	f           *os.File
	path        string
	size        int64
	contentType string
	filename    string
}

func (s *Server) persistTemp(part *multipart.Part) (*tempUpload, error) {
	tmpFile, err := os.CreateTemp("", "vaultdrop-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	var sniff []byte
	buf := make([]byte, 32*1024)
	var written int64
	for {
		n, readErr := part.Read(buf)
		if n > 0 {
			written += int64(n)
			if written > s.cfg.MaxFileSize {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				return nil, fmt.Errorf("file exceeds limit (%d bytes)", s.cfg.MaxFileSize)
			}
			if len(sniff) < 512 {
				chunk := n
				if remain := 512 - len(sniff); chunk > remain {
					chunk = remain
				}
				sniff = append(sniff, buf[:chunk]...)
			}
			if _, err := tmpFile.Write(buf[:n]); err != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				return nil, fmt.Errorf("write temp file: %w", err)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			return nil, fmt.Errorf("read file: %w", readErr)
		}
	}
	if written == 0 {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, errors.New("empty file")
	}
	contentType := http.DetectContentType(sniff)
	if _, err := tmpFile.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("rewind temp file: %w", err)
	}
	filename := part.FileName()
	if filename == "" {
		filename = "upload.pdf"
	}
	return &tempUpload{
		f:           tmpFile,
		path:        tmpFile.Name(),
		size:        written,
		contentType: contentType,
		filename:    filename,
	}, nil
}

func (s *Server) uploadToStorage(ctx context.Context, objectKey string, tmp *tempUpload) error {
	if _, err := tmp.f.Seek(0, 0); err != nil {
		return err
	}
	if err := s.store.UploadRaw(ctx, objectKey, tmp.f, tmp.size, tmp.contentType); err != nil {
		return err
	}
	return nil
}

func nextFilePart(mr *multipart.Reader) (*multipart.Part, error) {
	for {
		part, err := mr.NextPart()
		if err != nil {
			return nil, err
		}
		if part.FormName() == "file" {
			return part, nil
		}
		part.Close()
	}
}

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start))
	})
}
