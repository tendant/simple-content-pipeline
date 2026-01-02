package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/tendant/simple-content/pkg/simplecontent"
)

// ContentReader provides read access to content via simple-content service
type ContentReader struct {
	service simplecontent.Service
}

// NewContentReader creates a new content reader using simple-content service
func NewContentReader(service simplecontent.Service) *ContentReader {
	return &ContentReader{
		service: service,
	}
}

// GetReaderByContentID returns a reader for content by content ID
func (cr *ContentReader) GetReaderByContentID(ctx context.Context, contentID string) (io.ReadCloser, error) {
	// Parse content ID
	id, err := uuid.Parse(contentID)
	if err != nil {
		return nil, fmt.Errorf("invalid content ID: %w", err)
	}

	// Download content from simple-content service
	reader, err := cr.service.DownloadContent(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to download content: %w", err)
	}

	return reader, nil
}

// GetReader returns a reader for content (implements storage.Reader interface)
// The key parameter is expected to be a content ID
func (cr *ContentReader) GetReader(ctx context.Context, key string) (io.ReadCloser, error) {
	return cr.GetReaderByContentID(ctx, key)
}

// Exists checks if content exists by content ID
func (cr *ContentReader) Exists(ctx context.Context, key string) (bool, error) {
	// Parse content ID
	id, err := uuid.Parse(key)
	if err != nil {
		return false, fmt.Errorf("invalid content ID: %w", err)
	}

	// Try to get content metadata
	_, err = cr.service.GetContent(ctx, id)
	if err != nil {
		// Check if it's a not found error
		// For now, treat any error as not exists
		return false, nil
	}

	return true, nil
}

// GetMetadata returns metadata for content
func (cr *ContentReader) GetMetadata(ctx context.Context, key string) (*Metadata, error) {
	// Parse content ID
	id, err := uuid.Parse(key)
	if err != nil {
		return nil, fmt.Errorf("invalid content ID: %w", err)
	}

	// Get content details
	details, err := cr.service.GetContentDetails(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get content details: %w", err)
	}

	return &Metadata{
		Size:        details.FileSize,
		ContentType: details.MimeType,
	}, nil
}
