"""
Workflow Executors for ML Worker
Implements simple-workflow WorkflowExecutor interface
"""

import logging
from typing import Dict, Any

# Import from simple-workflow library
import sys
import os

# Add simple-workflow python package to path
simple_workflow_path = os.path.join(
    os.path.dirname(os.path.dirname(os.path.dirname(__file__))),
    'simple-workflow',
    'python'
)
if os.path.exists(simple_workflow_path):
    sys.path.insert(0, simple_workflow_path)

from simpleworkflow import WorkflowExecutor

# Import ML workflows
from workflows.object_detection import detect_objects_workflow
from workflows.ocr import ocr_workflow

logger = logging.getLogger(__name__)


class MLWorkflowExecutor(WorkflowExecutor):
    """
    Executor for ML workflows (OCR and Object Detection)
    """

    def execute(self, intent: Dict[str, Any]) -> Any:
        """
        Execute ML workflow based on intent name

        Args:
            intent: Dict containing 'id', 'name', 'payload', 'attempt_count', 'max_attempts'

        Returns:
            Workflow result (dict)

        Raises:
            ValueError: If workflow name is unknown
        """
        workflow_name = intent['name']
        payload = intent['payload']

        logger.info(f"Executing ML workflow: {workflow_name}")

        if workflow_name == 'content.ocr.v1':
            return self._execute_ocr(payload)
        elif workflow_name == 'content.object_detection.v1':
            return self._execute_object_detection(payload)
        else:
            raise ValueError(
                f"Unknown workflow: {workflow_name}. "
                f"Supported: ['content.ocr.v1', 'content.object_detection.v1']"
            )

    def _execute_ocr(self, payload: Dict[str, Any]) -> Dict[str, Any]:
        """Execute OCR workflow"""
        content_id = payload['content_id']
        metadata = payload.get('metadata', {})

        logger.info(f"Executing OCR for content_id={content_id}")

        # Execute OCR workflow
        result = ocr_workflow(content_id, metadata)

        logger.info(f"OCR completed for content_id={content_id}")
        return result

    def _execute_object_detection(self, payload: Dict[str, Any]) -> Dict[str, Any]:
        """Execute object detection workflow"""
        content_id = payload['content_id']
        metadata = payload.get('metadata', {})

        logger.info(f"Executing object detection for content_id={content_id}")

        # Execute object detection workflow
        result = detect_objects_workflow(content_id, metadata)

        logger.info(f"Object detection completed for content_id={content_id}")
        return result
