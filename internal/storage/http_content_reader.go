package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// HTTPContentReader provides read access to content via simple-content HTTP API
type HTTPContentReader struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPContentReader creates a new HTTP-based content reader
func NewHTTPContentReader(baseURL string) *HTTPContentReader {
	return &HTTPContentReader{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

// GetReaderByContentID returns a reader for content by content ID via HTTP API
func (cr *HTTPContentReader) GetReaderByContentID(ctx context.Context, contentID string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/api/v1/contents/%s/download", cr.baseURL, contentID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := cr.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download content: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// GetReader returns a reader for content (implements storage.Reader interface)
func (cr *HTTPContentReader) GetReader(ctx context.Context, key string) (io.ReadCloser, error) {
	return cr.GetReaderByContentID(ctx, key)
}

// Exists checks if content exists by content ID via HTTP API
func (cr *HTTPContentReader) Exists(ctx context.Context, key string) (bool, error) {
	url := fmt.Sprintf("%s/api/v1/contents/%s", cr.baseURL, key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := cr.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to check content: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}

// GetMetadata returns metadata for content via HTTP API
func (cr *HTTPContentReader) GetMetadata(ctx context.Context, key string) (*Metadata, error) {
	// TODO: Implement using /api/v1/contents/{id}/details endpoint
	return &Metadata{}, nil
}
