# Workflow Hooks Guide

This guide explains how to automatically trigger workflows using hooks and events.

## Overview

There are three main approaches to hook up workflows:

1. **Application Hooks** - Trigger workflows from your app's upload handlers
2. **Event-Driven Hooks** - React to events from simple-content service
3. **Webhook Integration** - External systems trigger workflows via HTTP

## 1. Application Hooks (Recommended)

The simplest approach: trigger workflows directly from your upload handler.

### Basic Pattern

```go
package main

import (
    "github.com/tendant/simple-content-pipeline/internal/workflows"
    "github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

type UploadHandler struct {
    contentService  simplecontent.Service
    pipelineRunner  *workflows.WorkflowRunner
}

// handleUpload uploads content and triggers processing
func (h *UploadHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
    // 1. Upload content to simple-content
    content, err := h.contentService.UploadContent(ctx, uploadReq)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }

    // 2. Trigger workflow hook (async - doesn't block response)
    go h.triggerWorkflows(content.ID.String(), content.DocumentType)

    // 3. Return response immediately
    json.NewEncoder(w).Encode(map[string]string{
        "content_id": content.ID.String(),
        "status":     "uploaded",
    })
}

// triggerWorkflows is the hook - runs in background
func (h *UploadHandler) triggerWorkflows(contentID, mimeType string) {
    // Determine which workflows to trigger based on content type
    if isImage(mimeType) {
        h.pipelineRunner.RunAsync(context.Background(), pipeline.ProcessRequest{
            ContentID: contentID,
            Job:       pipeline.JobThumbnail,
            Versions: map[string]int{
                pipeline.DerivedTypeThumbnail: 1,
            },
            Metadata: map[string]string{
                "mime": mimeType,
            },
        })
    }

    // Can trigger multiple workflows
    if needsOCR(mimeType) {
        h.pipelineRunner.RunAsync(context.Background(), pipeline.ProcessRequest{
            ContentID: contentID,
            Job:       "ocr",
            Versions:  map[string]int{"ocr_text": 1},
        })
    }
}

func isImage(mimeType string) bool {
    return strings.HasPrefix(mimeType, "image/")
}
```

### Complete Example

See [examples/hooks/application-hook/](examples/hooks/application-hook/) for a complete working example.

### Pros & Cons

✅ **Pros:**
- Simple and direct
- No additional infrastructure
- Easy to debug
- Works with any storage backend

❌ **Cons:**
- Tightly coupled to your application
- Need to deploy together
- Can't trigger workflows from other systems

## 2. Event-Driven Hooks

React to events emitted by simple-content service using an event bus or message queue.

### Architecture

```
┌─────────────────┐
│ simple-content  │
│                 │
│  Upload Event   │───┐
└─────────────────┘   │
                      │
                      v
              ┌───────────────┐
              │  Event Bus    │
              │  (Redis/NATS) │
              └───────┬───────┘
                      │
                      v
              ┌───────────────┐
              │ Event Handler │
              │ (Pipeline)    │
              └───────────────┘
```

### Using Redis Pub/Sub

```go
package main

import (
    "context"
    "encoding/json"
    "log"

    "github.com/redis/go-redis/v9"
    "github.com/tendant/simple-content-pipeline/internal/workflows"
    "github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// Event structure
type ContentUploadedEvent struct {
    ContentID    string `json:"content_id"`
    OwnerID      string `json:"owner_id"`
    DocumentType string `json:"document_type"`
    FileName     string `json:"file_name"`
}

// EventHandler listens for events and triggers workflows
type EventHandler struct {
    redis          *redis.Client
    pipelineRunner *workflows.WorkflowRunner
}

func NewEventHandler(redisURL string, runner *workflows.WorkflowRunner) *EventHandler {
    rdb := redis.NewClient(&redis.Options{
        Addr: redisURL,
    })

    return &EventHandler{
        redis:          rdb,
        pipelineRunner: runner,
    }
}

// Start listening for events
func (h *EventHandler) Start(ctx context.Context) error {
    pubsub := h.redis.Subscribe(ctx, "content:uploaded")
    defer pubsub.Close()

    log.Println("Listening for content upload events...")

    for {
        msg, err := pubsub.ReceiveMessage(ctx)
        if err != nil {
            return err
        }

        go h.handleEvent(ctx, msg.Payload)
    }
}

// handleEvent processes the event and triggers workflows
func (h *EventHandler) handleEvent(ctx context.Context, payload string) {
    var event ContentUploadedEvent
    if err := json.Unmarshal([]byte(payload), &event); err != nil {
        log.Printf("Failed to parse event: %v", err)
        return
    }

    log.Printf("Received upload event: %s", event.ContentID)

    // Trigger appropriate workflows based on content type
    if isImage(event.DocumentType) {
        runID, err := h.pipelineRunner.RunAsync(ctx, pipeline.ProcessRequest{
            ContentID: event.ContentID,
            Job:       pipeline.JobThumbnail,
            Versions: map[string]int{
                pipeline.DerivedTypeThumbnail: 1,
            },
            Metadata: map[string]string{
                "mime": event.DocumentType,
            },
        })
        if err != nil {
            log.Printf("Failed to trigger thumbnail workflow: %v", err)
            return
        }

        log.Printf("Triggered thumbnail workflow: %s", runID)
    }
}

// In your simple-content upload handler, publish events
func publishUploadEvent(rdb *redis.Client, content *Content) error {
    event := ContentUploadedEvent{
        ContentID:    content.ID.String(),
        OwnerID:      content.OwnerID.String(),
        DocumentType: content.DocumentType,
        FileName:     content.FileName,
    }

    data, _ := json.Marshal(event)
    return rdb.Publish(context.Background(), "content:uploaded", data).Err()
}
```

### Complete Example

See [examples/hooks/event-driven/](examples/hooks/event-driven/) for a complete working example with Redis.

### Pros & Cons

✅ **Pros:**
- Decoupled architecture
- Multiple consumers can react to same event
- Easy to add new workflows
- Can replay events

❌ **Cons:**
- Requires message broker (Redis/NATS/RabbitMQ)
- More complex infrastructure
- Need to handle event ordering/deduplication

## 3. Webhook Integration

Allow external systems to trigger workflows via HTTP webhooks.

### Webhook Handler

```go
package main

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/json"
    "io"
    "net/http"

    "github.com/tendant/simple-content-pipeline/internal/workflows"
    "github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

type WebhookHandler struct {
    pipelineRunner *workflows.WorkflowRunner
    webhookSecret  string
}

// WebhookPayload from external system
type WebhookPayload struct {
    Event     string `json:"event"`
    ContentID string `json:"content_id"`
    Metadata  map[string]string `json:"metadata"`
}

// handleWebhook processes webhook from external systems
func (h *WebhookHandler) handleWebhook(w http.ResponseWriter, r *http.Request) {
    // 1. Verify signature
    signature := r.Header.Get("X-Webhook-Signature")
    if !h.verifySignature(r.Body, signature) {
        http.Error(w, "Invalid signature", http.StatusUnauthorized)
        return
    }

    // 2. Parse payload
    var payload WebhookPayload
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        http.Error(w, "Invalid payload", http.StatusBadRequest)
        return
    }

    // 3. Trigger workflow based on event type
    if payload.Event == "content.uploaded" {
        runID, err := h.pipelineRunner.RunAsync(r.Context(), pipeline.ProcessRequest{
            ContentID: payload.ContentID,
            Job:       pipeline.JobThumbnail,
            Versions: map[string]int{
                pipeline.DerivedTypeThumbnail: 1,
            },
            Metadata: payload.Metadata,
        })
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        // 4. Return webhook response
        json.NewEncoder(w).Encode(map[string]string{
            "status": "accepted",
            "run_id": runID,
        })
    }
}

// verifySignature validates webhook authenticity
func (h *WebhookHandler) verifySignature(body io.Reader, signature string) bool {
    data, _ := io.ReadAll(body)

    mac := hmac.New(sha256.New, []byte(h.webhookSecret))
    mac.Write(data)
    expectedSignature := fmt.Sprintf("sha256=%x", mac.Sum(nil))

    return hmac.Equal([]byte(signature), []byte(expectedSignature))
}
```

### Webhook Configuration

```go
// Register webhook endpoint
mux.HandleFunc("/webhooks/content", webhookHandler.handleWebhook)
```

### Example Webhook Request

```bash
curl -X POST http://localhost:8080/webhooks/content \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Signature: sha256=..." \
  -d '{
    "event": "content.uploaded",
    "content_id": "abc123",
    "metadata": {
      "mime": "image/jpeg",
      "source": "mobile-app"
    }
  }'
```

### Pros & Cons

✅ **Pros:**
- External systems can trigger workflows
- Standard HTTP interface
- Can validate requests with signatures
- Works across network boundaries

❌ **Cons:**
- Need to expose HTTP endpoint
- Security considerations (authentication, rate limiting)
- Need webhook delivery guarantees

## 4. Database Triggers (Advanced)

Use PostgreSQL triggers to automatically enqueue workflows when content is inserted.

### PostgreSQL Function

```sql
-- Create function to enqueue workflow
CREATE OR REPLACE FUNCTION trigger_thumbnail_workflow()
RETURNS TRIGGER AS $$
BEGIN
    -- Insert into DBOS workflow queue
    INSERT INTO dbos.workflow_queue (
        workflow_name,
        input_data,
        created_at
    ) VALUES (
        'thumbnail-workflow',
        json_build_object(
            'content_id', NEW.id::text,
            'job', 'thumbnail',
            'versions', json_build_object('thumbnail', 1)
        )::text,
        NOW()
    );

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger on content table
CREATE TRIGGER content_uploaded_trigger
AFTER INSERT ON contents
FOR EACH ROW
WHEN (NEW.document_type LIKE 'image/%')
EXECUTE FUNCTION trigger_thumbnail_workflow();
```

### Pros & Cons

✅ **Pros:**
- Automatic - no application code needed
- Transactional consistency
- Can't be bypassed

❌ **Cons:**
- Database-specific
- Harder to debug
- Tightly coupled to database schema
- May not work with all DBOS versions

## Comparison Matrix

| Approach | Complexity | Decoupling | Infrastructure | Use Case |
|----------|------------|------------|----------------|----------|
| Application Hooks | ⭐ Low | Low | None | Simple apps, quick setup |
| Event-Driven | ⭐⭐ Medium | High | Message broker | Microservices, scalable |
| Webhooks | ⭐⭐ Medium | High | HTTP endpoint | External integrations |
| Database Triggers | ⭐⭐⭐ High | Low | PostgreSQL | Automatic, can't bypass |

## Recommended Patterns

### Pattern 1: Start Simple (Application Hooks)

```go
// In your upload handler
func handleUpload(w http.ResponseWriter, r *http.Request) {
    content, _ := uploadContent(r)

    // Immediate hook - trigger workflow
    go triggerThumbnailWorkflow(content.ID)

    respondOK(w, content)
}
```

**When to use:** MVP, single application, simple requirements

### Pattern 2: Scale with Events

```go
// Upload handler publishes event
func handleUpload(w http.ResponseWriter, r *http.Request) {
    content, _ := uploadContent(r)

    // Publish event
    publishEvent("content.uploaded", content)

    respondOK(w, content)
}

// Separate event handler triggers workflows
func handleContentUploadedEvent(event Event) {
    triggerWorkflows(event.ContentID, event.DocumentType)
}
```

**When to use:** Multiple services, need flexibility, want to add more workflows later

### Pattern 3: External Integration

```go
// Webhook endpoint
func handleWebhook(w http.ResponseWriter, r *http.Request) {
    payload := validateAndParse(r)

    runID, _ := triggerWorkflow(payload)

    respondAccepted(w, runID)
}
```

**When to use:** External systems need to trigger workflows, API-first

## Configuration Examples

### Environment Variables

```bash
# Application Hooks (none needed - direct integration)

# Event-Driven
REDIS_URL=redis://localhost:6379
EVENT_CHANNEL=content:uploaded

# Webhooks
WEBHOOK_SECRET=your-secret-key
WEBHOOK_ENDPOINT=/webhooks/content
```

### Feature Flags

```go
type Config struct {
    EnableAutoThumbnails bool   // Auto-trigger thumbnails on upload
    EnableOCR           bool   // Auto-trigger OCR
    EventBusURL         string // Optional event bus
    WebhookSecret       string // Optional webhook validation
}
```

## Testing Hooks

### Test Application Hook

```go
func TestUploadTriggersWorkflow(t *testing.T) {
    // Setup mock workflow runner
    mockRunner := &MockWorkflowRunner{}
    handler := UploadHandler{runner: mockRunner}

    // Upload content
    req := createUploadRequest()
    handler.handleUpload(w, req)

    // Verify workflow was triggered
    assert.Equal(t, 1, mockRunner.CallCount)
    assert.Equal(t, "thumbnail", mockRunner.LastJob)
}
```

### Test Event Handler

```go
func TestEventTriggerWorkflow(t *testing.T) {
    // Publish event
    publishEvent("content:uploaded", ContentEvent{
        ContentID: "test-id",
    })

    // Wait for handler
    time.Sleep(100 * time.Millisecond)

    // Verify workflow triggered
    assert.WorkflowTriggered(t, "test-id")
}
```

## Monitoring & Observability

### Log Hook Execution

```go
func (h *UploadHandler) triggerWorkflows(contentID string) {
    log.Printf("HOOK: Triggering workflows for content_id=%s", contentID)

    runID, err := h.pipelineRunner.RunAsync(ctx, req)
    if err != nil {
        log.Printf("HOOK_ERROR: Failed to trigger workflow: %v", err)
        // Optional: Send to error tracking (Sentry, etc.)
        return
    }

    log.Printf("HOOK_SUCCESS: Workflow queued with run_id=%s", runID)
}
```

### Metrics

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    hooksTriggered = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "pipeline_hooks_triggered_total",
            Help: "Total number of workflow hooks triggered",
        },
        []string{"hook_type", "job_type"},
    )
)

func triggerWorkflow(hookType, job string) {
    // Trigger workflow...

    // Record metric
    hooksTriggered.WithLabelValues(hookType, job).Inc()
}
```

## Best Practices

1. **Make hooks idempotent** - Workflow might be triggered multiple times
2. **Use goroutines for async hooks** - Don't block upload responses
3. **Log all hook executions** - Essential for debugging
4. **Handle errors gracefully** - Failed hooks shouldn't crash your app
5. **Use feature flags** - Easy to enable/disable hooks
6. **Monitor hook performance** - Track trigger rates and failures
7. **Test hook behavior** - Unit test + integration test
8. **Document hook triggers** - Clear when/why workflows run

## Troubleshooting

### Hook not triggering workflow

```go
// Add debug logging
log.Printf("DEBUG: Upload hook called for content_id=%s", contentID)
log.Printf("DEBUG: Workflow runner status: %+v", runner)
log.Printf("DEBUG: Attempting to trigger job=%s", job)

runID, err := runner.RunAsync(ctx, req)
log.Printf("DEBUG: RunAsync result: runID=%s, err=%v", runID, err)
```

### Workflow triggered multiple times

- Check if hook is called multiple times
- Verify goroutine usage (avoid race conditions)
- Use workflow deduplication in DBOS

### Event not received

- Check event bus connection
- Verify channel/topic name
- Check event format
- Look at event bus logs

## See Also

- [LIBRARY_USAGE.md](LIBRARY_USAGE.md) - Integration guide
- [examples/hooks/](examples/hooks/) - Working examples
- [API.md](API.md) - API reference
