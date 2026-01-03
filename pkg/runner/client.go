package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/tendant/simple-content-pipeline/internal/dbosruntime"
	"github.com/tendant/simple-content-pipeline/internal/workflows"
	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// Client provides a client-only API for starting workflows without executing them
// Use this in applications that want to enqueue workflows for workers to execute
type Client struct {
	runtime *dbosruntime.Runtime
	runner  *workflows.WorkflowRunner
}

// NewClient creates a client that can start workflows but doesn't execute them
// Workers must be running separately to execute the enqueued workflows
func NewClient(cfg Config) (*Client, error) {
	// Create DBOS runtime
	dbosRuntime, err := dbosruntime.NewRuntime(context.Background(), dbosruntime.Config{
		DatabaseURL: cfg.DatabaseURL,
		AppName:     cfg.AppName,
		QueueName:   cfg.QueueName,
		Concurrency: 0, // Client mode: don't process workflows
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DBOS: %w", err)
	}

	// Create workflow runner (for enqueueing only, no registration)
	workflowRunner := workflows.NewWorkflowRunner(dbosRuntime)

	// Launch DBOS (no workflows registered, client mode)
	if err := dbosRuntime.Launch(); err != nil {
		return nil, fmt.Errorf("failed to launch DBOS: %w", err)
	}

	return &Client{
		runtime: dbosRuntime,
		runner:  workflowRunner,
	}, nil
}

// RunThumbnail enqueues a thumbnail generation workflow for workers to execute
func (c *Client) RunThumbnail(ctx context.Context, contentID string, width, height int) (string, error) {
	return c.runner.RunAsync(ctx, pipeline.ProcessRequest{
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

// RunObjectDetection enqueues an object detection workflow for workers to execute
func (c *Client) RunObjectDetection(ctx context.Context, contentID string) (string, error) {
	return c.runner.RunAsync(ctx, pipeline.ProcessRequest{
		ContentID: contentID,
		Job:       "object_detection",
		Versions: map[string]int{
			"object_detection": 1,
		},
	})
}

// Shutdown gracefully shuts down the client
func (c *Client) Shutdown(timeoutSeconds int) {
	if c.runtime != nil {
		c.runtime.Shutdown(time.Duration(timeoutSeconds) * time.Second)
	}
}
