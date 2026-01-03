package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/tendant/simple-content-pipeline/internal/dbosruntime"
	"github.com/tendant/simple-content-pipeline/internal/storage"
	"github.com/tendant/simple-content-pipeline/internal/workflows"
	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// Config holds the configuration for initializing the pipeline runner
type Config struct {
	DatabaseURL        string // DBOS PostgreSQL connection string
	AppName            string // Application name for DBOS
	QueueName          string // DBOS queue name
	Concurrency        int    // Number of concurrent workers
	ContentAPIURL      string // URL of the content API server
	ApplicationVersion string // Optional: Override binary hash for version matching
}

// Runner provides a high-level API for running pipeline workflows via DBOS
type Runner struct {
	runtime *dbosruntime.Runtime
	runner  *workflows.WorkflowRunner
}

// New creates and initializes a new pipeline runner with DBOS integration
func New(cfg Config) (*Runner, error) {
	// Create DBOS runtime
	dbosRuntime, err := dbosruntime.NewRuntime(context.Background(), dbosruntime.Config{
		DatabaseURL:        cfg.DatabaseURL,
		AppName:            cfg.AppName,
		QueueName:          cfg.QueueName,
		Concurrency:        cfg.Concurrency,
		ApplicationVersion: cfg.ApplicationVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DBOS: %w", err)
	}

	// Create workflow runner
	workflowRunner := workflows.NewWorkflowRunner(dbosRuntime)

	// Setup storage adapters
	contentReader := storage.NewHTTPContentReader(cfg.ContentAPIURL)
	derivedWriter := storage.NewHTTPDerivedWriter(cfg.ContentAPIURL)

	// Register thumbnail workflow
	thumbnailWorkflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
	workflowRunner.Register(pipeline.JobThumbnail, thumbnailWorkflow)

	// Launch DBOS (must be after workflow registration)
	if err := dbosRuntime.Launch(); err != nil {
		return nil, fmt.Errorf("failed to launch DBOS: %w", err)
	}

	return &Runner{
		runtime: dbosRuntime,
		runner:  workflowRunner,
	}, nil
}

// RunThumbnail triggers a thumbnail generation workflow
func (r *Runner) RunThumbnail(ctx context.Context, contentID string, width, height int) (string, error) {
	return r.runner.RunAsync(ctx, pipeline.ProcessRequest{
		ContentID: contentID,
		Job:       pipeline.JobThumbnail,
		Versions: map[string]int{
			pipeline.DerivedTypeThumbnail: 1,
		},
		Metadata: map[string]string{
			"width":  fmt.Sprintf("%d", width),
			"height": fmt.Sprintf("%d", height),
		},
	})
}

// RunObjectDetection triggers an object detection workflow
func (r *Runner) RunObjectDetection(ctx context.Context, contentID string) (string, error) {
	// Start workflow by name (language-agnostic)
	fmt.Println("DEBUG: Using StartWorkflowByName for detect_objects_workflow")
	return r.runtime.StartWorkflowByName(ctx, "detect_objects_workflow", contentID, nil)
}

// RunOCR triggers an OCR workflow
func (r *Runner) RunOCR(ctx context.Context, contentID string) (string, error) {
	// Start workflow by name (language-agnostic)
	return r.runtime.StartWorkflowByName(ctx, "ocr_workflow", contentID, nil)
}

// Shutdown gracefully shuts down the pipeline runner
func (r *Runner) Shutdown(timeoutSeconds int) {
	if r.runtime != nil {
		r.runtime.Shutdown(time.Duration(timeoutSeconds) * time.Second)
	}
}
