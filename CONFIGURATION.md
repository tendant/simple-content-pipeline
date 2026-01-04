# Pipeline Configuration Guide

This document describes all configurable settings for the content pipeline workflows.

## Overview

The pipeline now supports runtime configuration through request metadata, allowing you to customize processing parameters without code changes.

---

## Thumbnail Generation

**Workflow**: `thumbnail`
**Worker**: Go pipeline worker
**Endpoint**: `POST /v1/process`

### Configurable Settings

#### 1. **Width** (pixels)
- **Parameter**: `metadata.width`
- **Default**: `300`
- **Valid Range**: > 0
- **Description**: Target thumbnail width in pixels

#### 2. **Height** (pixels)
- **Parameter**: `metadata.height`
- **Default**: `300`
- **Valid Range**: > 0
- **Description**: Target thumbnail height in pixels

#### 3. **Quality** (JPEG quality)
- **Parameter**: `metadata.quality`
- **Default**: `80`
- **Valid Range**: 1-100
- **Description**: JPEG compression quality (higher = better quality, larger file)

### Example Request

```bash
curl -X POST http://localhost:8081/v1/process \
  -H "Content-Type: application/json" \
  -d '{
    "content_id": "example-123",
    "job": "thumbnail",
    "metadata": {
      "width": "512",
      "height": "512",
      "quality": "90"
    },
    "versions": {"thumbnail": 1}
  }'
```

### Use Cases

**High-quality preview** (512x512, quality 95):
```json
{
  "metadata": {
    "width": "512",
    "height": "512",
    "quality": "95"
  }
}
```

**Small icon** (64x64, quality 70):
```json
{
  "metadata": {
    "width": "64",
    "height": "64",
    "quality": "70"
  }
}
```

**Balanced thumbnail** (default 300x300, quality 80):
```json
{
  "metadata": {}
}
```

---

## OCR Text Extraction

**Workflow**: `ocr`
**Worker**: Python ML worker
**Endpoint**: Via simple-workflow intent or DBOS queue

### Configurable Settings

#### 1. **Language**
- **Parameter**: `metadata.language`
- **Default**: `en` (English)
- **Valid Values**: Any PaddleOCR supported language code
- **Description**: OCR language model to use

**Supported Languages** (common):
- `en` - English
- `ch` - Chinese (Simplified)
- `fr` - French
- `german` - German
- `japan` - Japanese
- `korean` - Korean
- `es` - Spanish
- `pt` - Portuguese
- `ru` - Russian
- `ar` - Arabic
- [See PaddleOCR docs for full list](https://github.com/PaddlePaddle/PaddleOCR/blob/release/2.7/doc/doc_en/multi_languages_en.md)

### Example Request

```python
# Via DBOS
intent = {
    "content_id": "document-456",
    "metadata": {
        "language": "fr"  # French OCR
    }
}
```

### Use Cases

**English document** (default):
```json
{
  "metadata": {
    "language": "en"
  }
}
```

**French document**:
```json
{
  "metadata": {
    "language": "fr"
  }
}
```

**Chinese document**:
```json
{
  "metadata": {
    "language": "ch"
  }
}
```

### Important Notes

- **First use per language downloads the model** (~100-500MB depending on language)
- **Models are cached** - subsequent requests for the same language are fast
- **Multiple languages supported simultaneously** - each language has its own cached model

---

## Object Detection

**Workflow**: `object_detection`
**Worker**: Python ML worker
**Endpoint**: Via simple-workflow intent or DBOS queue

### Configurable Settings

#### 1. **Model Variant**
- **Parameter**: `metadata.model`
- **Default**: `yolo11n` (nano - fastest)
- **Valid Values**: `yolo11n`, `yolo11s`, `yolo11m`, `yolo11l`, `yolo11x`
- **Description**: YOLO model variant (size/accuracy tradeoff)

### Model Variants Comparison

| Model | Size | Speed | Accuracy | Use Case |
|-------|------|-------|----------|----------|
| `yolo11n` | Smallest (~6MB) | Fastest | Good | Real-time, low latency |
| `yolo11s` | Small (~22MB) | Very Fast | Better | Balanced speed/accuracy |
| `yolo11m` | Medium (~50MB) | Fast | High | Production workflows |
| `yolo11l` | Large (~100MB) | Moderate | Very High | Accuracy-critical |
| `yolo11x` | Extra Large (~150MB) | Slow | Highest | Maximum accuracy |

### Example Request

```python
# Via DBOS
intent = {
    "content_id": "photo-789",
    "metadata": {
        "model": "yolo11m"  # Medium model for better accuracy
    }
}
```

### Use Cases

**Real-time processing** (fastest):
```json
{
  "metadata": {
    "model": "yolo11n"
  }
}
```

**Production balance** (recommended):
```json
{
  "metadata": {
    "model": "yolo11m"
  }
}
```

**Maximum accuracy**:
```json
{
  "metadata": {
    "model": "yolo11x"
  }
}
```

### Important Notes

- **First use per model downloads weights** (~6-150MB depending on variant)
- **Models are cached** - subsequent requests for the same model are fast
- **Multiple models supported simultaneously** - each variant has its own cached instance
- **Performance Impact**: Larger models are slower but more accurate

---

## Environment Variables

### Worker Configuration

**Go Pipeline Worker**:
```bash
# Concurrency (number of concurrent workflows)
DBOS_QUEUE_CONCURRENCY=4        # Default: 4

# HTTP server
WORKER_HTTP_ADDR=:8081          # Default: :8081

# Database
DBOS_SYSTEM_DATABASE_URL="postgres://user:pass@host:port/db"

# Content API
CONTENT_API_URL="http://localhost:8080"
```

**Python ML Worker**:
```bash
# HTTP server
WORKER_HTTP_ADDR=:8082          # Default: :8082

# Content API
CONTENT_API_URL="http://localhost:8080"

# Database
DATABASE_URL="postgres://user:pass@host:port/db"
```

---

## Best Practices

### 1. **Choose Appropriate Defaults**

For most use cases:
- **Thumbnails**: 300x300, quality 80
- **OCR**: English (`en`)
- **Object Detection**: Nano model (`yolo11n`)

### 2. **Override When Needed**

Use metadata to override defaults for specific requirements:
```json
{
  "metadata": {
    "width": "1024",
    "quality": "95",
    "language": "fr",
    "model": "yolo11l"
  }
}
```

### 3. **Model/Language Caching**

- First request per language/model is slower (downloads weights)
- Subsequent requests are fast (cached in memory)
- Consider pre-loading common languages/models at startup

### 4. **Memory Considerations**

Each cached model/language uses memory:
- OCR languages: ~100-500MB each
- YOLO models: ~6-150MB each
- Monitor memory usage if using many variants

### 5. **Quality vs File Size**

JPEG quality impact on file size:
- Quality 60: ~50KB (small, lower quality)
- Quality 80: ~100KB (balanced - default)
- Quality 95: ~200KB (large, high quality)

---

## Configuration Examples

### High-Quality Document Processing

```json
{
  "job": "thumbnail",
  "content_id": "document-123",
  "metadata": {
    "width": "1024",
    "height": "1024",
    "quality": "95"
  }
}
```

### Multi-Language OCR Workflow

```python
# Process documents in different languages
languages = ['en', 'fr', 'es', 'de']

for lang in languages:
    process_ocr(content_id, metadata={'language': lang})
```

### Speed vs Accuracy Tradeoff

```python
# Fast processing (real-time)
detect_objects(content_id, metadata={'model': 'yolo11n'})

# Accurate processing (batch)
detect_objects(content_id, metadata={'model': 'yolo11l'})
```

---

## Validation Rules

**Thumbnail**:
- Width/Height: Must be positive integers
- Quality: Must be 1-100

**OCR**:
- Language: Any string (invalid codes will fail at runtime)

**Object Detection**:
- Model: Any string ending in `.pt` (invalid models will fail at runtime)

**Fallback Behavior**:
- Invalid values → Use defaults
- Missing metadata → Use defaults
- All settings are optional

---

## Troubleshooting

### "Model not found" Error

**Problem**: Invalid model variant specified
**Solution**: Use valid YOLO model names: `yolo11n`, `yolo11s`, `yolo11m`, `yolo11l`, `yolo11x`

### "Language not supported" Error

**Problem**: Invalid language code
**Solution**: Check [PaddleOCR supported languages](https://github.com/PaddlePaddle/PaddleOCR/blob/release/2.7/doc/doc_en/multi_languages_en.md)

### Slow First Request

**Problem**: First request per language/model is slow
**Reason**: Downloading model weights (~100-500MB)
**Solution**: Normal behavior, subsequent requests are fast

### High Memory Usage

**Problem**: Worker using too much memory
**Reason**: Multiple cached models/languages
**Solution**: Restart worker to clear cache, or limit variants used

---

## Migration Guide

### Before (Hardcoded)

```go
// Thumbnail always 300x300, quality 80
```

```python
# OCR always English
# Detection always yolo11n
```

### After (Configurable)

```json
{
  "metadata": {
    "width": "512",
    "quality": "90",
    "language": "fr",
    "model": "yolo11m"
  }
}
```

**Backward Compatibility**: ✅ All settings have sensible defaults. Existing code works without changes.

---

## Summary

| Setting | Parameter | Default | Valid Values |
|---------|-----------|---------|--------------|
| Thumbnail Width | `metadata.width` | 300 | > 0 |
| Thumbnail Height | `metadata.height` | 300 | > 0 |
| Thumbnail Quality | `metadata.quality` | 80 | 1-100 |
| OCR Language | `metadata.language` | `en` | PaddleOCR language codes |
| Detection Model | `metadata.model` | `yolo11n` | `yolo11n/s/m/l/x` |

**All settings are optional and backward compatible.**
