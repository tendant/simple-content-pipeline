"""
OCR Workflow using PaddleOCR
"""

import io
import json
import logging
import os
import time
from typing import Dict, Any, List
from PIL import Image
import requests
import numpy as np

from paddleocr import PaddleOCR
from dbos import DBOS

logger = logging.getLogger(__name__)

# Global OCR instance (loaded once)
_ocr_engine = None


def get_ocr_engine():
    """Get or initialize PaddleOCR engine (singleton pattern)"""
    global _ocr_engine
    if _ocr_engine is None:
        logger.info("Loading PaddleOCR engine...")
        # Use English model, enable textline orientation for rotated text
        _ocr_engine = PaddleOCR(
            use_textline_orientation=True,
            lang='en'
        )
        logger.info("✓ PaddleOCR engine loaded")
    return _ocr_engine


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
def ocr_workflow(content_id: str, metadata: Dict[str, Any] = None) -> Dict[str, Any]:
    """
    DBOS workflow for OCR text extraction

    Args:
        content_id: ID of content to process
        metadata: Optional metadata (e.g., language, confidence threshold)

    Returns:
        Dictionary with OCR results
    """
    metadata = metadata or {}
    run_id = DBOS.workflow_id

    logger.info(f"[{run_id}] Starting OCR workflow for content_id={content_id}")

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

        # Convert PIL Image to numpy array for PaddleOCR
        image_array = np.array(image)

        # Run OCR
        logger.info(f"[{run_id}] Running OCR extraction...")
        start_time = time.time()

        ocr_engine = get_ocr_engine()
        result = ocr_engine.ocr(image_array)

        processing_time_ms = int((time.time() - start_time) * 1000)

        # Parse OCR results
        # PaddleOCR v3.x returns: [{'rec_texts': [...], 'rec_scores': [...], 'rec_polys': [...]}]
        text_blocks = []
        full_text_lines = []
        total_confidence = 0.0

        if result and len(result) > 0 and isinstance(result[0], dict):
            ocr_result = result[0]
            rec_texts = ocr_result.get('rec_texts', [])
            rec_scores = ocr_result.get('rec_scores', [])
            rec_polys = ocr_result.get('rec_polys', [])

            for i in range(len(rec_texts)):
                text = rec_texts[i]
                confidence = float(rec_scores[i]) if i < len(rec_scores) else 0.0
                polygon = rec_polys[i] if i < len(rec_polys) else None

                if polygon is not None and len(polygon) >= 4:
                    # Calculate bounding box from polygon points
                    x_coords = [point[0] for point in polygon]
                    y_coords = [point[1] for point in polygon]

                    text_block = {
                        'text': text,
                        'confidence': confidence,
                        'bounding_box': {
                            'x': float(min(x_coords)),
                            'y': float(min(y_coords)),
                            'width': float(max(x_coords) - min(x_coords)),
                            'height': float(max(y_coords) - min(y_coords))
                        },
                        'polygon': [[float(p[0]), float(p[1])] for p in polygon]
                    }

                    text_blocks.append(text_block)
                    full_text_lines.append(text)
                    total_confidence += confidence

        # Calculate average confidence
        avg_confidence = total_confidence / len(text_blocks) if text_blocks else 0.0
        full_text = '\n'.join(full_text_lines)

        logger.info(f"[{run_id}] Extracted {len(text_blocks)} text blocks in {processing_time_ms}ms")

        # Create OCR result JSON
        ocr_result = {
            'text': full_text,
            'blocks': text_blocks,
            'total_blocks': len(text_blocks),
            'average_confidence': avg_confidence,
            'processing_time_ms': processing_time_ms,
            'engine': 'paddleocr',
            'language': metadata.get('language', 'en'),
            'image_size': {
                'width': image.size[0],
                'height': image.size[1]
            }
        }

        # Upload as derived content
        logger.info(f"[{run_id}] Uploading OCR results...")
        json_data = json.dumps(ocr_result, indent=2, ensure_ascii=False).encode('utf-8')

        derived_id = client.upload_derived(
            parent_id=content_id,
            derivation_type='ocr_text',
            variant='paddleocr_v1',
            data=json_data,
            filename='ocr.json'
        )

        logger.info(f"[{run_id}] Derived content written: {derived_id}")
        logger.info(f"[{run_id}] OCR workflow completed successfully")

        return {
            'success': True,
            'derived_id': derived_id,
            'text_blocks_count': len(text_blocks),
            'average_confidence': avg_confidence,
            'processing_time_ms': processing_time_ms
        }

    except Exception as e:
        logger.error(f"[{run_id}] OCR failed: {e}", exc_info=True)
        return {
            'success': False,
            'error': str(e)
        }


# Export workflow
__all__ = ['ocr_workflow']
