# Pipeline Testing Examples

This directory contains examples for testing the pipeline.

## Option 1: Quick Test (Standalone Mode)

The simplest way to test is using `pipeline-standalone` with its built-in test endpoint:

```bash
# Start the server
./pipeline-standalone

# Run test with bash script
./test-upload.sh

# Or with Go
cd examples/upload-test
go run main.go
```

This mode uses an embedded simple-content service, so everything runs in one process.

## Option 2: Production-Like Setup (Worker Mode)

For testing with actual file uploads, you need to run simple-content and pipeline-worker as separate processes:

### Terminal 1: Start simple-content server

```bash
cd ../simple-content
./standalone-server
# Runs on http://localhost:4000
```

### Terminal 2: Start pipeline-worker

```bash
cd simple-content-pipeline
./pipeline-worker
# Runs on http://localhost:8081
# Connects to simple-content at http://localhost:4000
```

### Terminal 3: Upload files and trigger processing

```bash
# Upload a file to simple-content
CONTENT_ID=$(curl -s -X POST http://localhost:4000/api/v1/contents \
  -H "Content-Type: application/json" \
  -d '{
    "owner_id": "00000000-0000-0000-0000-000000000001",
    "tenant_id": "00000000-0000-0000-0000-000000000002",
    "name": "My Image",
    "document_type": "image/jpeg",
    "file_name": "image.jpg",
    "data": "'$(base64 < image.jpg | tr -d '\n')'"
  }' | jq -r '.id')

echo "Content ID: $CONTENT_ID"

# Trigger thumbnail generation
RUN_ID=$(curl -s -X POST http://localhost:8081/v1/process \
  -H "Content-Type": application/json" \
  -d '{
    "content_id": "'"$CONTENT_ID"'",
    "job": "thumbnail",
    "versions": {"thumbnail": 1}
  }' | jq -r '.run_id')

echo "Run ID: $RUN_ID"

# Wait a moment
sleep 2

# Check derived content
curl -s "http://localhost:4000/api/v1/contents/$CONTENT_ID/derived" | jq .
```

## Workflow Diagrams

### Standalone Mode
```
┌───────────────────────┐
│  pipeline-standalone  │
│                       │
│  ┌─────────────────┐ │
│  │ simple-content  │ │
│  │   (embedded)    │ │
│  └─────────────────┘ │
│  ┌─────────────────┐ │
│  │ DBOS workflows  │ │
│  └─────────────────┘ │
└───────────────────────┘
     :8080
```

### Worker Mode (Production)
```
┌─────────────────┐         ┌──────────────────┐
│ simple-content  │         │ pipeline-worker  │
│   standalone    │◄────────┤                  │
│                 │ HTTP    │  ┌─────────────┐ │
│   :4000         │         │  │DBOS workflows│ │
└─────────────────┘         │  └─────────────┘ │
                            │      :8081       │
                            └──────────────────┘
```

## API Endpoints

### pipeline-standalone (:8080)
- `GET /health` - Health check
- `GET /v1/test` - Built-in end-to-end test
- `POST /v1/process` - Trigger workflow (requires content_id from embedded service)
- `GET /v1/runs/{id}` - Check workflow status

### pipeline-worker (:8081)
- `GET /health` - Health check
- `POST /v1/process` - Trigger workflow (requires content_id from external service)
- `GET /v1/runs/{id}` - Check workflow status

### simple-content standalone (:4000)
- `POST /api/v1/contents` - Upload content
- `GET /api/v1/contents/{id}` - Get content metadata
- `GET /api/v1/contents/{id}/download` - Download content
- `GET /api/v1/contents/{id}/derived` - List derived content
- `GET /api/v1/test` - Built-in test

## Testing Tips

1. **Use standalone mode for development** - Single process, easier debugging
2. **Use worker mode for production testing** - Separate concerns, more realistic
3. **Check PostgreSQL** - DBOS persists workflow state in the database
4. **Monitor logs** - Both services log workflow execution details
