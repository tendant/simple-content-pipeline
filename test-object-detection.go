package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/tendant/simple-content-pipeline/pkg/runner"
)

func main() {
	// Load .env file
	_ = godotenv.Load()

	// Get content ID from command line
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test-object-detection.go <content-id>")
		os.Exit(1)
	}
	contentID := os.Args[1]

	// Initialize pipeline client
	dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DBOS_SYSTEM_DATABASE_URL not set")
	}

	queueName := os.Getenv("DBOS_QUEUE_NAME")
	if queueName == "" {
		queueName = "default"
	}

	appVersion := os.Getenv("DBOS_APPLICATION_VERSION")
	contentAPIURL := os.Getenv("CONTENT_API_URL")
	if contentAPIURL == "" {
		contentAPIURL = "http://localhost:8080"
	}

	client, err := runner.NewClient(runner.Config{
		DatabaseURL:        dbURL,
		AppName:            "content-pipeline",
		QueueName:          queueName,
		ContentAPIURL:      contentAPIURL,
		ApplicationVersion: appVersion,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Shutdown(5)

	fmt.Printf("Triggering object detection for content: %s\n", contentID)

	// Trigger object detection
	runID, err := client.RunObjectDetection(context.Background(), contentID)
	if err != nil {
		log.Fatalf("Failed to trigger object detection: %v", err)
	}

	fmt.Printf("✓ Object detection workflow enqueued\n")
	fmt.Printf("  Run ID: %s\n", runID)
	fmt.Printf("\nCheck logs for results:\n")
	fmt.Printf("  tail -f python-worker/python-worker.log | grep %s\n", contentID[:8])
}
