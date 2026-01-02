# simple-content-pipeline (Go + DBOS)

**Goal:** Build a reusable content-processing pipeline where the *application embeds the trigger* and a *separate worker binary executes jobs* using DBOS. Duplicate processing is allowed but must be detectable.

---

## 1. High-Level Architecture

### Components

1. **Application (Uploader / API)**

   - Handles uploads and metadata persistence
   - Optionally computes `content_hash`
   - Triggers processing via HTTP client
   - Does **not** run DBOS workers

2. **Pipeline Worker (DBOS)**

   - Runs DBOS runtime and workers
   - Owns workflow definitions and processing logic
   - Reads/writes artifacts from storage (S3 / MinIO / FS)
   - Detects duplicate processing via Postgres ledger

---

## 2. Quick Start

### As a Standalone Service

```bash
# Start PostgreSQL
make postgres-start

# Run the all-in-one server
./pipeline-standalone

# Test it
./test-upload.sh
```

See [examples/upload-test/README.md](examples/upload-test/README.md) for more testing options.

### As a Library in Your Application

```bash
go get github.com/tendant/simple-content-pipeline
```

```go
import (
    "github.com/tendant/simple-content-pipeline/internal/dbosruntime"
    "github.com/tendant/simple-content-pipeline/internal/workflows"
    "github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// Initialize pipeline
dbosRuntime, _ := dbosruntime.NewRuntime(ctx, dbosruntime.Config{
    DatabaseURL: "postgresql://...",
    AppName:     "my-app",
})

runner := workflows.NewWorkflowRunner(dbosRuntime)
runner.Register(pipeline.JobThumbnail, thumbnailWorkflow)
dbosRuntime.Launch()

// Trigger workflow
runID, _ := runner.RunAsync(ctx, pipeline.ProcessRequest{
    ContentID: "content-id",
    Job:       pipeline.JobThumbnail,
    Versions:  map[string]int{"thumbnail": 1},
})
```

**See:**
- [LIBRARY_USAGE.md](LIBRARY_USAGE.md) - Complete integration guide
- [API.md](API.md) - API reference
- [HOOKS.md](HOOKS.md) - Automatic workflow triggering
- [examples/library-integration/](examples/library-integration/) - Integration example
- [examples/hooks/](examples/hooks/) - Hook examples

---

## 3. Trigger Model

### Trigger Endpoint (Worker)

`POST /v1/process`

Request:

```json
{
  "content_id": "string",
  "object_key": "string",
  "content_hash": "string | null",
  "job": "thumbnail|ocr|object_detection",
  "versions": {
    "thumbnail": 1,
    "ocr_text": 2,
    "object_detections": 3
  },
  "metadata": {
    "mime": "image/jpeg",
    "size": 12345
  }
}
```

Notes:

- **Each derived type has its own version.** The worker reads the version it needs from `versions[derived_type]`.
- If a version is missing for a derived type, the worker should default to a configured value (e.g., `1`) or reject the request.

Response:

```json
{
  "run_id": "dbos-workflow-id",
  "dedupe_seen_count": 2
}
```

Notes:

- Trigger is **idempotent-safe** (duplicates allowed)
- App may retry freely

---

## 4. Duplicate Detection Strategy

### Summary

Duplicates are **allowed**. We detect duplicates primarily by **content\_id**. Each step is responsible for determining whether it should run (skip if a particular derived output for its **own derived-type version** already exists).

### Dedupe Key

Primary key for duplicate detection:

```
content_id
```

Optional enhancements later:

- include `job` if you want to track duplicate triggers per job type
- include `object_key` if a single `content_id` can point to multiple source objects over time

### Dedupe Table (Postgres)

`process_dedupe`

| column            | type        | notes                    |
| ----------------- | ----------- | ------------------------ |
| content\_id       | text (PK)   | unique                   |
| pipeline          | text        | optional                 |
| pipeline\_version | int         | optional                 |
| first\_seen\_at   | timestamptz |                          |
| last\_seen\_at    | timestamptz |                          |
| seen\_count       | int         | increments on duplicates |

Insert behavior:

- `INSERT ... ON CONFLICT DO UPDATE`
- Always allow workflow execution

\------|------|------| | dedupe\_key | text (PK) | unique | | content\_hash | text | | | pipeline | text | | | pipeline\_version | int | | | first\_seen\_at | timestamptz | | | last\_seen\_at | timestamptz | | | seen\_count | int | increments on duplicates |

Insert behavior:

- `INSERT ... ON CONFLICT DO UPDATE`
- Always allow workflow execution

---

## 5. Workflow Design (DBOS)

### Workflow Naming (job-specific)

Since content processing is split into **multiple independent jobs**, define **one workflow per job type**. This keeps workflows simple, composable, and independently triggerable.

Primary workflows:

- `ContentThumbnailWorkflow`
- `ContentOCRWorkflow`
- `ContentObjectDetectionWorkflow`

Each workflow:

- is keyed by `content_id`
- allows duplicate execution
- relies on step-level skip logic to avoid redundant work

---

### Workflow 1: `ContentThumbnailWorkflow`

Purpose: Generate preview thumbnails immediately after upload.

Steps:

1. **RecordDedupeStep**

   - Upsert `process_dedupe` using `content_id`

2. **GenerateThumbnailStep**

   - Calls `DerivedWriter.HasDerived(content_id, "thumbnail", derived_version)` where `derived_version = versions["thumbnail"]`
   - If exists → skip
   - Else → generate thumbnail and call `DerivedWriter.PutDerived(...)` using `derived_version = versions["thumbnail"]`

Derived outputs:

- `derived_type = "thumbnail"`
- payload: thumbnail image bytes + metadata (dimensions, format)

---

### Workflow 2: `ContentOCRWorkflow`

Purpose: Extract text from content (PDFs, images) asynchronously.

Steps:

1. **RecordDedupeStep**

   - Upsert `process_dedupe` using `content_id`

2. **ExtractOCRTextStep**

   - Calls `DerivedWriter.HasDerived(content_id, "ocr_text", derived_version)` where `derived_version = versions["ocr_text"]`
   - If exists → skip
   - Else → run OCR and call `DerivedWriter.PutDerived(...)` using `derived_version = versions["ocr_text"]`

Derived outputs:

- `derived_type = "ocr_text"`
- payload: OCR JSON/text + metadata (engine, language, confidence)

---

### Workflow 3: `ContentObjectDetectionWorkflow`

Purpose: Run object detection / vision models on content.

Steps:

1. **RecordDedupeStep**

   - Upsert `process_dedupe` using `content_id`

2. **RunObjectDetectionStep**

   - Calls `DerivedWriter.HasDerived(content_id, "object_detections", derived_version)` where `derived_version = versions["object_detections"]`
   - If exists → skip
   - Else → run detection and call `DerivedWriter.PutDerived(...)` using `derived_version = versions["object_detections"]`

Derived outputs:

- `derived_type = "object_detections"`
- payload: detections JSON + metadata (model, threshold)

---

### Step-level “Should I run?” (common to all workflows)

Each step must be self-idempotent:

- Check if its expected artifact already exists
- Optionally verify metadata/version compatibility
- Skip safely if output is already present

This enables:

- duplicate triggers
- independent job retries
- manual or scheduled reprocessing

---

## 6. Derived Content & Artifact Management (via `simple-content`)

**Decision:** The pipeline worker does **not** write artifacts directly to storage paths. Instead, it calls the `simple-content` library/service to create and manage **Derived Content** records and storage.

### Derived Content Concept

Each workflow produces one or more **derived content objects** (e.g., thumbnail image, OCR JSON, detections JSON). The worker submits derived outputs to `simple-content`, which:

- stores the derived blob in the configured backend storage
- records metadata and lineage (source `content_id` → derived content)
- returns identifiers/keys for later retrieval

### Derived Types (initial)

- `thumbnail`
- `ocr_text`
- `object_detections`

### Versioning (per derived type)

**Each derived type has its own independent version.** For example:

- `thumbnail` v1
- `ocr_text` v2
- `object_detections` v3

The worker must read the version for the derived output it is producing (e.g., `versions["thumbnail"]`).

### Step-level “Should I run?” checks

Since storage details are owned by `simple-content`, steps decide whether to run by querying `simple-content` for existing derived outputs:

- `HasDerived(content_id, derived_type, derived_version)` → bool

If a derived output for that **derived\_type + derived\_version** exists → **skip**.

### Deterministic identity (recommended)

To support cheap dedupe and safe retries, derived content creation should include a deterministic identity such as:

- uniqueness on `(content_id, derived_type, derived_version)`

This can be enforced in `simple-content` (unique index) or implemented as an upsert.

---

## 7. Repository Layout

```
simple-content-pipeline/
├── pkg/
│   ├── client/           # HTTP client used by app
│   │   ├── client.go
│   │   └── types.go
│   └── pipeline/         # shared types
│       └── types.go
├── cmd/
│   └── pipeline-worker/  # DBOS worker binary
│       └── main.go
├── internal/
│   ├── workflows/
│   │   └── process_content.go
│   ├── steps/
│   │   ├── hash.go
│   │   ├── dedupe.go
│   │   └── image_v1.go
│   ├── storage/
│   │   ├── storage.go
│   │   ├── s3.go
│   │   └── filesystem.go
│   └── db/
│       └── migrations/
└── README.md
```

---

## 8. Public Go Interfaces

### Pipeline Client (used by app)

```go
type Client interface {
    Process(ctx context.Context, req ProcessRequest) (ProcessResponse, error)
}
```

### Storage Adapter

The worker should not depend on raw storage paths for derived artifacts. Keep storage adapters limited to **reading the source object** (unless your OCR/detection pipelines also require writing temporary blobs).

```go
type Storage interface {
    GetReader(ctx context.Context, key string) (io.ReadCloser, error)
}
```

### Derived Content Writer (via `simple-content`)

The worker writes outputs through `simple-content` as **Derived Content**:

```go
type DerivedWriter interface {
    // Returns true if a derived output already exists for the given type/version.
    HasDerived(ctx context.Context, contentID string, derivedType string, derivedVersion int) (bool, error)

    // Creates or upserts a derived output and returns its derived content ID.
    PutDerived(ctx context.Context, contentID string, derivedType string, derivedVersion int, r io.Reader, meta map[string]string) (derivedID string, err error)
}
```

---

## 9. Worker Runtime Configuration

Environment variables:

- `DATABASE_URL`
- `PIPELINE_HTTP_ADDR=:8080`
- `STORAGE_DRIVER=s3|fs`
- `STORAGE_BUCKET=...`
- `WORKER_CONCURRENCY=4`

---

## 10. Deployment Model

- App deployment:

  - Uploads content
  - Calls pipeline worker via HTTP

- Worker deployment:

  - Stateless replicas
  - Horizontal scaling supported
  - DBOS ensures workflow durability

Same container image can be reused across environments.

---

## 11. Milestones

### M1 — Skeleton ✅ COMPLETE

- ✅ Worker binary boots HTTP server
- ✅ `/v1/process` endpoint accepts requests
- ✅ HTTP client library (`pkg/client`)
- ✅ Shared types (`pkg/pipeline`)
- ✅ Basic project structure
- ⏸️ DBOS integration (deferred to M2)

### M2 — Dedupe Ledger (DEFERRED)

- ⏸️ Add `process_dedupe` migration (deferred - no DB tracking for now)
- ⏸️ Implement RecordDedupe step (deferred)

### M3 — Storage + Artifacts ✅ COMPLETE

- ✅ Filesystem storage reader (`internal/storage`)
- ✅ Storage interface (`Reader`, `ReaderWithMetadata`)
- ✅ Path traversal security checks
- ⏸️ S3 storage reader (deferred)
- ⏸️ Integration with simple-content for derived outputs (next)

### M4 — First Pipeline (Thumbnail) ✅ COMPLETE

- ✅ Workflow runner framework
- ✅ Thumbnail workflow implementation
- ✅ Request validation
- ✅ Source content verification via simple-content
- ✅ **Integration with simple-content via HTTP API**
  - ✅ HTTPContentReader for downloads
  - ✅ HTTPDerivedWriter for uploads
  - ✅ Supports both HTTP and embedded modes
- ✅ Derived content persistence via simple-content
- ⏸️ Actual image thumbnail generation (placeholder - using source as-is)

### M5 — App Integration ✅ COMPLETE

- ✅ HTTP API architecture (pipeline worker connects to simple-content server)
- ✅ App uploads content to simple-content
- ✅ App triggers pipeline via HTTP POST
- ✅ Pipeline processes and stores derived content back to simple-content
- ⏸️ Store `run_id` with content metadata (future enhancement)

---

## 12. Design Principles

- Build once, reuse everywhere
- DBOS is an implementation detail
- Duplicate processing is **observable**, not forbidden
- **content\_id-based** dedupe with `seen_count` for visibility
- Step-level skip logic (artifact existence and/or step state) prevents wasted compute
- Deterministic outputs > complex locking
- One clear trigger contract across all projects

---

## 13. Architecture Notes

### simple-content Integration

The pipeline worker uses `simple-content` library for all content operations:

- **ContentReader**: Reads source content via `simple-content.Service.DownloadContent()`
- **DerivedWriter**: Writes derived content via `simple-content.Service.UploadDerivedContent()`
- **No direct storage access**: All storage operations go through simple-content

**Architecture Modes:**

The pipeline worker supports two modes for accessing simple-content:

1. **HTTP API Mode** (recommended):
   ```
   App → simple-content HTTP Server
             ↑
   Pipeline Worker (HTTP client)
   ```
   - Set `CONTENT_API_URL=http://localhost:4000`
   - Worker uses HTTP client to call simple-content API
   - No shared database needed - all state in content service
   - **Best for production and testing**

2. **Embedded Mode** (development only):
   ```
   Pipeline Worker (embedded simple-content instance)
   ```
   - No `CONTENT_API_URL` set
   - Worker embeds simple-content library directly
   - Uses development preset (in-memory DB + filesystem storage)
   - **For quick testing of worker-only features**

**Storage:**
- simple-content handles all storage (database + files)
- Pipeline worker is stateless - just processes content via API

## 14. Deployment Modes & Setup

### Deployment Modes Comparison

| Mode | Setup Complexity | Use Case | Processes | Database | Storage |
|------|-----------------|----------|-----------|----------|---------|
| **Standalone** | ⭐ Easiest | Quick testing, development | 1 | In-memory | Filesystem |
| **HTTP API** | ⭐⭐ Medium | Production-like testing | 3 | PostgreSQL/In-memory | Any |

**Standalone Architecture:** Single process with embedded simple-content service
- `simple-content-pipeline/cmd/pipeline-standalone` (port 8080)
  - Embeds simple-content service (in-memory DB + filesystem storage)
  - Includes workflow execution
  - Zero external dependencies
  - Built-in test endpoint

**HTTP API Architecture:** Separate processes for content management and pipeline
- Process 1: `simple-content/cmd/server-configured` (port 4000)
- Process 2: `simple-content-pipeline/cmd/pipeline-worker` (port 8080)
- Process 3: Your application or test client

**Recommendation:** Start with **Standalone** for quick iteration (single process, no setup needed), move to **HTTP API** for production-like testing.

### Build and Run

```bash
# Build pipeline standalone (embedded mode)
cd simple-content-pipeline
go build -o pipeline-standalone ./cmd/pipeline-standalone

# Build HTTP-mode worker (for production-like testing)
go build -o pipeline-worker ./cmd/pipeline-worker
```

**Running Standalone Mode (Single Process):**
```bash
# Just one terminal needed!
cd simple-content-pipeline
./pipeline-standalone

# Or with custom settings:
./pipeline-standalone -port 8081 -data-dir ./my-data

# Or using go run:
go run ./cmd/pipeline-standalone
```

**Configuration:**

*For Standalone Mode:*
- `-port` / `PIPELINE_HTTP_ADDR` - HTTP address (default: `:8080`)
- `-data-dir` / `STORAGE_DIR` - Storage directory (default: `./dev-data`)

*For HTTP API Mode:*
- `CONTENT_API_URL` - simple-content HTTP API base URL
  - Example: `http://localhost:4000`

### Test with HTTP API Mode (Recommended)

```bash
# Terminal 1: Start simple-content server (in simple-content repo)
cd ../simple-content
go run ./cmd/server-configured

# Terminal 2: Start pipeline worker (connects to simple-content API)
cd ../simple-content-pipeline
CONTENT_API_URL=http://localhost:4000 go run ./cmd/pipeline-worker

# Terminal 3: Run example (uploads content and triggers pipeline)
go run ./examples/trigger/main.go
```

Expected output:
```
Step 1: Uploading content to simple-content...
✓ Content uploaded: c1acef84-e0a8-4f72-a75c-665ec1d5b8ed

Step 2: Triggering thumbnail generation...
✓ Processing triggered successfully
  Run ID: ed1f0628-740f-41c3-a938-e659eb4181f5
  Dedupe seen count: 0

Step 3: Checking derived content...
✓ Found 1 derived content(s):
  - Type: thumbnail, Variant: thumbnail_v1
```

### Test with Standalone Mode (Quickest - Single Process)

For the fastest testing with in-memory DB + filesystem storage (no database setup required):

```bash
# Just one terminal needed!
cd simple-content-pipeline
go run ./cmd/pipeline-standalone
# Server starts on port 8080 with embedded simple-content service

# Test the built-in endpoint:
curl http://localhost:8080/v1/test
```

**With custom settings:**
```bash
# Custom port and data directory
go run ./cmd/pipeline-standalone -port 9000 -data-dir /tmp/my-pipeline-data
```

Expected output:
```
=== Running End-to-End Test ===
Step 1: Uploading test content to simple-content...
✓ Content uploaded: <uuid> (status: uploaded)
Step 2: Triggering thumbnail generation...
✓ Workflow completed successfully (run_id: <uuid>)
Step 3: Checking derived content...
✓ Found 1 derived content(s)
  - Type: thumbnail, Variant: thumbnail_v1, Status: processed
=== Test Complete ===
```

**Architecture:**
```
┌───────────────────────────────────────────────┐
│ pipeline-standalone (port 8080)               │
│                                               │
│  ┌─────────────────────────────────────────┐ │
│  │ Embedded simple-content Service         │ │
│  │  - In-memory repository                 │ │
│  │  - Filesystem storage (./dev-data)      │ │
│  └─────────────────────────────────────────┘ │
│                                               │
│  ┌─────────────────────────────────────────┐ │
│  │ Pipeline Workflows                      │ │
│  │  - Thumbnail generation                 │ │
│  │  - Content processing                   │ │
│  └─────────────────────────────────────────┘ │
│                                               │
└───────────────────────────────────────────────┘
```

**Features:**
- **Single process**: Everything embedded (simple-content + pipeline)
- **Zero dependencies**: No database, no external services
- **Built-in test endpoint**: `/v1/test` for instant validation
- **Perfect for quick iteration**: Start with one command

**Configuration:**

*pipeline-standalone:*
- CLI: `-port <port>` - HTTP port (default: `8080`)
- CLI: `-data-dir <path>` - Storage directory (default: `./dev-data`)
- ENV: `PIPELINE_HTTP_ADDR` - HTTP address (overridden by -port flag)
- ENV: `STORAGE_DIR` - Storage directory (overridden by -data-dir flag)

**Priority:** CLI args > environment variables > defaults

### Using the Client Library

```go
import (
    "context"
    "github.com/tendant/simple-content-pipeline/pkg/client"
    "github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// Create client
c := client.New("http://localhost:8080")

// Trigger processing
req := pipeline.ProcessRequest{
    ContentID: "content-123",
    ObjectKey: "uploads/photo.jpg",
    Job:       pipeline.JobThumbnail,
    Versions: map[string]int{
        pipeline.DerivedTypeThumbnail: 1,
    },
    Metadata: map[string]string{
        "mime": "image/jpeg",
    },
}

resp, err := c.Process(context.Background(), req)
```

---

*This document is intended to be the canonical blueprint for **`simple-content-pipeline`**.*

