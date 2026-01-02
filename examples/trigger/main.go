package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/tendant/simple-content/pkg/simplecontent"
	"github.com/tendant/simple-content/pkg/simplecontent/presets"
	"github.com/tendant/simple-content-pipeline/pkg/client"
	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

func main() {
	ctx := context.Background()

	// Step 1: Upload content using simple-content
	fmt.Println("Step 1: Uploading content to simple-content...")
	svc, cleanup, err := presets.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to initialize simple-content: %v", err)
	}
	defer cleanup()

	// Upload test content
	content, err := svc.UploadContent(ctx, simplecontent.UploadContentRequest{
		OwnerID:      uuid.New(),
		TenantID:     uuid.New(),
		Name:         "Test Photo",
		DocumentType: "image/jpeg",
		Reader:       strings.NewReader("test image data"),
		FileName:     "photo.jpg",
		Tags:         []string{"test", "photo"},
	})
	if err != nil {
		log.Fatalf("Failed to upload content: %v", err)
	}

	fmt.Printf("✓ Content uploaded: %s\n", content.ID)

	// Step 2: Trigger pipeline processing
	fmt.Println("\nStep 2: Triggering thumbnail generation...")
	pipelineClient := client.New("http://localhost:8080")

	req := pipeline.ProcessRequest{
		ContentID: content.ID.String(),
		Job:       pipeline.JobThumbnail,
		Versions: map[string]int{
			pipeline.DerivedTypeThumbnail: 1,
		},
		Metadata: map[string]string{
			"mime": "image/jpeg",
		},
	}

	resp, err := pipelineClient.Process(ctx, req)
	if err != nil {
		log.Fatalf("Failed to trigger processing: %v", err)
	}

	fmt.Printf("✓ Processing triggered successfully\n")
	fmt.Printf("  Run ID: %s\n", resp.RunID)
	fmt.Printf("  Dedupe seen count: %d\n", resp.DedupeSeenCount)

	// Step 3: List derived content
	fmt.Println("\nStep 3: Checking derived content...")
	derived, err := svc.ListDerivedContent(ctx, simplecontent.WithParentID(content.ID))
	if err != nil {
		log.Fatalf("Failed to list derived content: %v", err)
	}

	if len(derived) > 0 {
		fmt.Printf("✓ Found %d derived content(s):\n", len(derived))
		for _, d := range derived {
			fmt.Printf("  - Type: %s, Variant: %s\n", d.DerivationType, d.Variant)
		}
	} else {
		fmt.Println("  No derived content found yet")
	}
}
