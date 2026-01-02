package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/tendant/simple-content/pkg/simplecontent"
	"github.com/tendant/simple-content/pkg/simplecontent/presets"
	"github.com/tendant/simple-content-pipeline/internal/storage"
	"github.com/tendant/simple-content-pipeline/internal/workflows"
	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// Standalone pipeline worker for quick testing
// Uses in-memory repository + filesystem storage (./dev-data)
// No external simple-content server needed
func main() {
	// Configuration from environment
	httpAddr := os.Getenv("PIPELINE_HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	storageDir := os.Getenv("STORAGE_DIR")
	if storageDir == "" {
		storageDir = "./dev-data"
	}

	log.Printf("Pipeline Standalone Worker")
	log.Printf("  Mode: Embedded (in-memory DB + filesystem storage)")
	log.Printf("  Storage directory: %s", storageDir)
	log.Printf("  HTTP address: %s", httpAddr)

	// Initialize simple-content service with development preset
	// This gives us: in-memory repository + filesystem storage
	svc, cleanup, err := presets.NewDevelopment(
		presets.WithDevStorage(storageDir),
	)
	if err != nil {
		log.Fatalf("Failed to initialize simple-content service: %v", err)
	}
	defer cleanup()

	log.Printf("✓ simple-content service initialized")

	// Initialize content reader and derived writer
	contentReader := storage.NewContentReader(svc)
	derivedWriter := storage.NewDerivedWriter(svc)

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
		service:        svc,
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
		log.Printf("✓ Pipeline worker ready on %s", httpAddr)
		log.Printf("")
		log.Printf("Quick test:")
		log.Printf("  curl http://localhost:8080/v1/test")
		log.Printf("")
		log.Printf("Available endpoints:")
		log.Printf("  GET  /health           - Health check")
		log.Printf("  POST /v1/process       - Process content (requires existing content_id)")
		log.Printf("  GET  /v1/test          - Run end-to-end test (upload + process + verify)")
		log.Printf("  POST /v1/test          - Run end-to-end test")
		log.Printf("")
		log.Printf("For production-like testing with separate processes:")
		log.Printf("  Terminal 1: cd ../simple-content && go run ./cmd/server-configured")
		log.Printf("  Terminal 2: CONTENT_API_URL=http://localhost:4000 go run ./cmd/pipeline-worker")
		log.Printf("  Terminal 3: go run ./examples/trigger/main.go")
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
	service        simplecontent.Service
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

	// Step 1: Upload test content
	log.Println("Step 1: Uploading test content...")
	testData := []byte("This is a test image file for thumbnail generation")

	content, err := h.service.UploadContent(ctx, simplecontent.UploadContentRequest{
		OwnerID:      uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		TenantID:     uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		Name:         "Test Image",
		DocumentType: "image/jpeg",
		Reader:       bytes.NewReader(testData),
		FileName:     "test-image.jpg",
		Tags:         []string{"test", "image"},
	})
	if err != nil {
		log.Printf("Failed to upload content: %v", err)
		http.Error(w, fmt.Sprintf("Upload failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("✓ Content uploaded: %s (status: %s)", content.ID, content.Status)

	// Step 2: Trigger thumbnail generation
	log.Println("Step 2: Triggering thumbnail generation...")
	runID := uuid.New().String()

	wctx := &workflows.WorkflowContext{
		Ctx: ctx,
		Request: pipeline.ProcessRequest{
			ContentID: content.ID.String(),
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

	// Step 3: List derived content
	log.Println("Step 3: Checking derived content...")
	derived, err := h.service.ListDerivedContent(ctx, simplecontent.WithParentID(content.ID))
	if err != nil {
		log.Printf("Failed to list derived content: %v", err)
		http.Error(w, fmt.Sprintf("List derived failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("✓ Found %d derived content(s)", len(derived))
	for _, d := range derived {
		log.Printf("  - Type: %s, Variant: %s, Status: %s", d.DerivationType, d.Variant, d.Status)
	}

	log.Println("=== Test Complete ===")

	// Return test results
	response := map[string]interface{}{
		"test_status":      "success",
		"content_id":       content.ID.String(),
		"run_id":           runID,
		"derived_count":    len(derived),
		"derived_contents": derived,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
