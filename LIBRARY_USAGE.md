# Using simple-content-pipeline as a Library

This guide shows how to integrate the pipeline library into your own Go applications.

## Installation

```bash
go get github.com/tendant/simple-content-pipeline
```

## Quick Start

Here's a minimal example of using the pipeline in your application:

```go
package main

import (
    "context"
    "log"

    "github.com/tendant/simple-content-pipeline/internal/dbosruntime"
    "github.com/tendant/simple-content-pipeline/internal/storage"
    "github.com/tendant/simple-content-pipeline/internal/workflows"
    "github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

func main() {
    // 1. Initialize DBOS runtime (required for durability)
    dbosRuntime, err := dbosruntime.NewRuntime(context.Background(), dbosruntime.Config{
        DatabaseURL: "postgresql://user:pass@localhost:5432/mydb",
        AppName:     "my-app",
        QueueName:   "default",
        Concurrency: 4,
    })
    if err != nil {
        log.Fatalf("Failed to initialize DBOS: %v", err)
    }

    // 2. Create workflow runner
    workflowRunner := workflows.NewWorkflowRunner(dbosRuntime)

    // 3. Register workflows (before launching DBOS)
    contentReader := storage.NewHTTPContentReader("http://localhost:4000")
    derivedWriter := storage.NewHTTPDerivedWriter("http://localhost:4000")

    thumbnailWorkflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
    workflowRunner.Register(pipeline.JobThumbnail, thumbnailWorkflow)

    // 4. Launch DBOS (must be after workflow registration)
    if err := dbosRuntime.Launch(); err != nil {
        log.Fatalf("Failed to launch DBOS: %v", err)
    }
    defer dbosRuntime.Shutdown(10 * time.Second)

    // 5. Trigger workflow processing (async)
    runID, err := workflowRunner.RunAsync(context.Background(), pipeline.ProcessRequest{
        ContentID: "your-content-id",
        Job:       pipeline.JobThumbnail,
        Versions: map[string]int{
            pipeline.DerivedTypeThumbnail: 1,
        },
    })
    if err != nil {
        log.Fatalf("Failed to enqueue workflow: %v", err)
    }

    log.Printf("Workflow enqueued with run_id: %s", runID)
}
```

## Architecture Overview

```
Your Application
    │
    ├─ DBOS Runtime (durability + queues)
    │
    ├─ Workflow Runner (orchestration)
    │   │
    │   └─ Workflows (thumbnail, etc.)
    │       │
    │       ├─ Content Reader (fetch source)
    │       └─ Derived Writer (store result)
    │
    └─ simple-content service (storage)
```

## Core Components

### 1. DBOS Runtime

Manages durable workflow execution with PostgreSQL-backed state:

```go
import "github.com/tendant/simple-content-pipeline/internal/dbosruntime"

dbosRuntime, err := dbosruntime.NewRuntime(ctx, dbosruntime.Config{
    DatabaseURL: "postgresql://...",  // Required
    AppName:     "my-app",            // Required
    QueueName:   "default",           // Default: "default"
    Concurrency: 4,                   // Default: 4
})
```

### 2. Workflow Runner

Orchestrates workflow execution:

```go
import "github.com/tendant/simple-content-pipeline/internal/workflows"

runner := workflows.NewWorkflowRunner(dbosRuntime)

// Register workflows BEFORE launching DBOS
runner.Register(pipeline.JobThumbnail, thumbnailWorkflow)

// Launch DBOS
dbosRuntime.Launch()

// Run workflow asynchronously
runID, err := runner.RunAsync(ctx, pipeline.ProcessRequest{
    ContentID: "content-id",
    Job:       pipeline.JobThumbnail,
    Versions:  map[string]int{"thumbnail": 1},
})
```

### 3. Storage Adapters

Connect to simple-content service:

```go
import "github.com/tendant/simple-content-pipeline/internal/storage"

// Option A: HTTP client (for distributed setup)
contentReader := storage.NewHTTPContentReader("http://localhost:4000")
derivedWriter := storage.NewHTTPDerivedWriter("http://localhost:4000")

// Option B: Embedded service (for all-in-one setup)
svc, cleanup, _ := presets.NewDevelopment()
defer cleanup()
contentReader := storage.NewContentReader(svc)
derivedWriter := storage.NewDerivedWriter(svc)
```

### 4. Built-in Workflows

#### Thumbnail Workflow

```go
import "github.com/tendant/simple-content-pipeline/internal/workflows"

thumbnailWorkflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
runner.Register(pipeline.JobThumbnail, thumbnailWorkflow)

// Trigger it
runID, _ := runner.RunAsync(ctx, pipeline.ProcessRequest{
    ContentID: "content-id",
    Job:       pipeline.JobThumbnail,
    Versions: map[string]int{
        pipeline.DerivedTypeThumbnail: 1,  // Version number
    },
    Metadata: map[string]string{
        "mime": "image/jpeg",
    },
})
```

## Custom Workflows

Create your own workflows by implementing the `Workflow` interface:

```go
package myworkflows

import (
    "github.com/tendant/simple-content-pipeline/internal/workflows"
    "github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

type CustomWorkflow struct {
    contentReader ContentReader
    derivedWriter DerivedWriter
}

func NewCustomWorkflow(reader ContentReader, writer DerivedWriter) *CustomWorkflow {
    return &CustomWorkflow{
        contentReader: reader,
        derivedWriter: writer,
    }
}

// Name returns the workflow identifier
func (w *CustomWorkflow) Name() string {
    return "CustomWorkflow"
}

// Execute implements the workflow logic
func (w *CustomWorkflow) Execute(wctx *workflows.WorkflowContext) (*workflows.WorkflowResult, error) {
    log.Printf("[%s] Starting custom workflow for content_id=%s",
        wctx.RunID, wctx.Request.ContentID)

    // 1. Fetch source content
    reader, err := w.contentReader.GetReaderByContentID(wctx.Ctx, wctx.Request.ContentID)
    if err != nil {
        return &workflows.WorkflowResult{Success: false, Error: err}, err
    }
    defer reader.Close()

    // 2. Process content (your custom logic)
    processedData := yourCustomProcessing(reader)

    // 3. Write derived content
    derivedID, err := w.derivedWriter.PutDerived(
        wctx.Ctx,
        wctx.Request.ContentID,
        "custom-type",  // Your derived type
        1,              // Version
        processedData,
        map[string]string{"metadata": "value"},
    )
    if err != nil {
        return &workflows.WorkflowResult{Success: false, Error: err}, err
    }

    log.Printf("[%s] Custom workflow completed: %s", wctx.RunID, derivedID)

    return &workflows.WorkflowResult{
        Success: true,
        Outputs: map[string]interface{}{"derived_id": derivedID},
    }, nil
}

// Register your workflow
const JobCustom = "custom-job"

runner.Register(JobCustom, NewCustomWorkflow(contentReader, derivedWriter))
```

## Integration Patterns

### Pattern 1: Embedded All-in-One

Best for: Single-process applications, development, testing

```go
package main

import (
    "github.com/tendant/simple-content/pkg/simplecontent/presets"
    "github.com/tendant/simple-content-pipeline/internal/dbosruntime"
    "github.com/tendant/simple-content-pipeline/internal/storage"
    "github.com/tendant/simple-content-pipeline/internal/workflows"
)

func main() {
    // Embedded simple-content service
    svc, cleanup, _ := presets.NewDevelopment()
    defer cleanup()

    // Initialize pipeline
    dbosRuntime, _ := dbosruntime.NewRuntime(ctx, dbosruntime.Config{
        DatabaseURL: os.Getenv("DBOS_SYSTEM_DATABASE_URL"),
        AppName:     "my-app",
    })

    runner := workflows.NewWorkflowRunner(dbosRuntime)

    contentReader := storage.NewContentReader(svc)
    derivedWriter := storage.NewDerivedWriter(svc)

    thumbnailWorkflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
    runner.Register(pipeline.JobThumbnail, thumbnailWorkflow)

    dbosRuntime.Launch()
    defer dbosRuntime.Shutdown(10 * time.Second)

    // Your application logic here
}
```

### Pattern 2: Distributed Worker

Best for: Production, microservices, horizontal scaling

```go
package main

import (
    "github.com/tendant/simple-content-pipeline/internal/dbosruntime"
    "github.com/tendant/simple-content-pipeline/internal/storage"
    "github.com/tendant/simple-content-pipeline/internal/workflows"
)

func main() {
    // Connect to external simple-content service
    contentAPIURL := os.Getenv("CONTENT_API_URL")
    contentReader := storage.NewHTTPContentReader(contentAPIURL)
    derivedWriter := storage.NewHTTPDerivedWriter(contentAPIURL)

    // Initialize pipeline worker
    dbosRuntime, _ := dbosruntime.NewRuntime(ctx, dbosruntime.Config{
        DatabaseURL: os.Getenv("DBOS_SYSTEM_DATABASE_URL"),
        AppName:     "my-worker",
        QueueName:   "processing-queue",
        Concurrency: 10,
    })

    runner := workflows.NewWorkflowRunner(dbosRuntime)

    thumbnailWorkflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
    runner.Register(pipeline.JobThumbnail, thumbnailWorkflow)

    dbosRuntime.Launch()
    defer dbosRuntime.Shutdown(10 * time.Second)

    // Worker stays running, processing queued workflows
    select {}
}
```

### Pattern 3: HTTP API Integration

Best for: Adding pipeline to existing web services

```go
package main

import (
    "net/http"
    "github.com/tendant/simple-content-pipeline/internal/handlers"
    "github.com/tendant/simple-content-pipeline/internal/workflows"
)

func main() {
    // Setup pipeline (as above)
    dbosRuntime, _ := dbosruntime.NewRuntime(...)
    runner := workflows.NewWorkflowRunner(dbosRuntime)
    // ... register workflows ...
    dbosRuntime.Launch()
    defer dbosRuntime.Shutdown(10 * time.Second)

    // Add pipeline endpoints to your existing HTTP server
    asyncHandler := handlers.NewAsyncHandler(runner)

    http.HandleFunc("/api/pipeline/process", asyncHandler.HandleProcessAsync)
    http.HandleFunc("/api/pipeline/runs/", asyncHandler.HandleStatus)

    http.ListenAndServe(":8080", nil)
}
```

## Configuration

### Environment Variables

```bash
# DBOS Configuration (Required)
DBOS_SYSTEM_DATABASE_URL=postgresql://user:pass@localhost:5432/mydb
DBOS_QUEUE_NAME=default
DBOS_QUEUE_CONCURRENCY=4

# simple-content API (for distributed setup)
CONTENT_API_URL=http://localhost:4000
```

### Load from .env file

```go
import "github.com/joho/godotenv"

func main() {
    // Load .env file (optional)
    _ = godotenv.Load()

    // Use environment variables
    dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
    contentAPIURL := os.Getenv("CONTENT_API_URL")

    // ... initialize pipeline ...
}
```

## API Reference

### ProcessRequest

```go
type ProcessRequest struct {
    ContentID string            // Source content ID
    Job       string            // Workflow type ("thumbnail", etc.)
    ObjectKey string            // Optional S3-like key
    Versions  map[string]int    // Derived type -> version number
    Metadata  map[string]string // Custom metadata
}
```

### ProcessResponse

```go
type ProcessResponse struct {
    RunID           string // Workflow execution ID
    DedupeSeenCount int    // Number of times seen (future feature)
}
```

### Workflow Result

```go
type WorkflowResult struct {
    Success bool
    Error   error
    Outputs map[string]interface{} // Custom output data
}
```

## Error Handling

```go
runID, err := runner.RunAsync(ctx, req)
if err != nil {
    if errors.Is(err, workflows.ErrWorkflowNotFound) {
        // Unknown job type
    } else if strings.Contains(err.Error(), "DBOS runtime not initialized") {
        // DBOS not set up
    } else {
        // Other errors
    }
}

// Check workflow status
status, err := runner.GetStatus(ctx, runID)
if err != nil {
    // Workflow not found or error
}

if status.State == "failed" {
    log.Printf("Workflow failed: %v", status.Error)
}
```

## Best Practices

1. **Always initialize DBOS** - Required for durability and crash recovery
2. **Register workflows before Launch()** - DBOS must know about workflows before starting
3. **Use async execution** - Call `RunAsync()` instead of `Run()` for background processing
4. **Handle errors gracefully** - Check workflow status, implement retries if needed
5. **Use environment variables** - Keep configuration flexible
6. **Monitor PostgreSQL** - DBOS stores workflow state in the database
7. **Scale horizontally** - Run multiple workers pointing to same database

## Example: Adding Pipeline to Existing App

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/joho/godotenv"
    "github.com/tendant/simple-content-pipeline/internal/dbosruntime"
    "github.com/tendant/simple-content-pipeline/internal/storage"
    "github.com/tendant/simple-content-pipeline/internal/workflows"
    "github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

func main() {
    // Load configuration
    _ = godotenv.Load()

    // Initialize pipeline
    pipelineRunner := initializePipeline()
    defer shutdownPipeline(pipelineRunner)

    // Your existing application logic
    runYourApp(pipelineRunner)
}

func initializePipeline() *PipelineRunner {
    dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
    if dbURL == "" {
        log.Fatal("DBOS_SYSTEM_DATABASE_URL required")
    }

    dbosRuntime, err := dbosruntime.NewRuntime(context.Background(), dbosruntime.Config{
        DatabaseURL: dbURL,
        AppName:     "my-application",
        QueueName:   "default",
        Concurrency: 4,
    })
    if err != nil {
        log.Fatalf("Failed to initialize DBOS: %v", err)
    }

    runner := workflows.NewWorkflowRunner(dbosRuntime)

    // Setup storage adapters
    contentAPIURL := os.Getenv("CONTENT_API_URL")
    if contentAPIURL == "" {
        contentAPIURL = "http://localhost:4000"
    }
    contentReader := storage.NewHTTPContentReader(contentAPIURL)
    derivedWriter := storage.NewHTTPDerivedWriter(contentAPIURL)

    // Register workflows
    thumbnailWorkflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
    runner.Register(pipeline.JobThumbnail, thumbnailWorkflow)

    // Launch DBOS
    if err := dbosRuntime.Launch(); err != nil {
        log.Fatalf("Failed to launch DBOS: %v", err)
    }

    log.Println("✓ Pipeline initialized")

    return &PipelineRunner{
        runtime: dbosRuntime,
        runner:  runner,
    }
}

type PipelineRunner struct {
    runtime *dbosruntime.Runtime
    runner  *workflows.WorkflowRunner
}

func (p *PipelineRunner) ProcessContent(contentID string) (string, error) {
    return p.runner.RunAsync(context.Background(), pipeline.ProcessRequest{
        ContentID: contentID,
        Job:       pipeline.JobThumbnail,
        Versions: map[string]int{
            pipeline.DerivedTypeThumbnail: 1,
        },
    })
}

func shutdownPipeline(p *PipelineRunner) {
    log.Println("Shutting down pipeline...")
    p.runtime.Shutdown(10 * time.Second)
    log.Println("Pipeline stopped")
}

func runYourApp(pipeline *PipelineRunner) {
    // Example: Process content when uploaded
    contentID := "some-uploaded-content-id"
    runID, err := pipeline.ProcessContent(contentID)
    if err != nil {
        log.Printf("Failed to process content: %v", err)
        return
    }
    log.Printf("Processing started: %s", runID)

    // Wait for shutdown signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
}
```

## See Also

- [README.md](README.md) - Project overview
- [examples/upload-test/README.md](examples/upload-test/README.md) - Testing examples
- [DBOS Documentation](https://docs.dbos.dev/golang/) - DBOS Transact for Go
