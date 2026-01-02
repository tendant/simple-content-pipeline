# Application Hook Example

This example demonstrates how to automatically trigger workflows when content is uploaded using application-level hooks.

## What This Shows

1. **Upload Handler** - Accepts file uploads
2. **Automatic Hook** - Detects content type and triggers appropriate workflows
3. **Background Processing** - Workflows run asynchronously, don't block upload response
4. **Conditional Logic** - Different workflows for different content types

## Architecture

```
User Upload
    ↓
┌─────────────────────┐
│  Upload Handler     │
│                     │
│  1. Save file       │
│  2. Trigger hooks   │ ← Hook function called
│  3. Return response │
└─────────────────────┘
    ↓ (goroutine)
┌─────────────────────┐
│  Hook Function      │
│                     │
│  - Check type       │
│  - Trigger workflow │
└─────────────────────┘
    ↓
┌─────────────────────┐
│  DBOS Workflow      │
│  (Background)       │
└─────────────────────┘
```

## Running the Example

### Prerequisites

```bash
# Start PostgreSQL
cd ../../..
make postgres-start
```

### Run the application

```bash
cd examples/hooks/application-hook
go run main.go
```

### Test file upload

```bash
# Create a test image
echo "Test image data" > test-image.jpg

# Upload it
curl -X POST http://localhost:8080/api/upload \
  -F "file=@test-image.jpg" \
  -F "name=My Test Image"
```

**Response:**
```json
{
  "content_id": "abc123...",
  "status": "uploaded",
  "document_type": "image/jpeg",
  "size": 16,
  "message": "Content uploaded, processing workflows triggered"
}
```

**Server logs:**
```
Uploading: My Test Image (image/jpeg, 16 bytes)
✓ Content uploaded: abc123...
🎣 HOOK: Checking workflows for content_id=abc123..., type=image/jpeg
🎣 HOOK: Image detected, triggering thumbnail workflow
🎣 HOOK SUCCESS: Thumbnail workflow queued (run_id=thumbnail-abc123-...)
```

## How It Works

### 1. Upload Handler

```go
func (app *Application) handleUpload(w http.ResponseWriter, r *http.Request) {
    // Upload file to storage
    content, _ := app.contentService.UploadContent(ctx, uploadReq)

    // Trigger hooks in background (doesn't block response)
    go app.triggerWorkflowHooks(content.ID.String(), contentType)

    // Return immediately
    json.NewEncoder(w).Encode(response)
}
```

### 2. Hook Function

```go
func (app *Application) triggerWorkflowHooks(contentID, contentType string) {
    // Conditional workflow triggering
    if isImage(contentType) {
        app.pipelineRunner.RunAsync(ctx, pipeline.ProcessRequest{
            ContentID: contentID,
            Job:       pipeline.JobThumbnail,
            Versions:  map[string]int{"thumbnail": 1},
        })
    }

    if isDocument(contentType) {
        // Trigger OCR workflow
    }

    // Add more hooks as needed
}
```

## Customizing Hooks

### Add New Hook for PDFs

```go
func (app *Application) triggerWorkflowHooks(contentID, contentType string) {
    // Existing hooks...

    // New hook for PDFs
    if isPDF(contentType) {
        app.pipelineRunner.RunAsync(ctx, pipeline.ProcessRequest{
            ContentID: contentID,
            Job:       "pdf-extract",
            Versions:  map[string]int{"pdf_text": 1},
        })
    }
}
```

### Hook Based on File Size

```go
func (app *Application) triggerWorkflowHooks(contentID, contentType string, fileSize int64) {
    if isImage(contentType) && fileSize > 1<<20 { // > 1MB
        // Trigger compression workflow
        app.pipelineRunner.RunAsync(ctx, pipeline.ProcessRequest{
            ContentID: contentID,
            Job:       "compress",
            Metadata:  map[string]string{"target_size": "500kb"},
        })
    }
}
```

### Hook Based on User Preferences

```go
func (app *Application) triggerWorkflowHooks(contentID string, user User) {
    // Check user preferences
    if user.Settings.AutoGenerateThumbnails {
        app.pipelineRunner.RunAsync(ctx, thumbnailRequest)
    }

    if user.Settings.EnableOCR {
        app.pipelineRunner.RunAsync(ctx, ocrRequest)
    }
}
```

## Hook Configuration

### Feature Flags

```go
type HookConfig struct {
    EnableThumbnails bool
    EnableOCR        bool
    EnableCompression bool
}

func (app *Application) triggerWorkflowHooks(contentID, contentType string) {
    if app.config.EnableThumbnails && isImage(contentType) {
        // Trigger thumbnail
    }

    if app.config.EnableOCR && isDocument(contentType) {
        // Trigger OCR
    }
}
```

### Environment Variables

```bash
# Enable/disable hooks
AUTO_THUMBNAILS=true
AUTO_OCR=false
AUTO_COMPRESSION=true
```

## Error Handling

### Graceful Degradation

```go
func (app *Application) triggerWorkflowHooks(contentID, contentType string) {
    // Don't fail upload if hook fails
    defer func() {
        if r := recover(); r != nil {
            log.Printf("Hook panic recovered: %v", r)
        }
    }()

    runID, err := app.pipelineRunner.RunAsync(ctx, req)
    if err != nil {
        // Log error but don't fail the upload
        log.Printf("Hook failed: %v", err)
        // Optional: Send to error tracking
        return
    }

    log.Printf("Hook success: %s", runID)
}
```

### Retry Logic

```go
func (app *Application) triggerWorkflowHooks(contentID, contentType string) {
    const maxRetries = 3

    for i := 0; i < maxRetries; i++ {
        runID, err := app.pipelineRunner.RunAsync(ctx, req)
        if err == nil {
            log.Printf("Hook success on attempt %d: %s", i+1, runID)
            return
        }

        log.Printf("Hook attempt %d failed: %v", i+1, err)
        time.Sleep(time.Second * time.Duration(i+1))
    }

    log.Printf("Hook failed after %d attempts", maxRetries)
}
```

## Monitoring

### Log All Hook Executions

```go
func (app *Application) triggerWorkflowHooks(contentID, contentType string) {
    start := time.Now()

    log.Printf("HOOK_START: content_id=%s type=%s", contentID, contentType)

    runID, err := app.pipelineRunner.RunAsync(ctx, req)

    duration := time.Since(start)

    if err != nil {
        log.Printf("HOOK_ERROR: content_id=%s duration=%v error=%v",
            contentID, duration, err)
    } else {
        log.Printf("HOOK_SUCCESS: content_id=%s run_id=%s duration=%v",
            contentID, runID, duration)
    }
}
```

### Metrics

```go
var hooksTriggered = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "app_hooks_triggered_total",
        Help: "Workflow hooks triggered",
    },
    []string{"content_type", "workflow"},
)

func (app *Application) triggerWorkflowHooks(contentID, contentType string) {
    if isImage(contentType) {
        app.pipelineRunner.RunAsync(ctx, req)
        hooksTriggered.WithLabelValues(contentType, "thumbnail").Inc()
    }
}
```

## Testing

### Unit Test

```go
func TestUploadTriggersHook(t *testing.T) {
    // Mock workflow runner
    mockRunner := &MockWorkflowRunner{}
    app := &Application{pipelineRunner: mockRunner}

    // Simulate upload
    app.triggerWorkflowHooks("test-id", "image/jpeg")

    // Verify hook was called
    assert.Equal(t, 1, mockRunner.CallCount)
    assert.Equal(t, "thumbnail", mockRunner.LastJob)
}
```

### Integration Test

```go
func TestEndToEnd(t *testing.T) {
    // Start application
    app := startTestApp(t)
    defer app.cleanup()

    // Upload file
    resp := uploadFile(t, "test.jpg")
    assert.Equal(t, 201, resp.StatusCode)

    // Wait for hook
    time.Sleep(time.Second)

    // Verify workflow was triggered
    workflows := app.pipelineRunner.ListRunning()
    assert.Contains(t, workflows, "thumbnail")
}
```

## Advantages

✅ **Simple** - Easy to understand and implement
✅ **Direct** - No additional infrastructure needed
✅ **Fast** - Immediate triggering after upload
✅ **Flexible** - Easy to add/remove hooks
✅ **Testable** - Can unit test hook logic

## When to Use

- Single application architecture
- Simple workflow triggering logic
- Quick prototyping / MVP
- No need for event replay
- Tight coupling is acceptable

## When NOT to Use

- Multiple services need to react to uploads
- Need event replay capabilities
- Want to decouple upload from processing
- Need to scale upload and processing independently

For those cases, see [event-driven example](../event-driven/).

## See Also

- [HOOKS.md](../../../HOOKS.md) - Complete hooks guide
- [event-driven example](../event-driven/) - Event-driven approach
- [LIBRARY_USAGE.md](../../../LIBRARY_USAGE.md) - Library integration
