package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
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
	// Construct the variant we're looking for
	variant := fmt.Sprintf("%s_v%d", derivedType, derivedVersion)

	// Query derived content from simple-content API
	url := fmt.Sprintf("%s/api/v1/contents/%s/derived", dw.baseURL, contentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := dw.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to query derived content: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// No derived content exists
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("query derived failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	var derivedList []struct {
		Variant string `json:"variant"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(bodyBytes, &derivedList); err != nil {
		return false, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check if any derived content matches our variant and is ready
	for _, derived := range derivedList {
		if derived.Variant == variant && (derived.Status == "uploaded" || derived.Status == "processed") {
			return true, nil
		}
	}

	return false, nil
}

// PutDerived creates derived content via simple-content HTTP API using multipart upload
func (dw *HTTPDerivedWriter) PutDerived(ctx context.Context, contentID string, derivedType string, derivedVersion int, r io.Reader, meta map[string]string) (string, error) {
	// Create variant name from type and version
	variant := fmt.Sprintf("%s_v%d", derivedType, derivedVersion)

	fileName := meta["file_name"]
	if fileName == "" {
		fileName = fmt.Sprintf("derived_%s.dat", derivedType)
	}

	// Read content into buffer
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("failed to read content: %w", err)
	}

	// Create multipart form with file and metadata
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add derivation metadata
	writer.WriteField("derivation_type", derivedType)
	writer.WriteField("variant", variant)

	// Add file
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("failed to write file data: %w", err)
	}

	// Close the multipart writer
	contentType := writer.FormDataContentType()
	writer.Close()

	// POST to derived content endpoint with multipart data
	url := fmt.Sprintf("%s/api/v1/contents/%s/derived", dw.baseURL, contentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

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
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract ID from response
	derivedID, ok := result["id"].(string)
	if !ok {
		// Try nested in "data" field
		if dataMap, dataOk := result["data"].(map[string]interface{}); dataOk {
			derivedID, ok = dataMap["id"].(string)
		}
	}

	if !ok || derivedID == "" {
		return "", fmt.Errorf("no ID in response")
	}

	return derivedID, nil
}
