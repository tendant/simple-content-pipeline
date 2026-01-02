package main

import (
	"context"
	"fmt"
	"log"

	"github.com/tendant/simple-content-pipeline/pkg/client"
	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

func main() {
	// Create pipeline client
	c := client.New("http://localhost:8080")

	// Trigger thumbnail generation
	req := pipeline.ProcessRequest{
		ContentID: "content-123",
		ObjectKey: "uploads/photo.jpg",
		Job:       pipeline.JobThumbnail,
		Versions: map[string]int{
			pipeline.DerivedTypeThumbnail: 1,
		},
		Metadata: map[string]string{
			"mime": "image/jpeg",
			"size": "1024000",
		},
	}

	resp, err := c.Process(context.Background(), req)
	if err != nil {
		log.Fatalf("Failed to trigger processing: %v", err)
	}

	fmt.Printf("✓ Processing triggered successfully\n")
	fmt.Printf("  Run ID: %s\n", resp.RunID)
	fmt.Printf("  Dedupe seen count: %d\n", resp.DedupeSeenCount)
}
