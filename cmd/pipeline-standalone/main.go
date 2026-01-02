package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/tendant/simple-content-pipeline/internal/storage"
	"github.com/tendant/simple-content-pipeline/internal/workflows"
	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// Standalone pipeline worker for quick testing
// Connects to simple-content standalone-server via HTTP
// Requires: simple-content/cmd/standalone-server running
func main() {
	// Command-line flags
	portFlag := flag.String("port", "", "HTTP port (default: 8080)")
	contentAPIFlag := flag.String("content-api", "", "simple-content API URL (default: http://localhost:4000)")
	flag.Parse()

	// Configuration priority: CLI args > environment variables > defaults
	httpAddr := *portFlag
	if httpAddr == "" {
		httpAddr = os.Getenv("PIPELINE_HTTP_ADDR")
	}
	if httpAddr == "" {
		httpAddr = ":8080"
	}
	// Ensure address has colon prefix
	if httpAddr[0] != ':' {
		httpAddr = ":" + httpAddr
	}

	contentAPIURL := *contentAPIFlag
	if contentAPIURL == "" {
		contentAPIURL = os.Getenv("CONTENT_API_URL")
	}
	if contentAPIURL == "" {
		contentAPIURL = "http://localhost:4000"
	}

	log.Printf("Pipeline Standalone Worker")
	log.Printf("  Mode: HTTP client to simple-content standalone-server")
	log.Printf("  Content API: %s", contentAPIURL)
	log.Printf("  HTTP address: %s", httpAddr)
	log.Printf("")
	log.Printf("Prerequisites:")
	log.Printf("  1. Start simple-content server:")
	log.Printf("     cd ../simple-content && go run ./cmd/standalone-server")
	log.Printf("  2. Verify content API is accessible:")
	log.Printf("     curl %s/health", contentAPIURL)
	log.Printf("")

	// Initialize content reader and derived writer via HTTP
	contentReader := storage.NewHTTPContentReader(contentAPIURL)
	derivedWriter := storage.NewHTTPDerivedWriter(contentAPIURL)

	log.Printf("✓ HTTP client initialized")

	// Initialize workflow runner
	workflowRunner := workflows.NewWorkflowRunner()

	// Register workflows
	thumbnailWorkflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
	workflowRunner.Register(pipeline.JobThumbnail, thumbnailWorkflow)
	log.Printf("✓ Registered workflow: %s for job: %s", thumbnailWorkflow.Name(), pipeline.JobThumbnail)

	// Create HTTP server
	mux := http.NewServeMux()

	// Create handler with dependencies
	handler := &Handler{
		workflowRunner: workflowRunner,
		contentAPIURL:  contentAPIURL,
	}

	// Register handlers
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/v1/process", handler.handleProcess)
	mux.HandleFunc("/v1/test", handler.handleTest) // Simple test endpoint

	server := &http.Server{
		Addr:    httpAddr,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		// Extract port number for display (remove leading colon)
		displayPort := httpAddr
		if displayPort[0] == ':' {
			displayPort = displayPort[1:]
		}

		log.Printf("✓ Pipeline worker ready on %s", httpAddr)
		log.Printf("")
		log.Printf("Quick test (after starting simple-content server):")
		log.Printf("  curl http://localhost:%s/v1/test", displayPort)
		log.Printf("")
		log.Printf("Available endpoints:")
		log.Printf("  GET  /health           - Health check")
		log.Printf("  POST /v1/process       - Process content (requires existing content_id)")
		log.Printf("  GET  /v1/test          - Run end-to-end test (upload + process + verify)")
		log.Printf("  POST /v1/test          - Run end-to-end test")
		log.Printf("")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

// handleHealth returns health status
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"mode":   "standalone",
	})
}

// Handler holds dependencies for HTTP handlers
type Handler struct {
	workflowRunner *workflows.WorkflowRunner
	contentAPIURL  string
}

// handleProcess handles the /v1/process endpoint
func (h *Handler) handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req pipeline.ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.ContentID == "" {
		http.Error(w, "content_id is required", http.StatusBadRequest)
		return
	}
	if req.Job == "" {
		http.Error(w, "job is required", http.StatusBadRequest)
		return
	}

	log.Printf("Processing request: content_id=%s, job=%s, object_key=%s", req.ContentID, req.Job, req.ObjectKey)

	// Generate run ID
	runID := uuid.New().String()

	// Create workflow context
	wctx := &workflows.WorkflowContext{
		Ctx:     r.Context(),
		Request: req,
		RunID:   runID,
	}

	// Execute workflow
	result, err := h.workflowRunner.Run(wctx)
	if err != nil {
		log.Printf("[%s] Workflow execution failed: %v", runID, err)
		http.Error(w, fmt.Sprintf("Workflow execution failed: %v", err), http.StatusInternalServerError)
		return
	}

	if !result.Success {
		log.Printf("[%s] Workflow completed with errors: %v", runID, result.Error)
		http.Error(w, fmt.Sprintf("Workflow failed: %v", result.Error), http.StatusInternalServerError)
		return
	}

	log.Printf("[%s] Workflow completed successfully", runID)

	// Return response
	resp := pipeline.ProcessResponse{
		RunID:           runID,
		DedupeSeenCount: 0, // TODO: Track dedupe count
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleTest handles the /v1/test endpoint for quick end-to-end testing
func (h *Handler) handleTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed (use GET or POST)", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	log.Println("=== Running End-to-End Test ===")

	// Step 1: Upload test content via simple-content HTTP API
	log.Println("Step 1: Uploading test content to simple-content...")
	testData := []byte("This is a test image file for thumbnail generation")

	// Create upload request
	uploadReq := map[string]interface{}{
		"owner_id":      "00000000-0000-0000-0000-000000000001",
		"tenant_id":     "00000000-0000-0000-0000-000000000002",
		"name":          "Test Image",
		"document_type": "image/jpeg",
		"file_name":     "test-image.jpg",
		"tags":          []string{"test", "image"},
		"content_data":  string(testData),
	}

	jsonData, err := json.Marshal(uploadReq)
	if err != nil {
		log.Printf("Failed to marshal upload request: %v", err)
		http.Error(w, fmt.Sprintf("Marshal failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Call simple-content API to upload
	uploadURL := fmt.Sprintf("%s/api/v1/contents", h.contentAPIURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(jsonData))
	if err != nil {
		log.Printf("Failed to create upload request: %v", err)
		http.Error(w, fmt.Sprintf("Request creation failed: %v", err), http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("Failed to upload content: %v", err)
		http.Error(w, fmt.Sprintf("Upload failed: %v. Is simple-content server running at %s?", err, h.contentAPIURL), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
		http.Error(w, fmt.Sprintf("Upload failed with status %d", resp.StatusCode), http.StatusInternalServerError)
		return
	}

	var content map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		log.Printf("Failed to decode upload response: %v", err)
		http.Error(w, fmt.Sprintf("Decode failed: %v", err), http.StatusInternalServerError)
		return
	}

	contentID := content["id"].(string)
	contentStatus := content["status"].(string)
	log.Printf("✓ Content uploaded: %s (status: %s)", contentID, contentStatus)

	// Step 2: Trigger thumbnail generation
	log.Println("Step 2: Triggering thumbnail generation...")
	runID := uuid.New().String()

	wctx := &workflows.WorkflowContext{
		Ctx: ctx,
		Request: pipeline.ProcessRequest{
			ContentID: contentID,
			Job:       pipeline.JobThumbnail,
			Versions: map[string]int{
				pipeline.DerivedTypeThumbnail: 1,
			},
			Metadata: map[string]string{
				"mime": "image/jpeg",
			},
		},
		RunID: runID,
	}

	result, err := h.workflowRunner.Run(wctx)
	if err != nil {
		log.Printf("Workflow execution failed: %v", err)
		http.Error(w, fmt.Sprintf("Workflow failed: %v", err), http.StatusInternalServerError)
		return
	}

	if !result.Success {
		log.Printf("Workflow completed with errors: %v", result.Error)
		http.Error(w, fmt.Sprintf("Workflow failed: %v", result.Error), http.StatusInternalServerError)
		return
	}

	log.Printf("✓ Workflow completed successfully (run_id: %s)", runID)

	// Step 3: List derived content via simple-content HTTP API
	log.Println("Step 3: Checking derived content...")

	listURL := fmt.Sprintf("%s/api/v1/contents/%s/derived", h.contentAPIURL, contentID)
	listReq, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		log.Printf("Failed to create list request: %v", err)
		http.Error(w, fmt.Sprintf("List request failed: %v", err), http.StatusInternalServerError)
		return
	}

	listResp, err := client.Do(listReq)
	if err != nil {
		log.Printf("Failed to list derived content: %v", err)
		http.Error(w, fmt.Sprintf("List failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer listResp.Body.Close()

	if listResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(listResp.Body)
		log.Printf("List failed with status %d: %s", listResp.StatusCode, string(bodyBytes))
		http.Error(w, fmt.Sprintf("List failed with status %d", listResp.StatusCode), http.StatusInternalServerError)
		return
	}

	var derived []map[string]interface{}
	if err := json.NewDecoder(listResp.Body).Decode(&derived); err != nil {
		log.Printf("Failed to decode list response: %v", err)
		http.Error(w, fmt.Sprintf("Decode failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("✓ Found %d derived content(s)", len(derived))
	for _, d := range derived {
		derivationType := d["derivation_type"]
		variant := d["variant"]
		status := d["status"]
		log.Printf("  - Type: %v, Variant: %v, Status: %v", derivationType, variant, status)
	}

	log.Println("=== Test Complete ===")

	// Return test results
	response := map[string]interface{}{
		"test_status":      "success",
		"content_id":       contentID,
		"run_id":           runID,
		"derived_count":    len(derived),
		"derived_contents": derived,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
