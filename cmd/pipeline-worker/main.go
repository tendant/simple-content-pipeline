package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/tendant/simple-content-pipeline/internal/dbosruntime"
	"github.com/tendant/simple-content-pipeline/internal/handlers"
	"github.com/tendant/simple-content-pipeline/internal/storage"
	"github.com/tendant/simple-content-pipeline/internal/workflows"
	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
	"github.com/tendant/simple-content/pkg/simplecontent/presets"
)

func main() {
	// Load .env file if it exists (silently ignore if not found)
	_ = godotenv.Load()

	// Configuration from environment
	httpAddr := os.Getenv("WORKER_HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8081"
	}

	// Initialize content reader and derived writer
	// Use HTTP API if CONTENT_API_URL is set, otherwise use embedded service
	contentAPIURL := os.Getenv("CONTENT_API_URL")

	var contentReader interface {
		GetReaderByContentID(ctx context.Context, contentID string) (io.ReadCloser, error)
		Exists(ctx context.Context, key string) (bool, error)
	}
	var derivedWriter interface {
		HasDerived(ctx context.Context, contentID string, derivedType string, derivedVersion int) (bool, error)
		PutDerived(ctx context.Context, contentID string, derivedType string, derivedVersion int, r io.Reader, meta map[string]string) (string, error)
	}
	var cleanup func()

	if contentAPIURL != "" {
		log.Printf("Using simple-content HTTP API at: %s", contentAPIURL)
		contentReader = storage.NewHTTPContentReader(contentAPIURL)
		derivedWriter = storage.NewHTTPDerivedWriter(contentAPIURL)
		cleanup = func() {} // No cleanup needed for HTTP client
	} else {
		log.Printf("Using embedded simple-content service (development preset)")
		svc, cleanupFn, err := presets.NewDevelopment()
		if err != nil {
			log.Fatalf("Failed to initialize simple-content service: %v", err)
		}
		contentReader = storage.NewContentReader(svc)
		derivedWriter = storage.NewDerivedWriter(svc)
		cleanup = cleanupFn
	}
	defer cleanup()

	// Initialize DBOS runtime (required)
	dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
	if dbURL == "" {
		log.Fatalf("DBOS_SYSTEM_DATABASE_URL is required")
	}

	queueName := os.Getenv("DBOS_QUEUE_NAME")
	if queueName == "" {
		queueName = "default"
	}

	dbosRuntime, err := dbosruntime.NewRuntime(context.Background(), dbosruntime.Config{
		DatabaseURL: dbURL,
		AppName:     "pipeline-worker",
		QueueName:   queueName,
		Concurrency: 4, // TODO: read from env
	})
	if err != nil {
		log.Fatalf("Failed to initialize DBOS: %v", err)
	}

	// Initialize workflow runner with DBOS support (registers workflows with DBOS)
	workflowRunner := workflows.NewWorkflowRunner(dbosRuntime)

	// Register workflows
	thumbnailWorkflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
	workflowRunner.Register(pipeline.JobThumbnail, thumbnailWorkflow)
	log.Printf("✓ Registered workflow: %s for job: %s", thumbnailWorkflow.Name(), pipeline.JobThumbnail)

	// Launch DBOS (must be done after workflow registration)
	if err := dbosRuntime.Launch(); err != nil {
		log.Fatalf("Failed to launch DBOS: %v", err)
	}
	defer dbosRuntime.Shutdown(10 * time.Second)

	log.Printf("✓ DBOS runtime initialized")
	log.Printf("  Database: %s", dbURL)
	log.Printf("  Queue: %s", dbosRuntime.QueueName())
	log.Printf("  Concurrency: %d", dbosRuntime.Concurrency())

	// Create HTTP server
	mux := http.NewServeMux()

	// Create async handler (DBOS-only)
	asyncHandler := handlers.NewAsyncHandler(workflowRunner)

	// Register handlers
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/v1/process", asyncHandler.HandleProcessAsync)
	mux.HandleFunc("/v1/runs/", asyncHandler.HandleStatus)

	log.Printf("✓ Registered async endpoints")

	server := &http.Server{
		Addr:    httpAddr,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Pipeline worker starting on %s", httpAddr)
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
	})
}

// Handler holds dependencies for HTTP handlers
type Handler struct {
	workflowRunner *workflows.WorkflowRunner
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
