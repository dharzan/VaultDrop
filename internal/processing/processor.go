// Package processing simulates a background worker pool that handles long
// running tasks. Goroutines + channels (core Go concurrency primitives) power
// the implementation.
package processing

import (
	"context"
	"log"
	"time"

	"github.com/dharsanguruparan/VaultDrop/internal/model"
	"github.com/dharsanguruparan/VaultDrop/internal/storage"
)

// Job represents background processing work. Simple structs like this make it
// easy to extend later without changing channel type signatures.
type Job struct {
	FileID string
}

// Processor consumes Jobs and updates their lifecycle.
type Processor struct {
	store   *storage.MemoryStore
	queue   chan Job
	workers int
}

// New builds a Processor with queue capacity tied to worker count.
func New(store *storage.MemoryStore, workers int) *Processor {
	if workers <= 0 {
		workers = 1
	}
	return &Processor{
		store: store,
		// make(chan T, N) creates a buffered channel that can hold N messages
		// without blocking producers, keeping uploads responsive.
		queue:   make(chan Job, workers*4),
		workers: workers,
	}
}

// Start launches worker goroutines.
func (p *Processor) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		// go keyword starts a new goroutine (lightweight thread managed by the
		// Go runtime). Each worker listens for jobs until the context closes.
		go p.worker(ctx)
	}
}

// Submit queues a job for async processing.
func (p *Processor) Submit(job Job) {
	select {
	case p.queue <- job:
	default:
		// default branch activates when the channel buffer is full; we opt to
		// drop work but mark the file failed so the API reflects reality.
		log.Printf("processor queue full, dropping job for %s", job.FileID)
		_ = p.store.UpdateStatus(job.FileID, model.StatusFailed, "processing queue full")
	}
}

func (p *Processor) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// Propagate cancellation by exiting the goroutine when the context
			// is cancelled (triggered by signal handling in main.go).
			return
		case job := <-p.queue:
			p.process(job)
		}
	}
}

func (p *Processor) process(job Job) {
	if err := p.store.UpdateStatus(job.FileID, model.StatusProcessing, "processing started"); err != nil {
		return
	}
	// Simulate heavy work
	time.Sleep(2 * time.Second)
	if err := p.store.UpdateStatus(job.FileID, model.StatusComplete, "processing finished"); err != nil {
		log.Printf("update status failed: %v", err)
	}
}
