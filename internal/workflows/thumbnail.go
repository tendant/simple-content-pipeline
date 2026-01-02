package workflows

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// ContentReader interface for reading content
type ContentReader interface {
	GetReaderByContentID(ctx context.Context, contentID string) (io.ReadCloser, error)
	Exists(ctx context.Context, key string) (bool, error)
}

// DerivedWriter interface for writing derived content
type DerivedWriter interface {
	HasDerived(ctx context.Context, contentID string, derivedType string, derivedVersion int) (bool, error)
	PutDerived(ctx context.Context, contentID string, derivedType string, derivedVersion int, r io.Reader, meta map[string]string) (string, error)
}

// ThumbnailWorkflow generates thumbnails for content
type ThumbnailWorkflow struct {
	contentReader ContentReader
	derivedWriter DerivedWriter
}

// NewThumbnailWorkflow creates a new thumbnail generation workflow
func NewThumbnailWorkflow(contentReader ContentReader, derivedWriter DerivedWriter) *ThumbnailWorkflow {
	return &ThumbnailWorkflow{
		contentReader: contentReader,
		derivedWriter: derivedWriter,
	}
}

// Name returns the workflow name
func (w *ThumbnailWorkflow) Name() string {
	return "ThumbnailWorkflow"
}

// Execute runs the thumbnail generation workflow
func (w *ThumbnailWorkflow) Execute(wctx *WorkflowContext) (*WorkflowResult, error) {
	log.Printf("[%s] Starting thumbnail workflow for content_id=%s", wctx.RunID, wctx.Request.ContentID)

	// Step 1: Validate request
	if err := w.validateRequest(&wctx.Request); err != nil {
		log.Printf("[%s] Validation failed: %v", wctx.RunID, err)
		return &WorkflowResult{
			Success: false,
			Error:   fmt.Errorf("validation failed: %w", err),
		}, err
	}

	derivedType := pipeline.DerivedTypeThumbnail
	derivedVersion := wctx.Request.Versions[derivedType]

	// Step 2: Check if derived content already exists (skip if present)
	hasDerived, err := w.derivedWriter.HasDerived(wctx.Ctx, wctx.Request.ContentID, derivedType, derivedVersion)
	if err != nil {
		log.Printf("[%s] Failed to check derived content: %v", wctx.RunID, err)
		// Continue anyway - don't fail on check error
	} else if hasDerived {
		log.Printf("[%s] Derived content already exists (type=%s, version=%d) - skipping", wctx.RunID, derivedType, derivedVersion)
		return &WorkflowResult{
			Success: true,
			Outputs: map[string]interface{}{
				"content_id":   wctx.Request.ContentID,
				"derived_type": derivedType,
				"version":      derivedVersion,
				"skipped":      true,
			},
		}, nil
	}

	// Step 3: Check if source content exists
	exists, err := w.contentReader.Exists(wctx.Ctx, wctx.Request.ContentID)
	if err != nil {
		log.Printf("[%s] Failed to check content existence: %v", wctx.RunID, err)
		return &WorkflowResult{
			Success: false,
			Error:   fmt.Errorf("content check failed: %w", err),
		}, err
	}

	if !exists {
		log.Printf("[%s] Source content not found: %s", wctx.RunID, wctx.Request.ContentID)
		return &WorkflowResult{
			Success: false,
			Error:   fmt.Errorf("source content not found: %s", wctx.Request.ContentID),
		}, nil
	}

	log.Printf("[%s] Source content exists: %s", wctx.RunID, wctx.Request.ContentID)

	// Step 4: Download source content
	reader, err := w.contentReader.GetReaderByContentID(wctx.Ctx, wctx.Request.ContentID)
	if err != nil {
		log.Printf("[%s] Failed to download source content: %v", wctx.RunID, err)
		return &WorkflowResult{
			Success: false,
			Error:   fmt.Errorf("download failed: %w", err),
		}, err
	}
	defer reader.Close()

	log.Printf("[%s] Source content downloaded successfully", wctx.RunID)

	// Step 5: Generate thumbnail (placeholder for now)
	// TODO: Implement actual thumbnail generation
	// For now, we'll just use the source content as-is
	log.Printf("[%s] Thumbnail generation (placeholder) - using source as-is", wctx.RunID)

	// Step 6: Write derived content
	meta := map[string]string{
		"file_name": fmt.Sprintf("thumbnail_v%d.jpg", derivedVersion),
	}

	derivedID, err := w.derivedWriter.PutDerived(wctx.Ctx, wctx.Request.ContentID, derivedType, derivedVersion, reader, meta)
	if err != nil {
		log.Printf("[%s] Failed to write derived content: %v", wctx.RunID, err)
		return &WorkflowResult{
			Success: false,
			Error:   fmt.Errorf("derived write failed: %w", err),
		}, err
	}

	log.Printf("[%s] Derived content written: %s", wctx.RunID, derivedID)
	log.Printf("[%s] Thumbnail workflow completed successfully", wctx.RunID)

	return &WorkflowResult{
		Success: true,
		Outputs: map[string]interface{}{
			"content_id":   wctx.Request.ContentID,
			"derived_id":   derivedID,
			"derived_type": derivedType,
			"version":      derivedVersion,
		},
	}, nil
}

// validateRequest validates the workflow request
func (w *ThumbnailWorkflow) validateRequest(req *pipeline.ProcessRequest) error {
	// content_id is validated at the HTTP handler level

	// Check that thumbnail version is provided
	version, ok := req.Versions[pipeline.DerivedTypeThumbnail]
	if !ok {
		return fmt.Errorf("thumbnail version not provided in versions map")
	}

	if version < 1 {
		return fmt.Errorf("invalid thumbnail version: %d", version)
	}

	return nil
}
