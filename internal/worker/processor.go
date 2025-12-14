package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/hibiken/asynq"

	pdfutil "github.com/dharsanguruparan/VaultDrop/internal/pdf"
	"github.com/dharsanguruparan/VaultDrop/internal/queue"
	"github.com/dharsanguruparan/VaultDrop/internal/repository"
	"github.com/dharsanguruparan/VaultDrop/internal/s3storage"
)

// Processor is plugged into the asynq worker loop.
type Processor struct {
	repo  *repository.DocumentRepository
	store *s3storage.Storage
}

// NewProcessor constructs a worker processor.
func NewProcessor(repo *repository.DocumentRepository, store *s3storage.Storage) *Processor {
	return &Processor{repo: repo, store: store}
}

// Handler registers the extract job handler.
func (p *Processor) Handler() *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.ExtractDocumentTask, p.handleExtract)
	return mux
}

func (p *Processor) handleExtract(ctx context.Context, task *asynq.Task) error {
	var payload queue.ExtractPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	failure := func(err error) error {
		log.Printf("extract failed for %s: %v", payload.DocumentID, err)
		_ = p.repo.MarkFailed(ctx, payload.DocumentID, err.Error())
		return err
	}
	if err := p.repo.MarkProcessing(ctx, payload.DocumentID); err != nil {
		return failure(err)
	}
	data, err := p.store.DownloadRaw(ctx, payload.ObjectKey)
	if err != nil {
		return failure(err)
	}
	text, err := pdfutil.ExtractText(data)
	if err != nil {
		return failure(err)
	}
	processedKey := processedObjectKey(payload.ObjectKey)
	if err := p.store.UploadProcessed(ctx, processedKey, []byte(text)); err != nil {
		return failure(err)
	}
	if err := p.repo.MarkCompleted(ctx, payload.DocumentID, processedKey, text); err != nil {
		return failure(err)
	}
	log.Printf("document %s processed (%d bytes)", payload.DocumentID, len(text))
	return nil
}

func processedObjectKey(objectKey string) string {
	base := strings.TrimSuffix(objectKey, filepath.Ext(objectKey))
	return fmt.Sprintf("%s.txt", base)
}
