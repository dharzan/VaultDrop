// Package model contains simple struct definitions shared across packages.
package model

import (
	"time"
)

// FileStatus describes the processing lifecycle. In Go a type declared via
// "type X string" creates a new named type with string as the underlying
// representation, enabling better type safety than using plain strings.
type FileStatus string

const (
	// const blocks group related symbolic names; each constant is strongly typed.
	StatusUploaded   FileStatus = "uploaded"
	StatusScanned    FileStatus = "scanned"
	StatusQueued     FileStatus = "queued"
	StatusProcessing FileStatus = "processing"
	StatusComplete   FileStatus = "complete"
	StatusRejected   FileStatus = "rejected"
	StatusFailed     FileStatus = "failed"
)

// FileRecord holds metadata about an uploaded file. Struct tags such as
// `json:"id"` instruct the encoding/json package to use custom field names when
// marshalling/unmarshalling.
type FileRecord struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Size        int64      `json:"size"`
	ContentType string     `json:"contentType"`
	// Path is omitted from JSON output because of the "-" struct tag.
	Path        string     `json:"-"`
	Status      FileStatus `json:"status"`
	// time.Time represents instants in UTC with nanosecond precision.
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	// omitempty instructs encoders to drop the field when empty.
	Message     string     `json:"message,omitempty"`
}
