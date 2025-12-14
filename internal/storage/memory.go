// Package storage contains the in-memory metadata persistence layer. Go keeps
// each package in its own folder; files in the folder share a namespace.
package storage

import (
	"errors"
	"sync"
	"time"

	"github.com/dharsanguruparan/VaultDrop/internal/model"
)

var (
	// ErrNotFound is exported so callers elsewhere can compare errors using
	// errors.Is; Go encourages sentinel errors for simple cases.
	ErrNotFound = errors.New("file not found")
)

// MemoryStore provides an in-memory metadata store using RWMutex. RWMutex lets
// us differentiate read locks (multiple concurrent readers) from write locks
// (single writer), which suits the request-heavy nature of APIs.
type MemoryStore struct {
	mu    sync.RWMutex
	files map[string]*model.FileRecord
}

// NewMemoryStore constructs a MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		files: make(map[string]*model.FileRecord),
	}
}

// Save inserts or replaces a record.
func (m *MemoryStore) Save(record *model.FileRecord) {
	m.mu.Lock()
	// defer schedules code to run when the function returns, guaranteeing the
	// mutex unlock even if the function exits early.
	defer m.mu.Unlock()
	// time.Now returns local time; calling UTC standardizes timestamps for API.
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	m.files[record.ID] = record
}

// UpdateStatus updates status/message.
func (m *MemoryStore) UpdateStatus(id string, status model.FileStatus, msg string) error {
	m.mu.Lock()
	// As with Save we guard the map with the write lock.
	defer m.mu.Unlock()
	// Maps in Go return (value, bool) when looked up; bool indicates presence.
	rec, ok := m.files[id]
	if !ok {
		return ErrNotFound
	}
	rec.Status = status
	rec.Message = msg
	rec.UpdatedAt = time.Now().UTC()
	return nil
}

// Get returns a record copy.
func (m *MemoryStore) Get(id string) (*model.FileRecord, error) {
	m.mu.RLock()
	// Read locks allow multiple concurrent readers, improving throughput.
	defer m.mu.RUnlock()
	rec, ok := m.files[id]
	if !ok {
		return nil, ErrNotFound
	}
	// Returning a shallow copy prevents callers from mutating internal state.
	copy := *rec
	return &copy, nil
}
