# Python ML Worker

Python worker for ML-based content processing workflows (OCR, object detection) using DBOS.

## Features

- **Object Detection**: YOLOv8-based object detection in images
- **OCR**: PaddleOCR text extraction (coming soon)
- **DBOS Integration**: Durable workflow execution with PostgreSQL
- **Simple-content API**: Seamless integration with content storage

## Prerequisites

- Python 3.9+
- PostgreSQL (shared with Go worker)
- Virtual environment (recommended)

## Installation

```bash
# Create virtual environment
python3 -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate

# Install dependencies
pip install -r requirements.txt
```

## Configuration

Copy `.env.example` to `.env` and configure:

```bash
cp .env.example .env
```

Key settings:
- `DBOS_SYSTEM_DATABASE_URL`: PostgreSQL connection (same as Go worker)
- `CONTENT_API_URL`: PAS API endpoint (default: http://localhost:8080)
- `WORKER_HTTP_ADDR`: Python worker HTTP port (default: :8082)
- `YOLO_MODEL`: Model variant (yolov8n/yolov8m/yolov8l)

## Running

```bash
# Activate virtual environment
source venv/bin/activate

# Run worker
python main.py
```

The worker will:
1. Connect to DBOS PostgreSQL
2. Register object detection workflow
3. Start HTTP server on port 8082
4. Listen for workflow requests on the shared queue

## Workflows

### Object Detection

Detects objects in images using YOLOv8.

**Input**: Image content ID
**Output**: JSON with detected objects

```json
{
  "objects": [
    {
      "label": "person",
      "confidence": 0.92,
      "bounding_box": {
        "x": 100,
        "y": 150,
        "width": 200,
        "height": 400
      }
    }
  ],
  "total_objects": 2,
  "processing_time_ms": 850,
  "model": "yolov8n"
}
```

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────┐
│  Go Worker      │     │  Python Worker   │     │    PAS      │
│  (Thumbnails)   │────▶│  (ML Workloads)  │────▶│    API      │
│  :8081          │     │  :8082           │     │   :8080     │
└────────┬────────┘     └────────┬─────────┘     └──────────────┘
         │                       │
         └───────────┬───────────┘
                     ↓
              ┌──────────────┐
              │   DBOS       │
              │  PostgreSQL  │
              │    :5432     │
              └──────────────┘
```

## Model Download

On first run, YOLOv8 will automatically download the model (~6MB for yolov8n).

## Development

```bash
# Run with debug logging
export LOG_LEVEL=DEBUG
python main.py

# Test object detection workflow
curl -X POST http://localhost:8082/v1/process \
  -H "Content-Type: application/json" \
  -d '{
    "content_id": "your-content-id",
    "job": "object_detection"
  }'
```

## Troubleshooting

**Import errors**: Ensure virtual environment is activated and dependencies installed

**DBOS connection failed**: Check PostgreSQL is running and credentials match

**YOLO download fails**: Check internet connection, model will be cached after first download

**Out of memory**: Use smaller model (yolov8n) or increase available RAM
