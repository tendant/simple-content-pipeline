package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/tendant/simple-content/pkg/simplecontent"
)

// DerivedWriter provides write access for derived content via simple-content service
type DerivedWriter struct {
	service simplecontent.Service
}

// NewDerivedWriter creates a new derived content writer
func NewDerivedWriter(service simplecontent.Service) *DerivedWriter {
	return &DerivedWriter{
		service: service,
	}
}

// HasDerived checks if a derived output already exists for the given type/version
func (dw *DerivedWriter) HasDerived(ctx context.Context, contentID string, derivedType string, derivedVersion int) (bool, error) {
	// Parse content ID
	parentID, err := uuid.Parse(contentID)
	if err != nil {
		return false, fmt.Errorf("invalid content ID: %w", err)
	}

	// List derived content for parent
	derived, err := dw.service.ListDerivedContent(ctx,
		simplecontent.WithParentID(parentID),
		simplecontent.WithDerivationType(derivedType),
	)
	if err != nil {
		return false, fmt.Errorf("failed to list derived content: %w", err)
	}

	// Check if any derived content matches the version
	// Note: simple-content stores variant, not version directly
	// We'll need to check the variant string (e.g., "thumbnail_v1")
	for _, d := range derived {
		// For now, just check if derived type exists
		// TODO: Properly handle version checking via variant
		if d.DerivationType == derivedType {
			return true, nil
		}
	}

	return false, nil
}

// PutDerived creates or upserts a derived output and returns its derived content ID
func (dw *DerivedWriter) PutDerived(ctx context.Context, contentID string, derivedType string, derivedVersion int, r io.Reader, meta map[string]string) (string, error) {
	// Parse parent content ID
	parentID, err := uuid.Parse(contentID)
	if err != nil {
		return "", fmt.Errorf("invalid content ID: %w", err)
	}

	// Create variant name from type and version
	variant := fmt.Sprintf("%s_v%d", derivedType, derivedVersion)

	// Extract metadata fields
	fileName := meta["file_name"]
	if fileName == "" {
		fileName = fmt.Sprintf("derived_%s.dat", derivedType)
	}

	// Upload derived content using simple-content
	derivedContent, err := dw.service.UploadDerivedContent(ctx, simplecontent.UploadDerivedContentRequest{
		ParentID:       parentID,
		DerivationType: derivedType,
		Variant:        variant,
		Reader:         r,
		FileName:       fileName,
		Tags:           []string{derivedType, variant},
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload derived content: %w", err)
	}

	return derivedContent.ID.String(), nil
}
