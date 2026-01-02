package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FilesystemStorage implements storage.Reader for local filesystem
type FilesystemStorage struct {
	baseDir string
}

// NewFilesystemStorage creates a new filesystem storage reader
func NewFilesystemStorage(baseDir string) (*FilesystemStorage, error) {
	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &FilesystemStorage{
		baseDir: baseDir,
	}, nil
}

// GetReader returns a reader for the file at the given key
func (fs *FilesystemStorage) GetReader(ctx context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(fs.baseDir, key)

	// Security: prevent directory traversal
	if !filepath.HasPrefix(filepath.Clean(path), filepath.Clean(fs.baseDir)) {
		return nil, fmt.Errorf("invalid key: path traversal detected")
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", key)
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return file, nil
}

// Exists checks if a file exists at the given key
func (fs *FilesystemStorage) Exists(ctx context.Context, key string) (bool, error) {
	path := filepath.Join(fs.baseDir, key)

	// Security: prevent directory traversal
	if !filepath.HasPrefix(filepath.Clean(path), filepath.Clean(fs.baseDir)) {
		return false, fmt.Errorf("invalid key: path traversal detected")
	}

	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat file: %w", err)
	}

	return true, nil
}

// GetMetadata returns metadata for the file at the given key
func (fs *FilesystemStorage) GetMetadata(ctx context.Context, key string) (*Metadata, error) {
	path := filepath.Join(fs.baseDir, key)

	// Security: prevent directory traversal
	if !filepath.HasPrefix(filepath.Clean(path), filepath.Clean(fs.baseDir)) {
		return nil, fmt.Errorf("invalid key: path traversal detected")
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", key)
		}
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	return &Metadata{
		Size: info.Size(),
		// ContentType detection would require reading file headers
		// Can be added later if needed
	}, nil
}
