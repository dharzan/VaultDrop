// Package server wires together HTTP routes, dependency injection, and business
// logic. Go's net/http package builds servers via handler functions which
// receive http.ResponseWriter + *http.Request.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dharsanguruparan/VaultDrop/internal/config"
	"github.com/dharsanguruparan/VaultDrop/internal/model"
	"github.com/dharsanguruparan/VaultDrop/internal/processing"
	"github.com/dharsanguruparan/VaultDrop/internal/signing"
	"github.com/dharsanguruparan/VaultDrop/internal/storage"
)

// Server hosts HTTP handlers for VaultDrop. It stitches together configuration,
// storage, background processing, and signing helpers. Struct embedding is not
// needed here; fields are explicitly referenced for clarity.
type Server struct {
	cfg       *config.Config
	store     *storage.MemoryStore
	processor *processing.Processor
	signer    *signing.Signer
	uploadDir string
	once      sync.Once
}

// New creates a configured server. In Go it's conventional to return (*Type,
// error) so callers can handle initialization failures (e.g., inability to
// create the upload directory).
func New(cfg *config.Config, store *storage.MemoryStore, processor *processing.Processor, signer *signing.Signer) (*Server, error) {
	dir := filepath.Join(os.TempDir(), "vaultdrop")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &Server{
		cfg:       cfg,
		store:     store,
		processor: processor,
		signer:    signer,
		uploadDir: dir,
	}, nil
}

// Serve launches the HTTP server until the context is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	s.once.Do(func() {
		// sync.Once ensures we only start the background workers once even if
		// Serve is called multiple times in tests.
		s.processor.Start(ctx)
	})
	httpServer := &http.Server{
		Addr:    s.cfg.Address,
		Handler: s.routes(),
	}
	go func() {
		<-ctx.Done()
		// When the context is cancelled we gracefully shutdown with a timeout.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	// HandleFunc registers path-specific handler functions on the ServeMux.
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/download", s.handleDownload)
	mux.HandleFunc("/files/", s.handleFileRoute)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Respond with JSON so clients can confirm the process is alive.
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// http.MaxBytesReader wraps the Body to protect against oversized payloads.
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxFileSize+1024)
	// MultipartReader parses streaming uploads without loading entire files
	// into memory, which is a big reason Go is great for large uploads.
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "expecting multipart form", http.StatusBadRequest)
		return
	}
	var saved *model.FileRecord
	for {
		// MultipartReader.NextPart streams one part at a time; io.EOF indicates
		// there are no more parts.
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			http.Error(w, "failed to read upload", http.StatusBadRequest)
			return
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		// Persist the first file part we encounter and ignore others.
		record, err := s.persistPart(part)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		saved = record
		break
	}
	if saved == nil {
		http.Error(w, "missing file part", http.StatusBadRequest)
		return
	}
	if err := s.scan(saved); err != nil {
		_ = os.Remove(saved.Path)
		// Errors are ignored because the best effort update suffices for API.
		_ = s.store.UpdateStatus(saved.ID, model.StatusRejected, err.Error())
		http.Error(w, "file rejected: "+err.Error(), http.StatusBadRequest)
		return
	}
	_ = s.store.UpdateStatus(saved.ID, model.StatusScanned, "scan clean")
	_ = s.store.UpdateStatus(saved.ID, model.StatusQueued, "queued for processing")
	s.processor.Submit(processing.Job{FileID: saved.ID})
	respondJSON(w, http.StatusAccepted, map[string]string{
		"id":     saved.ID,
		"status": string(model.StatusQueued),
	})
}

func (s *Server) handleFileRoute(w http.ResponseWriter, r *http.Request) {
	// The /files/ prefix supports nested resources like /files/{id}/signed-url.
	path := strings.TrimPrefix(r.URL.Path, "/files/")
	// strings.Split returns a slice; we inspect segments to route requests.
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		s.handleFileInfo(w, r, id)
		return
	}
	if parts[1] == "signed-url" {
		s.handleSignedURL(w, r, id)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleFileInfo(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	record, err := s.store.Get(id)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	// Avoid leaking server-side paths when returning JSON.
	record.Path = ""
	respondJSON(w, http.StatusOK, record)
}

func (s *Server) handleSignedURL(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := s.store.Get(id); err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	// Build a short-lived URL by combining the ID, expiry timestamp, and HMAC
	// signature. Unix() returns seconds since epoch which is easy to transmit.
	expiry := time.Now().Add(s.cfg.SignedURLTTL).Unix()
	signature := s.signer.Sign(id, expiry)
	downloadURL := &urlBuilder{
		base: "/download",
		params: map[string]string{
			"file":      id,
			"expires":   strconv.FormatInt(expiry, 10),
			"signature": signature,
		},
	}
	respondJSON(w, http.StatusOK, map[string]string{
		"url":     downloadURL.String(),
		"expires": strconv.FormatInt(expiry, 10),
	})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Query parameters are retrieved via r.URL.Query().Get().
	id := r.URL.Query().Get("file")
	expires := r.URL.Query().Get("expires")
	signature := r.URL.Query().Get("signature")
	if id == "" || expires == "" || signature == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}
	expiryUnix, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		http.Error(w, "invalid expires", http.StatusBadRequest)
		return
	}
	if time.Unix(expiryUnix, 0).Before(time.Now()) {
		http.Error(w, "url expired", http.StatusUnauthorized)
		return
	}
	if !s.signer.Validate(id, expires, signature) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	record, err := s.store.Get(id)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	f, err := os.Open(record.Path)
	if err != nil {
		http.Error(w, "file unavailable", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	// HTTP headers describe the file; ServeContent streams data efficiently.
	w.Header().Set("Content-Type", record.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(record.Size, 10))
	w.Header().Set("Content-Disposition", "attachment; filename=\""+record.Name+"\"")
	http.ServeContent(w, r, record.Name, record.UpdatedAt, f)
}

func (s *Server) persistPart(part *multipart.Part) (*model.FileRecord, error) {
	defer part.Close()
	fileID := randomID()
	path := filepath.Join(s.uploadDir, fileID)
	dst, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	defer dst.Close()
	var sniff []byte
	// Allocate a 32 KiB buffer reused for every Read call; this keeps memory
	// usage bounded regardless of upload size.
	buf := make([]byte, 32*1024)
	// written tracks bytes persisted so we can enforce the configured limit.
	var written int64
	for {
		n, readErr := part.Read(buf)
		if n > 0 {
			written += int64(n)
			if written > s.cfg.MaxFileSize {
				os.Remove(path)
				return nil, errors.New("file exceeds limit")
			}
			// Capture up to 512 bytes so http.DetectContentType can sniff the
			// MIME type according to RFC 2616.
			if len(sniff) < 512 {
				chunk := n
				if remain := 512 - len(sniff); chunk > remain {
					chunk = remain
				}
				sniff = append(sniff, buf[:chunk]...)
			}
			if _, err := dst.Write(buf[:n]); err != nil {
				os.Remove(path)
				return nil, err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			os.Remove(path)
			return nil, readErr
		}
	}
	if written == 0 {
		os.Remove(path)
		return nil, errors.New("empty file")
	}
	contentType := http.DetectContentType(sniff)
	if !s.allowedType(contentType) {
		os.Remove(path)
		return nil, errors.New("file type not allowed")
	}
	name := part.FileName()
	if name == "" {
		// Some clients omit filenames, so we generate a deterministic fallback.
		name = "upload-" + fileID
	}
	record := &model.FileRecord{
		ID:          fileID,
		Name:        name,
		Size:        written,
		ContentType: contentType,
		Path:        path,
		Status:      model.StatusUploaded,
	}
	s.store.Save(record)
	return record, nil
}

func (s *Server) scan(record *model.FileRecord) error {
	data, err := os.ReadFile(record.Path)
	if err != nil {
		return err
	}
	// Our toy scanner simply searches for the word "virus" ignoring case; in a
	// real system this would call out to an AV engine.
	if strings.Contains(strings.ToLower(string(data)), "virus") {
		return errors.New("malware signature detected")
	}
	// Sleep simulates the latency of a real AV scan without doing heavy work.
	time.Sleep(300 * time.Millisecond)
	return nil
}

func (s *Server) allowedType(contentType string) bool {
	for _, allowed := range s.cfg.AllowedTypes {
		// range returns index+value when iterating slices; we ignore index via _
		if allowed == contentType {
			return true
		}
	}
	return false
}

func randomID() string {
	// IDs are random hex strings so they are easy to include in URLs.
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// If the secure RNG fails we fall back to a timestamp-based identifier.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

type urlBuilder struct {
	// Small helper struct to keep signed URL creation tidy. Maps in Go are
	// reference types so we can mutate params if needed.
	base   string
	params map[string]string
}

func (u *urlBuilder) String() string {
	q := make([]string, 0, len(u.params))
	for k, v := range u.params {
		// url.QueryEscape ensures values are encoded safely for URLs.
		q = append(q, k+"="+url.QueryEscape(v))
	}
	return u.base + "?" + strings.Join(q, "&")
}

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	// ResponseWriter exposes headers + status writing; once WriteHeader is
	// called we must send the body, so always set headers first.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		log.Printf("encode json failed: %v", err)
	}
}
