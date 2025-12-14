package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const (
	// ExtractDocumentTask is scheduled each time a PDF is uploaded.
	ExtractDocumentTask = "document:extract"
)

// ExtractPayload is serialized into the task payload so the worker knows which
// object to download from MinIO.
type ExtractPayload struct {
	DocumentID string `json:"document_id"`
	ObjectKey  string `json:"object_key"`
	FileName   string `json:"file_name"`
}

// EnqueueExtract enqueues a PDF extraction job.
func EnqueueExtract(ctx context.Context, client *asynq.Client, payload ExtractPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	task := asynq.NewTask(ExtractDocumentTask, data)
	if _, err := client.EnqueueContext(ctx, task, asynq.MaxRetry(5)); err != nil {
		return fmt.Errorf("enqueue extract task: %w", err)
	}
	return nil
}
