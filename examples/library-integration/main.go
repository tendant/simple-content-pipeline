package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/tendant/simple-content-pipeline/internal/dbosruntime"
	"github.com/tendant/simple-content-pipeline/internal/storage"
	"github.com/tendant/simple-content-pipeline/internal/workflows"
	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// Example: Using simple-content-pipeline as a library in your application
//
// This demonstrates how to:
// 1. Initialize the pipeline in your own application
// 2. Trigger workflows programmatically
// 3. Handle graceful shutdown

func main() {
	log.Println("=== Pipeline Library Integration Example ===")
	log.Println()

	// Load configuration from .env file
	if err := godotenv.Load("../../.env"); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Initialize pipeline
	pipelineRunner := mustInitializePipeline()
	defer shutdownPipeline(pipelineRunner)

	log.Println("✓ Pipeline ready")
	log.Println()

	// Example usage: Process some content
	runExample(pipelineRunner)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	log.Println("Press Ctrl+C to shutdown...")
	<-quit
}

// PipelineRunner wraps the pipeline components
type PipelineRunner struct {
	runtime *dbosruntime.Runtime
	runner  *workflows.WorkflowRunner
}

// mustInitializePipeline sets up the pipeline or exits on error
func mustInitializePipeline() *PipelineRunner {
	log.Println("Initializing pipeline...")

	// Get DBOS configuration
	dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DBOS_SYSTEM_DATABASE_URL environment variable is required")
	}

	queueName := os.Getenv("DBOS_QUEUE_NAME")
	if queueName == "" {
		queueName = "default"
	}

	// Create DBOS runtime
	dbosRuntime, err := dbosruntime.NewRuntime(context.Background(), dbosruntime.Config{
		DatabaseURL: dbURL,
		AppName:     "example-app",
		QueueName:   queueName,
		Concurrency: 4,
	})
	if err != nil {
		log.Fatalf("Failed to initialize DBOS: %v", err)
	}

	// Create workflow runner
	runner := workflows.NewWorkflowRunner(dbosRuntime)

	// Setup storage adapters (connect to simple-content API)
	contentAPIURL := os.Getenv("CONTENT_API_URL")
	if contentAPIURL == "" {
		contentAPIURL = "http://localhost:4000"
	}

	log.Printf("  Content API: %s", contentAPIURL)

	contentReader := storage.NewHTTPContentReader(contentAPIURL)
	derivedWriter := storage.NewHTTPDerivedWriter(contentAPIURL)

	// Register workflows BEFORE launching DBOS
	thumbnailWorkflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
	runner.Register(pipeline.JobThumbnail, thumbnailWorkflow)

	log.Printf("  ✓ Registered workflow: %s", thumbnailWorkflow.Name())

	// Launch DBOS (must be after workflow registration)
	if err := dbosRuntime.Launch(); err != nil {
		log.Fatalf("Failed to launch DBOS: %v", err)
	}

	log.Printf("  ✓ DBOS launched")
	log.Printf("    Database: %s", dbURL)
	log.Printf("    Queue: %s", queueName)

	return &PipelineRunner{
		runtime: dbosRuntime,
		runner:  runner,
	}
}

// ProcessContent triggers thumbnail generation for a content ID
func (p *PipelineRunner) ProcessContent(contentID string) (string, error) {
	log.Printf("Processing content: %s", contentID)

	runID, err := p.runner.RunAsync(context.Background(), pipeline.ProcessRequest{
		ContentID: contentID,
		Job:       pipeline.JobThumbnail,
		Versions: map[string]int{
			pipeline.DerivedTypeThumbnail: 1,
		},
		Metadata: map[string]string{
			"mime": "image/jpeg",
		},
	})
	if err != nil {
		return "", err
	}

	log.Printf("  ✓ Workflow enqueued: %s", runID)
	return runID, nil
}

// GetWorkflowStatus retrieves the status of a workflow execution
func (p *PipelineRunner) GetWorkflowStatus(runID string) (*workflows.WorkflowStatus, error) {
	return p.runner.GetStatus(context.Background(), runID)
}

// shutdownPipeline gracefully stops the pipeline
func shutdownPipeline(p *PipelineRunner) {
	log.Println()
	log.Println("Shutting down pipeline...")
	p.runtime.Shutdown(10 * time.Second)
	log.Println("✓ Pipeline stopped")
}

// runExample demonstrates how to use the pipeline in your application
func runExample(p *PipelineRunner) {
	log.Println("Example: Triggering thumbnail generation")
	log.Println()

	// Example 1: Process a content ID
	// In a real application, this would be a content ID from your upload handler
	exampleContentID := "123e4567-e89b-12d3-a456-426614174000"

	runID, err := p.ProcessContent(exampleContentID)
	if err != nil {
		log.Printf("  ✗ Failed to process content: %v", err)
		log.Println()
		log.Println("Note: This example expects a running simple-content server.")
		log.Println("To run the full test:")
		log.Println("  1. Start simple-content: cd ../simple-content && ./standalone-server")
		log.Println("  2. Upload a file to get a content_id")
		log.Println("  3. Run this example with that content_id")
		return
	}

	log.Printf("  Run ID: %s", runID)
	log.Println()

	// Example 2: Check workflow status
	log.Println("Example: Checking workflow status")
	time.Sleep(1 * time.Second) // Give it a moment

	status, err := p.GetWorkflowStatus(runID)
	if err != nil {
		log.Printf("  ✗ Failed to get status: %v", err)
	} else {
		log.Printf("  State: %s", status.State)
		log.Printf("  Run ID: %s", status.RunID)
	}
	log.Println()

	// Example 3: Process multiple items
	log.Println("Example: Batch processing")
	contentIDs := []string{
		"223e4567-e89b-12d3-a456-426614174001",
		"323e4567-e89b-12d3-a456-426614174002",
	}

	for _, contentID := range contentIDs {
		runID, err := p.ProcessContent(contentID)
		if err != nil {
			log.Printf("  ✗ Failed %s: %v", contentID, err)
		} else {
			log.Printf("  ✓ Queued %s -> %s", contentID, runID)
		}
	}
	log.Println()

	log.Println("All workflows are executing in the background via DBOS.")
	log.Println("They will complete even if this process crashes and restarts.")
	log.Println()
}
