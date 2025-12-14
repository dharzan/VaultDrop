package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DocumentStatus enumerates the lifecycle of a PDF during processing.
type DocumentStatus string

const (
	StatusQueued     DocumentStatus = "queued"
	StatusProcessing DocumentStatus = "processing"
	StatusCompleted  DocumentStatus = "completed"
	StatusFailed     DocumentStatus = "failed"
)

// Document represents a row in the documents table.
type Document struct {
	ID            string         `json:"id"`
	FileName      string         `json:"fileName"`
	ObjectKey     string         `json:"objectKey"`
	ProcessedKey  *string        `json:"processedKey,omitempty"`
	Status        DocumentStatus `json:"status"`
	Content       string         `json:"content,omitempty"`
	ErrorMessage  *string        `json:"errorMessage,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

// DocumentRepository wraps all SQL used throughout the API and worker.
type DocumentRepository struct {
	pool *pgxpool.Pool
}

// NewDocumentRepository constructs a repository.
func NewDocumentRepository(pool *pgxpool.Pool) *DocumentRepository {
	return &DocumentRepository{pool: pool}
}

// Create inserts a queued document before processing begins.
func (r *DocumentRepository) Create(ctx context.Context, doc *Document) error {
	now := time.Now().UTC()
	doc.Status = StatusQueued
	doc.CreatedAt = now
	doc.UpdatedAt = now
	_, err := r.pool.Exec(ctx, `
		INSERT INTO documents (id, file_name, object_key, status, content, error_message, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, doc.ID, doc.FileName, doc.ObjectKey, doc.Status, "", nil, doc.CreatedAt, doc.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert document: %w", err)
	}
	return nil
}

// Get returns a document by id.
func (r *DocumentRepository) Get(ctx context.Context, id string) (*Document, error) {
	var (
		doc          Document
		processedKey sql.NullString
		errorMsg     sql.NullString
	)
	row := r.pool.QueryRow(ctx, `
		SELECT id, file_name, object_key, processed_key, status, COALESCE(content,''), error_message, created_at, updated_at
		FROM documents WHERE id=$1
	`, id)
	if err := row.Scan(&doc.ID, &doc.FileName, &doc.ObjectKey, &processedKey, &doc.Status, &doc.Content, &errorMsg, &doc.CreatedAt, &doc.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("document not found: %w", err)
		}
		return nil, fmt.Errorf("select document: %w", err)
	}
	if processedKey.Valid {
		key := processedKey.String
		doc.ProcessedKey = &key
	}
	if errorMsg.Valid {
		msg := errorMsg.String
		doc.ErrorMessage = &msg
	}
	return &doc, nil
}

// MarkProcessing sets the status to processing.
func (r *DocumentRepository) MarkProcessing(ctx context.Context, id string) error {
	return r.updateStatus(ctx, id, StatusProcessing, nil, nil, nil)
}

// MarkFailed marks the processing attempt as failed and stores the message.
func (r *DocumentRepository) MarkFailed(ctx context.Context, id string, msg string) error {
	return r.updateStatus(ctx, id, StatusFailed, nil, nil, &msg)
}

// MarkCompleted updates the status and stores the processed artifact references.
func (r *DocumentRepository) MarkCompleted(ctx context.Context, id, processedKey, content string) error {
	return r.updateStatus(ctx, id, StatusCompleted, &processedKey, &content, nil)
}

func (r *DocumentRepository) updateStatus(ctx context.Context, id string, status DocumentStatus, processedKey *string, content *string, errorMsg *string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE documents
		SET status=$1,
			processed_key = COALESCE($2, processed_key),
			content = COALESCE($3, content),
			error_message = $4,
			updated_at=$5
		WHERE id=$6
	`, status, processedKey, content, errorMsg, now, id)
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}
	return nil
}
