package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// HTTPDerivedWriter provides write access for derived content via simple-content HTTP API
type HTTPDerivedWriter struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPDerivedWriter creates a new HTTP-based derived content writer
func NewHTTPDerivedWriter(baseURL string) *HTTPDerivedWriter {
	return &HTTPDerivedWriter{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

// HasDerived checks if a derived output already exists for the given type/version
func (dw *HTTPDerivedWriter) HasDerived(ctx context.Context, contentID string, derivedType string, derivedVersion int) (bool, error) {
	// TODO: Query derived content via HTTP API
	// For now, always return false (regenerate)
	return false, nil
}

// PutDerived creates derived content via simple-content HTTP API
func (dw *HTTPDerivedWriter) PutDerived(ctx context.Context, contentID string, derivedType string, derivedVersion int, r io.Reader, meta map[string]string) (string, error) {
	// Create variant name from type and version
	variant := fmt.Sprintf("%s_v%d", derivedType, derivedVersion)

	fileName := meta["file_name"]
	if fileName == "" {
		fileName = fmt.Sprintf("derived_%s.dat", derivedType)
	}

	// Read content into buffer (required for HTTP upload)
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("failed to read content: %w", err)
	}

	// Create derived content request
	reqBody := map[string]interface{}{
		"parent_id":       contentID,
		"derivation_type": derivedType,
		"variant":         variant,
		"file_name":       fileName,
		"tags":            []string{derivedType, variant},
		"content_data":    string(data), // TODO: Handle binary properly
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/contents/%s/derived", dw.baseURL, contentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := dw.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create derived content: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create derived failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to get derived content ID
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract ID from response
	derivedID, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("no ID in response")
	}

	return derivedID, nil
}
