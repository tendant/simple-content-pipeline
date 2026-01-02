# API Reference

Public interfaces for using simple-content-pipeline as a library.

## Package Structure

```
github.com/tendant/simple-content-pipeline/
├── pkg/pipeline/              # Public API types
├── internal/dbosruntime/      # DBOS runtime management
├── internal/workflows/        # Workflow execution
├── internal/storage/          # Storage adapters
└── internal/handlers/         # HTTP handlers
```

## Core Types

### pipeline.ProcessRequest

Request to trigger workflow processing.

```go
package pipeline

type ProcessRequest struct {
    ContentID string            `json:"content_id"` // Source content ID
    Job       string            `json:"job"`        // Workflow type
    ObjectKey string            `json:"object_key"` // Optional object key
    Versions  map[string]int    `json:"versions"`   // Derived type -> version
    Metadata  map[string]string `json:"metadata"`   // Custom metadata
}
```

**Fields:**
- `ContentID` - UUID of the source content to process
- `Job` - Workflow type to execute (e.g., `JobThumbnail`)
- `ObjectKey` - Optional S3-like object key for storage
- `Versions` - Map of derived type to version number
- `Metadata` - Custom metadata passed to workflow

**Example:**
```go
req := pipeline.ProcessRequest{
    ContentID: "9f8e7d6c-5b4a-3210-fedc-ba9876543210",
    Job:       pipeline.JobThumbnail,
    Versions: map[string]int{
        pipeline.DerivedTypeThumbnail: 1,
    },
    Metadata: map[string]string{
        "mime":   "image/jpeg",
        "source": "upload",
    },
}
```

### pipeline.ProcessResponse

Response from triggering a workflow.

```go
type ProcessResponse struct {
    RunID           string `json:"run_id"`            // Workflow execution ID
    DedupeSeenCount int    `json:"dedupe_seen_count"` // Deduplication count
}
```

**Fields:**
- `RunID` - Unique identifier for this workflow execution
- `DedupeSeenCount` - Number of times this request has been seen (future feature)

### pipeline Constants

```go
package pipeline

const (
    // Job types
    JobThumbnail = "thumbnail"

    // Derived types
    DerivedTypeThumbnail = "thumbnail"
)
```

## DBOS Runtime

### dbosruntime.Config

Configuration for DBOS runtime.

```go
package dbosruntime

type Config struct {
    DatabaseURL string  // PostgreSQL connection string (required)
    AppName     string  // Application name (required)
    QueueName   string  // Queue name (default: "default")
    Concurrency int     // Worker concurrency (default: 4)
}
```

**Example:**
```go
cfg := dbosruntime.Config{
    DatabaseURL: "postgresql://user:pass@localhost:5432/db",
    AppName:     "my-app",
    QueueName:   "processing",
    Concurrency: 10,
}
```

### dbosruntime.Runtime

Manages DBOS runtime lifecycle.

```go
func NewRuntime(ctx context.Context, cfg Config) (*Runtime, error)
```

Creates a new DBOS runtime instance.

**Returns:**
- `*Runtime` - Runtime instance
- `error` - Error if initialization fails

**Methods:**

#### Launch
```go
func (r *Runtime) Launch() error
```

Starts the DBOS runtime and workers. Must be called AFTER registering workflows.

#### Shutdown
```go
func (r *Runtime) Shutdown(timeout time.Duration) error
```

Gracefully shuts down the DBOS runtime.

**Parameters:**
- `timeout` - Maximum time to wait for shutdown

#### Context
```go
func (r *Runtime) Context() dbos.DBOSContext
```

Returns the DBOS context for workflow registration.

#### QueueName
```go
func (r *Runtime) QueueName() string
```

Returns the configured queue name.

#### Concurrency
```go
func (r *Runtime) Concurrency() int
```

Returns the configured concurrency level.

## Workflow Runner

### workflows.WorkflowRunner

Orchestrates workflow execution.

```go
func NewWorkflowRunner(dbosRuntime *dbosruntime.Runtime) *WorkflowRunner
```

Creates a new workflow runner.

**Methods:**

#### Register
```go
func (r *WorkflowRunner) Register(job string, workflow Workflow)
```

Registers a workflow for a job type. Must be called BEFORE `dbosRuntime.Launch()`.

**Parameters:**
- `job` - Job type identifier (e.g., `pipeline.JobThumbnail`)
- `workflow` - Workflow implementation

#### RunAsync
```go
func (r *WorkflowRunner) RunAsync(ctx context.Context, req pipeline.ProcessRequest) (string, error)
```

Enqueues a workflow for async execution.

**Returns:**
- `string` - Run ID for tracking
- `error` - Error if enqueueing fails

#### Run (Synchronous - deprecated)
```go
func (r *WorkflowRunner) Run(wctx *WorkflowContext) (*WorkflowResult, error)
```

Executes a workflow synchronously. Use `RunAsync` instead for production.

#### GetStatus
```go
func (r *WorkflowRunner) GetStatus(ctx context.Context, runID string) (*WorkflowStatus, error)
```

Retrieves workflow execution status.

**Returns:**
- `*WorkflowStatus` - Status information
- `error` - Error if status retrieval fails

### workflows.WorkflowContext

Context passed to workflow execution.

```go
type WorkflowContext struct {
    Ctx     context.Context       // Execution context
    Request pipeline.ProcessRequest // Original request
    RunID   string                // Workflow run ID
}
```

### workflows.WorkflowResult

Result from workflow execution.

```go
type WorkflowResult struct {
    Success bool
    Error   error
    Outputs map[string]interface{} // Custom output data
}
```

### workflows.WorkflowStatus

Status of a workflow execution.

```go
type WorkflowStatus struct {
    RunID      string
    State      string         // "pending", "running", "succeeded", "failed"
    StartedAt  time.Time
    FinishedAt *time.Time
    Result     *WorkflowResult
    Error      error
}
```

### workflows.Workflow Interface

Interface for implementing custom workflows.

```go
type Workflow interface {
    Execute(wctx *WorkflowContext) (*WorkflowResult, error)
    Name() string
}
```

**Example Implementation:**
```go
type MyWorkflow struct {
    reader ContentReader
    writer DerivedWriter
}

func (w *MyWorkflow) Name() string {
    return "MyWorkflow"
}

func (w *MyWorkflow) Execute(wctx *WorkflowContext) (*WorkflowResult, error) {
    // Your workflow logic here
    return &WorkflowResult{Success: true}, nil
}
```

## Storage Adapters

### HTTP Storage Adapters

For connecting to external simple-content API:

```go
package storage

func NewHTTPContentReader(apiURL string) *HTTPContentReader
func NewHTTPDerivedWriter(apiURL string) *HTTPDerivedWriter
```

**Example:**
```go
contentReader := storage.NewHTTPContentReader("http://localhost:4000")
derivedWriter := storage.NewHTTPDerivedWriter("http://localhost:4000")
```

### Embedded Storage Adapters

For using embedded simple-content service:

```go
func NewContentReader(svc simplecontent.Service) *ContentReader
func NewDerivedWriter(svc simplecontent.Service) *DerivedWriter
```

**Example:**
```go
svc, cleanup, _ := presets.NewDevelopment()
defer cleanup()

contentReader := storage.NewContentReader(svc)
derivedWriter := storage.NewDerivedWriter(svc)
```

### ContentReader Interface

```go
type ContentReader interface {
    GetReaderByContentID(ctx context.Context, contentID string) (io.ReadCloser, error)
    Exists(ctx context.Context, key string) (bool, error)
}
```

### DerivedWriter Interface

```go
type DerivedWriter interface {
    HasDerived(ctx context.Context, contentID string, derivedType string, version int) (bool, error)
    PutDerived(ctx context.Context, contentID string, derivedType string, version int, r io.Reader, meta map[string]string) (string, error)
}
```

## HTTP Handlers

### handlers.AsyncHandler

HTTP handlers for async workflow endpoints.

```go
package handlers

func NewAsyncHandler(runner *workflows.WorkflowRunner) *AsyncHandler
```

**Methods:**

#### HandleProcessAsync
```go
func (h *AsyncHandler) HandleProcessAsync(w http.ResponseWriter, r *http.Request)
```

Handles `POST /v1/process` - Enqueues workflow and returns 202 Accepted.

**Request Body:**
```json
{
  "content_id": "uuid",
  "job": "thumbnail",
  "versions": {"thumbnail": 1},
  "metadata": {"key": "value"}
}
```

**Response (202 Accepted):**
```json
{
  "run_id": "workflow-uuid",
  "dedupe_seen_count": 0
}
```

#### HandleStatus
```go
func (h *AsyncHandler) HandleStatus(w http.ResponseWriter, r *http.Request)
```

Handles `GET /v1/runs/{runID}` - Returns workflow status.

**Response:**
```json
{
  "run_id": "workflow-uuid",
  "state": "running",
  "started_at": "2026-01-01T12:00:00Z"
}
```

## Built-in Workflows

### ThumbnailWorkflow

Generates thumbnails from images.

```go
package workflows

func NewThumbnailWorkflow(reader ContentReader, writer DerivedWriter) *ThumbnailWorkflow
```

**Usage:**
```go
workflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
runner.Register(pipeline.JobThumbnail, workflow)
```

**Process Request:**
```go
req := pipeline.ProcessRequest{
    ContentID: "content-uuid",
    Job:       pipeline.JobThumbnail,
    Versions: map[string]int{
        pipeline.DerivedTypeThumbnail: 1,
    },
    Metadata: map[string]string{
        "mime": "image/jpeg",
    },
}
```

## Error Handling

### Common Errors

```go
var (
    ErrWorkflowNotFound = errors.New("workflow not found for job type")
    ErrSourceNotFound   = errors.New("source content not found")
)
```

### Error Checking

```go
import "errors"

runID, err := runner.RunAsync(ctx, req)
if err != nil {
    if errors.Is(err, workflows.ErrWorkflowNotFound) {
        // Unknown job type
    } else if strings.Contains(err.Error(), "DBOS runtime not initialized") {
        // DBOS not configured
    } else {
        // Other error
    }
}
```

## Complete Example

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/tendant/simple-content-pipeline/internal/dbosruntime"
    "github.com/tendant/simple-content-pipeline/internal/storage"
    "github.com/tendant/simple-content-pipeline/internal/workflows"
    "github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

func main() {
    // 1. Initialize DBOS runtime
    dbosRuntime, err := dbosruntime.NewRuntime(context.Background(), dbosruntime.Config{
        DatabaseURL: "postgresql://user:pass@localhost:5432/db",
        AppName:     "my-app",
        QueueName:   "default",
        Concurrency: 4,
    })
    if err != nil {
        log.Fatalf("Failed to init DBOS: %v", err)
    }

    // 2. Create workflow runner
    runner := workflows.NewWorkflowRunner(dbosRuntime)

    // 3. Setup storage
    contentReader := storage.NewHTTPContentReader("http://localhost:4000")
    derivedWriter := storage.NewHTTPDerivedWriter("http://localhost:4000")

    // 4. Register workflows
    thumbnailWorkflow := workflows.NewThumbnailWorkflow(contentReader, derivedWriter)
    runner.Register(pipeline.JobThumbnail, thumbnailWorkflow)

    // 5. Launch DBOS
    if err := dbosRuntime.Launch(); err != nil {
        log.Fatalf("Failed to launch: %v", err)
    }
    defer dbosRuntime.Shutdown(10 * time.Second)

    // 6. Trigger workflow
    runID, err := runner.RunAsync(context.Background(), pipeline.ProcessRequest{
        ContentID: "your-content-id",
        Job:       pipeline.JobThumbnail,
        Versions:  map[string]int{pipeline.DerivedTypeThumbnail: 1},
    })
    if err != nil {
        log.Fatalf("Failed to run workflow: %v", err)
    }

    log.Printf("Workflow queued: %s", runID)

    // 7. Check status
    time.Sleep(2 * time.Second)
    status, _ := runner.GetStatus(context.Background(), runID)
    log.Printf("Status: %s", status.State)
}
```

## Version Compatibility

- Go 1.21+
- PostgreSQL 12+
- DBOS Transact for Go v0.9.0+

## See Also

- [LIBRARY_USAGE.md](LIBRARY_USAGE.md) - Integration guide
- [README.md](README.md) - Project overview
- [examples/library-integration/](examples/library-integration/) - Working example
