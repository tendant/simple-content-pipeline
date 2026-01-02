# Workflow Hooks Examples

Examples of different approaches to automatically trigger workflows.

## Available Examples

### 1. [Application Hook](application-hook/) ⭐ Recommended

**What it does:** Automatically trigger workflows directly from your upload handler.

**Best for:**
- Simple applications
- Quick setup
- Tight integration

**Example:**
```go
// In your upload handler
func handleUpload(w http.ResponseWriter, r *http.Request) {
    content, _ := uploadFile(r)

    // Hook: trigger workflow in background
    go triggerThumbnailWorkflow(content.ID)

    respondOK(w, content)
}
```

**Run it:**
```bash
cd application-hook
go run main.go

# Upload a file
curl -X POST http://localhost:8080/api/upload \
  -F "file=@image.jpg" \
  -F "name=My Image"
```

### 2. Event-Driven Hook (Coming Soon)

**What it does:** React to events from a message broker (Redis/NATS).

**Best for:**
- Microservices
- Decoupled architecture
- Multiple consumers

**Example:**
```go
// Event handler listens for uploads
func handleEvent(event ContentUploadedEvent) {
    if isImage(event.Type) {
        triggerThumbnailWorkflow(event.ContentID)
    }
}
```

### 3. Webhook Integration (Coming Soon)

**What it does:** External systems trigger workflows via HTTP webhooks.

**Best for:**
- Third-party integrations
- API-first architecture
- External triggers

**Example:**
```bash
# External system sends webhook
curl -X POST http://localhost:8080/webhooks/content \
  -H "X-Webhook-Signature: sha256=..." \
  -d '{
    "event": "content.uploaded",
    "content_id": "abc123"
  }'
```

## Quick Comparison

| Approach | Complexity | Infrastructure | Decoupling | Use Case |
|----------|------------|----------------|------------|----------|
| **Application Hook** | ⭐ Low | None | Low | Simple apps, MVP |
| **Event-Driven** | ⭐⭐ Medium | Message broker | High | Microservices |
| **Webhooks** | ⭐⭐ Medium | HTTP endpoint | High | External integrations |

## Getting Started

### 1. Choose Your Approach

Start with **Application Hook** if:
- You have a single application
- You want simple, direct integration
- You don't need event replay

Use **Event-Driven** if:
- You have microservices
- Multiple services need to react to uploads
- You want to decouple components

Use **Webhooks** if:
- External systems need to trigger workflows
- You need a standard HTTP API
- You want fine-grained access control

### 2. Run an Example

```bash
# Application Hook (simplest)
cd application-hook
go run main.go
```

### 3. Integrate into Your App

See the example code and adapt it to your application.

## Hook Patterns

### Pattern 1: Immediate Hook (Application)

```
Upload → Save → Hook (goroutine) → Workflow
                   ↓
              Response
```

User gets immediate response, workflow runs in background.

### Pattern 2: Event-Driven Hook

```
Upload → Save → Publish Event → Response
                       ↓
                Event Handler → Workflow
```

Upload and processing are completely decoupled.

### Pattern 3: Webhook

```
External System → Webhook → Verify → Workflow → Response
```

External system triggers workflow and waits for confirmation.

## Common Hook Use Cases

### Auto-Generate Thumbnails

```go
if isImage(contentType) {
    triggerWorkflow("thumbnail", contentID)
}
```

### OCR for Documents

```go
if isPDF(contentType) || isImage(contentType) {
    triggerWorkflow("ocr", contentID)
}
```

### Virus Scanning

```go
// Scan all uploads
triggerWorkflow("virus-scan", contentID)
```

### Content Moderation

```go
if needsModeration(userRole) {
    triggerWorkflow("moderation", contentID)
}
```

### Multi-Format Conversion

```go
if isVideo(contentType) {
    triggerWorkflow("convert-mp4", contentID)
    triggerWorkflow("convert-webm", contentID)
    triggerWorkflow("extract-thumbnail", contentID)
}
```

## Configuration

### Feature Flags

```go
type HookConfig struct {
    AutoThumbnails  bool
    AutoOCR         bool
    AutoModeration  bool
}

if config.AutoThumbnails && isImage(type) {
    triggerWorkflow("thumbnail", id)
}
```

### Environment Variables

```bash
AUTO_THUMBNAILS=true
AUTO_OCR=false
AUTO_MODERATION=true
```

## Best Practices

1. ✅ **Run hooks asynchronously** - Use goroutines, don't block uploads
2. ✅ **Handle errors gracefully** - Log but don't fail the upload
3. ✅ **Make hooks idempotent** - Workflows might trigger multiple times
4. ✅ **Log all hook executions** - Essential for debugging
5. ✅ **Use feature flags** - Easy to enable/disable
6. ✅ **Monitor hook performance** - Track rates and failures

## Troubleshooting

### Hook not triggered

```go
// Add logging
log.Printf("HOOK: Checking content_id=%s type=%s", id, type)
```

### Workflow triggered multiple times

- Ensure hook uses goroutine
- Check for duplicate event handlers
- Use workflow deduplication in DBOS

### Performance issues

- Use buffered channels for high volume
- Batch workflow triggers
- Monitor queue depth

## Documentation

- [HOOKS.md](../../HOOKS.md) - Complete hooks guide
- [Application Hook Example](application-hook/README.md) - Detailed walkthrough
- [LIBRARY_USAGE.md](../../LIBRARY_USAGE.md) - Integration patterns

## Next Steps

1. **Try the application hook example** - Simplest to understand
2. **Integrate into your app** - Adapt the pattern
3. **Add monitoring** - Track hook execution
4. **Scale as needed** - Move to events if required
