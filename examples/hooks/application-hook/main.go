package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/tendant/simple-content/pkg/simplecontent"
	"github.com/tendant/simple-content/pkg/simplecontent/presets"
	"github.com/tendant/simple-content-pipeline/internal/dbosruntime"
	"github.com/tendant/simple-content-pipeline/internal/storage"
	"github.com/tendant/simple-content-pipeline/internal/workflows"
	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// Example: Application with automatic workflow hooks
//
// This demonstrates:
// - Upload handler that automatically triggers workflows
// - Hook functions that run in background
// - Conditional workflow triggering based on content type

func main() {
	log.Println("=== Application Hook Example ===")
	log.Println()

	// Load configuration
	_ = godotenv.Load("../../../.env")

	// Initialize services
	app := mustInitialize()
	defer app.cleanup()

	// Start HTTP server
	srv := app.startServer()
	defer srv.Close()

	// Wait for shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
}

// Application holds all dependencies
type Application struct {
	contentService simplecontent.Service
	pipelineRunner *workflows.WorkflowRunner
	dbosRuntime    *dbosruntime.Runtime
	cleanup        func()
}

func mustInitialize() *Application {
	log.Println("Initializing application...")

	// Initialize simple-content service (embedded)
	svc, cleanup, err := presets.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to init content service: %v", err)
	}

	// Initialize DBOS runtime
	dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DBOS_SYSTEM_DATABASE_URL required")
	}

	dbosRuntime, err := dbosruntime.NewRuntime(context.Background(), dbosruntime.Config{
		DatabaseURL: dbURL,
		AppName:     "app-with-hooks",
		QueueName:   "default",
		Concurrency: 4,
	})
	if err != nil {
		cleanup()
		log.Fatalf("Failed to init DBOS: %v", err)
	}

	// Initialize pipeline
	runner := workflows.NewWorkflowRunner(dbosRuntime)

	contentReader := storage.NewContentReader(svc)
	derivedWriter := storage.NewDerivedWriter(svc)

	thumbnailWorkflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
	runner.Register(pipeline.JobThumbnail, thumbnailWorkflow)

	// Launch DBOS
	if err := dbosRuntime.Launch(); err != nil {
		cleanup()
		log.Fatalf("Failed to launch DBOS: %v", err)
	}

	log.Println("✓ Application initialized")
	log.Println("✓ Hooks enabled: auto-trigger thumbnails for images")
	log.Println()

	return &Application{
		contentService: svc,
		pipelineRunner: runner,
		dbosRuntime:    dbosRuntime,
		cleanup: func() {
			dbosRuntime.Shutdown(10 * time.Second)
			cleanup()
		},
	}
}

func (app *Application) startServer() *http.Server {
	mux := http.NewServeMux()

	// Upload endpoint with hooks
	mux.HandleFunc("/api/upload", app.handleUpload)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		log.Println("✓ Server ready on http://localhost:8080")
		log.Println()
		log.Println("Example upload:")
		log.Println("  curl -X POST http://localhost:8080/api/upload \\")
		log.Println("    -F 'file=@image.jpg' \\")
		log.Println("    -F 'name=My Image'")
		log.Println()

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	return srv
}

// handleUpload uploads content and triggers workflows via hooks
func (app *Application) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Parse multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	name := r.FormValue("name")
	if name == "" {
		name = header.Filename
	}

	// Read file data
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	// Detect content type
	contentType := http.DetectContentType(data)

	log.Printf("Uploading: %s (%s, %d bytes)", name, contentType, len(data))

	// Upload to simple-content
	content, err := app.contentService.UploadContent(ctx, simplecontent.UploadContentRequest{
		OwnerID:      uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		TenantID:     uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		Name:         name,
		DocumentType: contentType,
		Reader:       bytes.NewReader(data),
		FileName:     header.Filename,
		Tags:         []string{"uploaded", "via-api"},
	})
	if err != nil {
		log.Printf("Upload failed: %v", err)
		http.Error(w, fmt.Sprintf("Upload failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("✓ Content uploaded: %s", content.ID)

	// 🎣 HOOK: Automatically trigger workflows based on content type
	go app.triggerWorkflowHooks(content.ID.String(), contentType)

	// Return response immediately (workflows run in background)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"content_id":    content.ID.String(),
		"status":        "uploaded",
		"document_type": contentType,
		"size":          len(data),
		"message":       "Content uploaded, processing workflows triggered",
	})
}

// triggerWorkflowHooks is the hook function - runs in background
// This is where you define the automatic workflow triggering logic
func (app *Application) triggerWorkflowHooks(contentID, contentType string) {
	log.Printf("🎣 HOOK: Checking workflows for content_id=%s, type=%s", contentID, contentType)

	// Hook 1: Trigger thumbnail for images
	if isImage(contentType) {
		log.Printf("🎣 HOOK: Image detected, triggering thumbnail workflow")

		runID, err := app.pipelineRunner.RunAsync(context.Background(), pipeline.ProcessRequest{
			ContentID: contentID,
			Job:       pipeline.JobThumbnail,
			Versions: map[string]int{
				pipeline.DerivedTypeThumbnail: 1,
			},
			Metadata: map[string]string{
				"mime":   contentType,
				"source": "upload-hook",
			},
		})
		if err != nil {
			log.Printf("🎣 HOOK ERROR: Failed to trigger thumbnail: %v", err)
			return
		}

		log.Printf("🎣 HOOK SUCCESS: Thumbnail workflow queued (run_id=%s)", runID)
	}

	// Hook 2: Trigger OCR for documents (future)
	if isDocument(contentType) {
		log.Printf("🎣 HOOK: Document detected (OCR workflow not implemented yet)")
		// runID, _ := app.pipelineRunner.RunAsync(ctx, ocrRequest)
	}

	// Hook 3: Custom processing based on file type
	if isPDF(contentType) {
		log.Printf("🎣 HOOK: PDF detected (custom workflow not implemented yet)")
	}

	// You can add more hooks here based on your business logic
}

// Content type helpers
func isImage(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}

func isDocument(contentType string) bool {
	return strings.Contains(contentType, "pdf") ||
		strings.Contains(contentType, "word") ||
		strings.Contains(contentType, "document")
}

func isPDF(contentType string) bool {
	return strings.Contains(contentType, "pdf")
}
