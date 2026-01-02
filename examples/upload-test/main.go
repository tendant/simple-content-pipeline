package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

// Simple example showing how to call the pipeline's built-in test endpoint
// For production use with actual file uploads, you would need to:
// 1. Run simple-content server separately
// 2. Upload files to simple-content API
// 3. Call pipeline-worker with the content_id

func main() {
	pipelineURL := "http://localhost:8080"
	if len(os.Args) > 1 {
		pipelineURL = os.Args[1]
	}

	fmt.Println("=== Pipeline Test ===")
	fmt.Println()
	fmt.Printf("Testing: %s/v1/test\n", pipelineURL)
	fmt.Println()

	// Call the built-in test endpoint
	resp, err := http.Get(pipelineURL + "/v1/test")
	if err != nil {
		log.Fatalf("Failed to call test endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Test failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Fatalf("Failed to parse response: %v", err)
	}

	status, _ := result["test_status"].(string)
	if status == "success" {
		fmt.Println("✓ Test PASSED")
		fmt.Println()

		// Pretty print the result
		prettyJSON, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println("Results:")
		fmt.Println(string(prettyJSON))
	} else {
		fmt.Println("❌ Test FAILED")
		fmt.Println()
		prettyJSON, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(prettyJSON))
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("=== Test Complete ===")
}
