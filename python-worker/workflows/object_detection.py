"""
Object Detection Workflow using YOLOv8
"""

import io
import json
import logging
import os
import time
from typing import Dict, Any, List
from PIL import Image
import requests

from ultralytics import YOLO
from dbos import DBOS, SetWorkflowID

logger = logging.getLogger(__name__)

# Global model instance (loaded once)
_yolo_model = None


def get_yolo_model():
    """Get or initialize YOLO model (singleton pattern)"""
    global _yolo_model
    if _yolo_model is None:
        logger.info("Loading YOLOv8 model...")
        # Use YOLOv8n (nano) for speed, can switch to yolov8m/yolov8l for accuracy
        _yolo_model = YOLO('yolov8n.pt')
        logger.info("✓ YOLOv8 model loaded")
    return _yolo_model


class ContentHTTPClient:
    """HTTP client for accessing content via simple-content API"""

    def __init__(self, base_url: str):
        self.base_url = base_url.rstrip('/')
        self.session = requests.Session()

    def download_content(self, content_id: str) -> bytes:
        """Download content binary data"""
        url = f"{self.base_url}/api/v1/contents/{content_id}/download"
        response = self.session.get(url, timeout=30)
        response.raise_for_status()
        return response.content

    def upload_derived(self, parent_id: str, derivation_type: str, variant: str,
                      data: bytes, filename: str) -> str:
        """Upload derived content using multipart form"""
        url = f"{self.base_url}/api/v1/contents/{parent_id}/derived"

        files = {
            'file': (filename, data, 'application/json')
        }
        data_form = {
            'derivation_type': derivation_type,
            'variant': variant
        }

        response = self.session.post(url, files=files, data=data_form, timeout=30)
        response.raise_for_status()

        result = response.json()
        # Extract ID from nested or direct response
        if 'data' in result and 'id' in result['data']:
            return result['data']['id']
        elif 'id' in result:
            return result['id']
        else:
            raise ValueError(f"No ID in response: {result}")


@DBOS.workflow()
def detect_objects_workflow(content_id: str, metadata: Dict[str, Any] = None) -> Dict[str, Any]:
    """
    DBOS workflow for object detection

    Args:
        content_id: ID of content to process
        metadata: Optional metadata (e.g., model config, confidence threshold)

    Returns:
        Dictionary with detection results
    """
    metadata = metadata or {}
    run_id = DBOS.workflow_id

    logger.info(f"[{run_id}] Starting object detection workflow for content_id={content_id}")

    try:
        # Get content API URL from environment
        content_api_url = metadata.get('content_api_url') or \
                         os.environ.get('CONTENT_API_URL', 'http://localhost:8080')

        client = ContentHTTPClient(content_api_url)

        # Download source image
        logger.info(f"[{run_id}] Downloading source content...")
        image_data = client.download_content(content_id)
        logger.info(f"[{run_id}] Downloaded {len(image_data)} bytes")

        # Load image
        image = Image.open(io.BytesIO(image_data))
        logger.info(f"[{run_id}] Image loaded: {image.size} {image.mode}")

        # Run object detection
        logger.info(f"[{run_id}] Running object detection...")
        start_time = time.time()

        model = get_yolo_model()
        results = model(image, verbose=False)

        processing_time_ms = int((time.time() - start_time) * 1000)

        # Parse detection results
        detections = []
        for result in results:
            boxes = result.boxes
            for box in boxes:
                detection = {
                    'label': model.names[int(box.cls[0])],
                    'confidence': float(box.conf[0]),
                    'bounding_box': {
                        'x': float(box.xyxy[0][0]),
                        'y': float(box.xyxy[0][1]),
                        'width': float(box.xyxy[0][2] - box.xyxy[0][0]),
                        'height': float(box.xyxy[0][3] - box.xyxy[0][1])
                    }
                }
                detections.append(detection)

        logger.info(f"[{run_id}] Detected {len(detections)} objects in {processing_time_ms}ms")

        # Create detection result JSON
        detection_result = {
            'objects': detections,
            'total_objects': len(detections),
            'processing_time_ms': processing_time_ms,
            'model': 'yolov8n',
            'image_size': {
                'width': image.size[0],
                'height': image.size[1]
            }
        }

        # Upload as derived content
        logger.info(f"[{run_id}] Uploading detection results...")
        json_data = json.dumps(detection_result, indent=2).encode('utf-8')

        derived_id = client.upload_derived(
            parent_id=content_id,
            derivation_type='object_detection',
            variant='object_detection_v1',
            data=json_data,
            filename='detections.json'
        )

        logger.info(f"[{run_id}] Derived content written: {derived_id}")
        logger.info(f"[{run_id}] Object detection workflow completed successfully")

        return {
            'success': True,
            'derived_id': derived_id,
            'detections_count': len(detections),
            'processing_time_ms': processing_time_ms
        }

    except Exception as e:
        logger.error(f"[{run_id}] Object detection failed: {e}", exc_info=True)
        return {
            'success': False,
            'error': str(e)
        }


# Export workflow
__all__ = ['detect_objects_workflow']
