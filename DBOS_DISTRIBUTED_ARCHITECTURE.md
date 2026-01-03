# DBOS Distributed Architecture

This document explains how `simple-content-pipeline` uses DBOS to enable distributed workflow execution across multiple components.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [The application_version Challenge](#the-application_version-challenge)
3. [Configuration Requirements](#configuration-requirements)
4. [Component Setup](#component-setup)
5. [How Workflows Flow](#how-workflows-flow)
6. [Troubleshooting](#troubleshooting)
7. [Production Deployment](#production-deployment)

---

## Architecture Overview

### The Problem

A typical content pipeline needs:
- **Upload handling** (Application server)
- **Image processing** (Go worker)
- **ML workloads** (Python worker)

Traditional approaches require tight coupling or complex message queue infrastructure.

### The DBOS Solution

DBOS (Database-Oriented Operating System) enables **distributed durable workflow execution** using only PostgreSQL:

```
┌─────────────┐
│Application  │──┐
│  (Client)   │  │
└─────────────┘  │
                 │   Enqueue Workflows
┌─────────────┐  │   ↓
│ Go Worker   │  ├──→ DBOS Queue (PostgreSQL)
│ (Executor)  │  │   ↓
└─────────────┘  │   Distribute to Workers
                 │
┌─────────────┐  │
│Python Worker│──┘
│ (Executor)  │
└─────────────┘
```

**Key Benefits:**
- **Durable**: Workflows survive crashes and restarts
- **Distributed**: Multiple workers share workload
- **Simple**: No Redis, Kafka, or RabbitMQ needed
- **Transactional**: Workflow state stored in PostgreSQL

---

## The application_version Challenge

### Default Behavior (Doesn't Work)

By default, DBOS generates a **SHA-256 hash of each binary** as its `application_version`:

```bash
# Application binary
application_version: "a1b2c3d4..."  (hash of your-app binary)

# Go worker binary
application_version: "e5f6g7h8..."  (hash of pipeline-worker binary)

# Python worker
application_version: "i9j0k1l2..."  (hash of python interpreter + code)
```

**Problem:** DBOS only routes workflows between components with **matching application_version**.

Result: `workflow not found` errors when workers try to execute workflows enqueued by your application.

### The Solution: Custom Shared Version

Override DBOS's default hash with a **custom shared version string**:

```bash
# All components use the SAME version
export DBOS_APPLICATION_VERSION=my-app-v1
```

Now all components share workflows:
```bash
# Your application
application_version: "my-app-v1"  ✓

# Go worker
application_version: "my-app-v1"  ✓

# Python worker
application_version: "my-app-v1"  ✓
```

**Result:** Workflows flow seamlessly between components.

---

## Configuration Requirements

### Critical Environment Variables

**All components MUST have matching values:**

```bash
# Content pipeline application version (shared)
# Used by: Your application, Go worker, Python worker
DBOS_APPLICATION_VERSION=my-app-v1  # MUST MATCH

# PostgreSQL connection (shared)
DBOS_SYSTEM_DATABASE_URL=postgres://user:pwd@localhost:5432/mydb?sslmode=disable

# Queue name (shared)
DBOS_QUEUE_NAME=default
```

### Component-Specific Settings

**Your Application (.env):**
```bash
DBOS_APPLICATION_VERSION=my-app-v1
DBOS_SYSTEM_DATABASE_URL=postgres://user:pwd@localhost:5432/mydb?sslmode=disable
DBOS_QUEUE_NAME=default
PIPELINE_ENABLED=true
CONTENT_API_URL=http://localhost:8080
```

**Go Worker (.env):**
```bash
DBOS_APPLICATION_VERSION=my-app-v1
DBOS_SYSTEM_DATABASE_URL=postgres://user:pwd@localhost:5432/mydb?sslmode=disable
DBOS_QUEUE_NAME=default
DBOS_QUEUE_CONCURRENCY=4
CONTENT_API_URL=http://localhost:8080
```

**Python Worker (.env):**
```bash
DBOS_APPLICATION_VERSION=my-app-v1
DBOS_SYSTEM_DATABASE_URL=postgres://user:pwd@localhost:5432/mydb?sslmode=disable
DBOS_QUEUE_NAME=default
CONTENT_API_URL=http://localhost:8080
```

---

## Component Setup

### 1. Application Server (Client Mode)

**Purpose:** Enqueue workflows, do not execute them

**Code Example:**
```go
import "github.com/tendant/simple-content-pipeline/pkg/runner"

appVersion := os.Getenv("DBOS_APPLICATION_VERSION")

// Client mode: Concurrency = 0 (no execution)
client, err := runner.NewClient(runner.Config{
    DatabaseURL:        dbURL,
    AppName:            "content-pipeline",
    QueueName:          "default",
    ApplicationVersion: appVersion,  // Shared version
})

// Enqueue thumbnail workflow
runID, _ := client.RunThumbnail(ctx, contentID, 300, 300)
log.Printf("Enqueued: %s", runID)
```

**Logs:**
```
✓ DBOS pipeline client initialized
  Database: postgres://user:pwd@localhost:5432/mydb?sslmode=disable
  Queue: default
  Mode: Client (enqueue only, workers execute)
```

### 2. Go Worker (Executor Mode)

**Purpose:** Execute thumbnail workflows

**Code Example:**
```go
import (
    "github.com/tendant/simple-content-pipeline/internal/dbosruntime"
    "github.com/tendant/simple-content-pipeline/internal/workflows"
)

appVersion := os.Getenv("DBOS_APPLICATION_VERSION")

// Executor mode: Concurrency > 0
dbosRuntime, _ := dbosruntime.NewRuntime(ctx, dbosruntime.Config{
    DatabaseURL:        dbURL,
    AppName:            "content-pipeline",
    QueueName:          "default",
    Concurrency:        4,  // Execute 4 workflows concurrently
    ApplicationVersion: appVersion,  // Shared version
})

// Register workflow
runner := workflows.NewWorkflowRunner(dbosRuntime)
runner.Register("thumbnail", thumbnailWorkflow)

// Launch worker (blocks, processes queue)
dbosRuntime.Launch()
```

**Logs:**
```
✓ Registered workflow: ThumbnailWorkflow for job: thumbnail
✓ DBOS runtime initialized
  Database: postgres://user:pwd@localhost:5432/mydb?sslmode=disable
  Queue: default
  Concurrency: 4
[thumbnail-abc123] Starting thumbnail workflow
[thumbnail-abc123] Thumbnail workflow completed successfully
```

### 3. Python Worker (Executor Mode)

**Purpose:** Execute ML workflows

**Code Example:**
```python
import os
from dbos import DBOS

app_version = os.getenv('DBOS_APPLICATION_VERSION', '')

DBOS(
    config={
        'name': 'content-pipeline',  # Shared app name
        'system_database_url': dbos_db_url,
        'application_version': app_version,  # Shared version
    }
)

@DBOS.workflow()
def detect_objects_workflow(content_id: str):
    # ML processing logic
    pass

if __name__ == '__main__':
    DBOS.launch()  # Start worker
```

**Logs:**
```
✓ Registered workflow: ObjectDetectionWorkflow for job: object_detection
Python ML worker ready to process workflows from queue: default
[object_detection-xyz789] Starting object detection workflow
[object_detection-xyz789] Workflow completed
```

---

## How Workflows Flow

### End-to-End Example: Thumbnail Generation

**1. User uploads image to your application:**
```bash
curl -X POST http://localhost:8080/api/v1/content \
  -F "file=@cat.jpg"
```

**2. Application enqueues thumbnail workflows:**
```go
// Application code
for _, size := range []struct{w, h int}{
    {300, 300},   // thumbnail
    {800, 600},   // preview
    {1920, 1080}, // full
} {
    runID, _ := pipelineClient.RunThumbnail(ctx, contentID, size.w, size.h)
    log.Printf("✓ Thumbnail workflow enqueued: %s", runID)
}
```

**Application Log:**
```
✓ Thumbnail workflow enqueued: thumbnail-abc123-300x300
✓ Thumbnail workflow enqueued: thumbnail-abc123-800x600
✓ Thumbnail workflow enqueued: thumbnail-abc123-1920x1080
```

**3. Go worker picks up from DBOS queue:**

DBOS query (automatic):
```sql
SELECT workflow_uuid, workflow_inputs
FROM dbos.workflow_status
WHERE status = 'PENDING'
  AND application_version = 'my-app-v1'
  AND queue_name = 'default'
LIMIT 1
FOR UPDATE SKIP LOCKED;
```

**4. Go worker executes workflow:**
```go
// Workflow steps:
// 1. Download source content from simple-content API
// 2. Generate thumbnail
// 3. Upload derived content back to simple-content API
```

**Go Worker Log:**
```
[thumbnail-abc123-300x300] Starting thumbnail workflow for content_id=abc123
[thumbnail-abc123-300x300] Source content downloaded successfully
[thumbnail-abc123-300x300] Thumbnail generated: 300x300
[thumbnail-abc123-300x300] Derived content written: derived-xyz789
[thumbnail-abc123-300x300] Thumbnail workflow completed successfully
```

**5. DBOS marks workflow complete:**
```sql
UPDATE dbos.workflow_status
SET status = 'SUCCESS', updated_at = NOW()
WHERE workflow_uuid = 'thumbnail-abc123-300x300';
```

**6. User retrieves processed content:**
```bash
# Get content info (includes derived content)
curl http://localhost:8080/api/v1/contents/abc123

# Download thumbnail
curl http://localhost:8080/api/v1/contents/derived-xyz789/download
```

---

## Troubleshooting

### Issue 1: "workflow not found"

**Symptom:**
```
Error executing workflow: workflow not found: ThumbnailWorkflow
application_version mismatch
```

**Diagnosis:**
```bash
# Check DBOS database for version mismatches
psql -U user -d mydb -c "
  SELECT DISTINCT application_version, COUNT(*)
  FROM dbos.workflow_status
  GROUP BY application_version;
"

# Output (BAD):
# a1b2c3d4...  | 3   (Application binary hash)
# e5f6g7h8...  | 0   (Go worker hash)
```

**Fix:**
```bash
# 1. Add to all .env files
echo "DBOS_APPLICATION_VERSION=my-app-v1" >> your-application/.env
echo "DBOS_APPLICATION_VERSION=my-app-v1" >> simple-content-pipeline/.env
echo "DBOS_APPLICATION_VERSION=my-app-v1" >> simple-content-pipeline/python-worker/.env

# 2. Restart all components
killall your-app pipeline-worker python
./your-app &
./pipeline-worker &
python main.py &

# 3. Verify (should show only one version)
psql -U user -d mydb -c "
  SELECT DISTINCT application_version
  FROM dbos.workflow_status;
"

# Output (GOOD):
# my-app-v1
```

### Issue 2: Workers not picking up workflows

**Symptom:**
- Application enqueues workflows (logs show "enqueued")
- Workers are running but idle
- Workflows stuck in PENDING state

**Diagnosis:**
```bash
# Check queue depth
psql -U user -d mydb -c "
  SELECT status, COUNT(*)
  FROM dbos.workflow_status
  WHERE application_version = 'my-app-v1'
  GROUP BY status;
"

# Output:
# PENDING  | 50   (stuck workflows)
# SUCCESS  | 10
```

**Possible Causes:**
1. **Different queue names**
   ```bash
   # Application: DBOS_QUEUE_NAME=default
   # Worker: DBOS_QUEUE_NAME=production  ← WRONG
   ```

2. **Different databases**
   ```bash
   # Application: localhost:5432/mydb
   # Worker: localhost:5432/worker_db  ← WRONG
   ```

3. **Worker concurrency = 0**
   ```bash
   # Worker .env
   DBOS_QUEUE_CONCURRENCY=0  ← Client mode, won't execute
   ```

**Fix:**
```bash
# Ensure matching configuration
grep DBOS_QUEUE_NAME */.*env
grep DBOS_SYSTEM_DATABASE_URL */.*env
grep DBOS_QUEUE_CONCURRENCY simple-content-pipeline/.env  # Should be > 0
```

### Issue 3: PostgreSQL connection errors

**Symptom:**
```
Failed to connect to DBOS system database
dial tcp: connect: connection refused
```

**Diagnosis:**
```bash
# Test PostgreSQL connection
psql "$DBOS_SYSTEM_DATABASE_URL" -c "SELECT 1"

# Check if DBOS schema exists
psql -U user -d mydb -c "\dn" | grep dbos
```

**Fix:**
```bash
# Start PostgreSQL if not running
pg_ctl start -D /usr/local/var/postgres

# Create DBOS schema (automatic on first launch)
# Or manually:
psql -U user -d mydb -c "CREATE SCHEMA IF NOT EXISTS dbos;"
```

---

## Production Deployment

### Deployment Architecture

```
                    ┌─────────────────────┐
                    │   Load Balancer     │
                    └──────────┬──────────┘
                               │
                ┌──────────────┼──────────────┐
                ↓              ↓              ↓
         ┌───────────┐  ┌───────────┐  ┌───────────┐
         │   App 1   │  │   App 2   │  │   App 3   │
         │ (Client)  │  │ (Client)  │  │ (Client)  │
         └─────┬─────┘  └─────┬─────┘  └─────┬─────┘
               │              │              │
               └──────────────┼──────────────┘
                              │
                    ┌─────────▼─────────┐
                    │  DBOS (PostgreSQL)│
                    │  (Managed RDS)    │
                    └─────────┬─────────┘
                              │
               ┌──────────────┼──────────────┬──────────────┐
               ↓              ↓              ↓              ↓
        ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐
        │Go Worker 1│  │Go Worker 2│  │Python ML 1│  │Python ML 2│
        │(Executor) │  │(Executor) │  │(Executor) │  │(Executor) │
        └───────────┘  └───────────┘  └───────────┘  └───────────┘
        CPU optimized  CPU optimized  GPU instance   GPU instance
```

### Configuration Management

**Use environment-specific configs:**

```bash
# .env.production
DBOS_APPLICATION_VERSION=my-app-v1  # Consistent across all
DBOS_SYSTEM_DATABASE_URL=postgres://user:pass@rds.internal/mydb?sslmode=require
DBOS_QUEUE_NAME=production
CONTENT_API_URL=http://api.internal:8080

# .env.staging
DBOS_APPLICATION_VERSION=my-app-v1-staging  # Different from prod
DBOS_SYSTEM_DATABASE_URL=postgres://user:pass@staging-db.internal/mydb
DBOS_QUEUE_NAME=staging
```

### Scaling Strategies

**Horizontal Scaling:**
```bash
# Run multiple Go workers (no coordination needed)
# Each picks up work from shared DBOS queue

# Server 1:
./pipeline-worker &

# Server 2:
./pipeline-worker &

# Server 3:
./pipeline-worker &

# DBOS automatically distributes work via PostgreSQL row locking
```

**Concurrency Tuning:**
```bash
# Thumbnail workers (CPU-bound)
DBOS_QUEUE_CONCURRENCY=8  # Process 8 images concurrently

# ML workers (GPU-bound)
DBOS_QUEUE_CONCURRENCY=2  # Limit to GPU capacity
```

### Monitoring

**Metrics to Track:**

1. **Queue Depth** (workflows waiting)
   ```sql
   SELECT COUNT(*) FROM dbos.workflow_status
   WHERE status = 'PENDING'
   AND application_version = 'my-app-v1';
   ```

2. **Workflow Success Rate**
   ```sql
   SELECT
     status,
     COUNT(*) * 100.0 / SUM(COUNT(*)) OVER () AS percentage
   FROM dbos.workflow_status
   WHERE application_version = 'my-app-v1'
   GROUP BY status;
   ```

3. **Processing Latency**
   ```sql
   SELECT
     name,
     AVG(EXTRACT(EPOCH FROM (updated_at - created_at))) AS avg_seconds
   FROM dbos.workflow_status
   WHERE status = 'SUCCESS'
   AND application_version = 'my-app-v1'
   GROUP BY name;
   ```

### Health Checks

**Application API:**
```bash
# Check DBOS connection
curl http://localhost:8080/health/dbos
# {"status": "ok", "version": "my-app-v1"}
```

**Workers:**
```bash
# Check worker is processing
tail -f pipeline-worker.log | grep "completed successfully"

# Check PostgreSQL connectivity
psql "$DBOS_SYSTEM_DATABASE_URL" -c "SELECT 1"
```

### Disaster Recovery

**Workflow Durability:**
- DBOS stores all workflow state in PostgreSQL
- Workers can crash and restart without losing workflows
- In-flight workflows resume from last checkpoint

**Recovery Procedure:**
```bash
# 1. Worker crashes
# 2. PostgreSQL keeps workflow state
# 3. Restart worker with same config
./pipeline-worker

# 4. DBOS automatically resumes pending workflows
# Logs: "Recovering X workflows from previous run"
```

---

## Best Practices

1. **Always set DBOS_APPLICATION_VERSION** - Don't rely on binary hashes
2. **Use the same PostgreSQL instance** - DBOS and your application can share (different schemas)
3. **Monitor queue depth** - Alert if PENDING workflows grow too large
4. **Set appropriate concurrency** - Match to your hardware (CPU cores, GPU memory)
5. **Version your workflows** - Bump `application_version` when making breaking changes
6. **Use managed PostgreSQL in production** - RDS/Cloud SQL for reliability
7. **Implement health checks** - Detect stuck workers or database issues
8. **Log run_ids** - Track workflows end-to-end across components

---

## See Also

- [README.md](README.md) - Pipeline overview
- [LIBRARY_USAGE.md](LIBRARY_USAGE.md) - Integration guide
- [python-worker/README.md](python-worker/README.md) - Python ML worker setup
- [DBOS Documentation](https://docs.dbos.dev/) - Framework reference
