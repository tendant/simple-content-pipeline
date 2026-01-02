package storage

import (
	"context"
	"io"
)

// Reader provides read access to stored content
type Reader interface {
	// GetReader returns a reader for the content at the given key
	GetReader(ctx context.Context, key string) (io.ReadCloser, error)

	// Exists checks if content exists at the given key
	Exists(ctx context.Context, key string) (bool, error)
}

// Metadata contains storage object metadata
type Metadata struct {
	Size        int64
	ContentType string
	ETag        string
}

// ReaderWithMetadata provides read access with metadata
type ReaderWithMetadata interface {
	Reader

	// GetMetadata returns metadata for content at the given key
	GetMetadata(ctx context.Context, key string) (*Metadata, error)
}
