# Library Integration Example

This example demonstrates how to use `simple-content-pipeline` as a library in your own Go application.

## Running the Example

### Prerequisites

1. PostgreSQL running (for DBOS)
2. simple-content server running (optional, for full test)

```bash
# Terminal 1: Start PostgreSQL
make postgres-start

# Terminal 2: Start simple-content (optional)
cd ../simple-content
./standalone-server
```

### Run the example

```bash
cd examples/library-integration
go run main.go
```

## What This Example Shows

1. **Initialization** - How to set up the pipeline in your application
2. **Configuration** - Loading settings from environment variables
3. **Workflow Execution** - Triggering thumbnail generation programmatically
4. **Status Checking** - Querying workflow execution status
5. **Graceful Shutdown** - Properly stopping the pipeline

## Integration Steps

### Step 1: Add the dependency

```bash
go get github.com/tendant/simple-content-pipeline
```

### Step 2: Initialize the pipeline

```go
import (
    "github.com/tendant/simple-content-pipeline/internal/dbosruntime"
    "github.com/tendant/simple-content-pipeline/internal/storage"
    "github.com/tendant/simple-content-pipeline/internal/workflows"
    "github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// Create DBOS runtime
dbosRuntime, err := dbosruntime.NewRuntime(context.Background(), dbosruntime.Config{
    DatabaseURL: "postgresql://...",
    AppName:     "my-app",
    QueueName:   "default",
    Concurrency: 4,
})

// Create workflow runner
runner := workflows.NewWorkflowRunner(dbosRuntime)

// Setup storage adapters
contentReader := storage.NewHTTPContentReader("http://localhost:4000")
derivedWriter := storage.NewHTTPDerivedWriter("http://localhost:4000")

// Register workflows
thumbnailWorkflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
runner.Register(pipeline.JobThumbnail, thumbnailWorkflow)

// Launch DBOS
dbosRuntime.Launch()
defer dbosRuntime.Shutdown(10 * time.Second)
```

### Step 3: Trigger workflows

```go
// Trigger thumbnail generation
runID, err := runner.RunAsync(ctx, pipeline.ProcessRequest{
    ContentID: "your-content-id",
    Job:       pipeline.JobThumbnail,
    Versions: map[string]int{
        pipeline.DerivedTypeThumbnail: 1,
    },
})

if err != nil {
    // Handle error
}

log.Printf("Workflow queued: %s", runID)
```

### Step 4: Check status (optional)

```go
status, err := runner.GetStatus(ctx, runID)
if err != nil {
    // Handle error
}

log.Printf("Workflow state: %s", status.State)
```

## Use Cases

### Use Case 1: File Upload Handler

```go
func handleUpload(w http.ResponseWriter, r *http.Request) {
    // 1. Upload file to simple-content
    content, err := contentService.UploadContent(ctx, uploadReq)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }

    // 2. Trigger thumbnail generation
    runID, err := pipelineRunner.ProcessContent(content.ID.String())
    if err != nil {
        log.Printf("Failed to trigger pipeline: %v", err)
        // Continue anyway - processing happens in background
    }

    // 3. Return response immediately
    json.NewEncoder(w).Encode(map[string]string{
        "content_id": content.ID.String(),
        "run_id":     runID,
        "status":     "processing",
    })
}
```

### Use Case 2: Batch Processing

```go
func processBatch(contentIDs []string) {
    for _, contentID := range contentIDs {
        runID, err := pipelineRunner.ProcessContent(contentID)
        if err != nil {
            log.Printf("Failed to queue %s: %v", contentID, err)
            continue
        }
        log.Printf("Queued %s -> %s", contentID, runID)
    }
}
```

### Use Case 3: Event-Driven Processing

```go
// React to content upload events
func onContentUploaded(event ContentUploadedEvent) {
    // Automatically trigger thumbnail generation
    runID, err := pipelineRunner.ProcessContent(event.ContentID)
    if err != nil {
        log.Printf("Failed to process %s: %v", event.ContentID, err)
        return
    }

    log.Printf("Processing %s in background: %s", event.ContentID, runID)
}
```

## Architecture

```
┌─────────────────────┐
│  Your Application   │
│                     │
│  ┌───────────────┐ │
│  │ Upload Handler│ │
│  └───────┬───────┘ │
│          │         │
│          v         │
│  ┌───────────────┐ │
│  │Pipeline Runner│ │  Trigger workflows
│  │               │ │
│  │  ┌─────────┐  │ │
│  │  │  DBOS   │  │ │  Durable execution
│  │  └─────────┘  │ │
│  └───────────────┘ │
└─────────┬───────────┘
          │
          v
┌─────────────────────┐
│ simple-content API  │  Storage
└─────────────────────┘
```

## Configuration

The example loads configuration from environment variables:

```bash
# Required
DBOS_SYSTEM_DATABASE_URL=postgresql://pipeline:pwd@localhost:5434/pipeline

# Optional
DBOS_QUEUE_NAME=default
DBOS_QUEUE_CONCURRENCY=4
CONTENT_API_URL=http://localhost:4000
```

You can use a `.env` file in the project root (see `../../.env.example`).

## Troubleshooting

### "DBOS_SYSTEM_DATABASE_URL is required"

Set the environment variable or create a `.env` file:

```bash
export DBOS_SYSTEM_DATABASE_URL=postgresql://pipeline:pwd@localhost:5434/pipeline
```

### "Failed to call content API"

Make sure simple-content server is running:

```bash
cd ../simple-content
./standalone-server
```

Or update `CONTENT_API_URL` to point to your server.

### "Cannot register workflow after DBOS has launched"

Make sure you register workflows BEFORE calling `dbosRuntime.Launch()`:

```go
runner.Register(pipeline.JobThumbnail, thumbnailWorkflow)  // BEFORE
dbosRuntime.Launch()                                        // Launch
```

## Next Steps

- See [LIBRARY_USAGE.md](../../LIBRARY_USAGE.md) for complete API documentation
- See [README.md](../../README.md) for project overview
- Implement custom workflows for your specific needs
