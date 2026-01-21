package executors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	simpleworkflow "github.com/tendant/simple-workflow"
	"github.com/tendant/simple-content-pipeline/internal/workflows"
	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// ThumbnailExecutor implements simpleworkflow.WorkflowExecutor for thumbnail generation
type ThumbnailExecutor struct {
	contentReader workflows.ContentReader
	derivedWriter workflows.DerivedWriter
}

// NewThumbnailExecutor creates a new thumbnail executor
func NewThumbnailExecutor(
	contentReader workflows.ContentReader,
	derivedWriter workflows.DerivedWriter,
) *ThumbnailExecutor {
	return &ThumbnailExecutor{
		contentReader: contentReader,
		derivedWriter: derivedWriter,
	}
}

// Execute implements simpleworkflow.WorkflowExecutor
func (e *ThumbnailExecutor) Execute(ctx context.Context, run *simpleworkflow.WorkflowRun) (interface{}, error) {
	// Parse payload
	var params struct {
		ContentID string `json:"content_id"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	}
	if err := json.Unmarshal(run.Payload, &params); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	runID := uuid.New().String()
	log.Printf("[%s] Executing thumbnail workflow for content_id=%s (%dx%d)", runID, params.ContentID, params.Width, params.Height)

	// Create thumbnail workflow instance
	thumbnailWorkflow := workflows.NewThumbnailWorkflow(e.contentReader, e.derivedWriter)

	// Build workflow request
	request := pipeline.ProcessRequest{
		ContentID: params.ContentID,
		Job:       pipeline.JobThumbnail,
		Versions: map[string]int{
			pipeline.DerivedTypeThumbnail: 1,
		},
		Metadata: map[string]string{
			"width":  fmt.Sprintf("%d", params.Width),
			"height": fmt.Sprintf("%d", params.Height),
		},
	}

	// Build workflow context
	wctx := &workflows.WorkflowContext{
		Ctx:     ctx,
		Request: request,
		RunID:   runID,
	}

	// Execute thumbnail workflow directly
	result, err := thumbnailWorkflow.Execute(wctx)
	if err != nil {
		return nil, fmt.Errorf("thumbnail workflow failed: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("thumbnail workflow returned failure: %v", result.Error)
	}

	log.Printf("[%s] Thumbnail workflow completed successfully", runID)

	return map[string]interface{}{
		"run_id":     runID,
		"content_id": params.ContentID,
		"dimensions": fmt.Sprintf("%dx%d", params.Width, params.Height),
		"outputs":    result.Outputs,
	}, nil
}
