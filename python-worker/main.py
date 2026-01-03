#!/usr/bin/env python3
"""
Python ML Worker for simple-content-pipeline
Handles OCR and object detection workflows using DBOS
"""

import os
import sys
import logging
import threading
from dotenv import load_dotenv

from dbos import DBOS
from workflows.object_detection import detect_objects_workflow
from workflows.ocr import ocr_workflow

# Add simple-workflow python package to path
simple_workflow_path = os.path.join(
    os.path.dirname(os.path.dirname(os.path.dirname(__file__))),
    'simple-workflow',
    'python'
)
if os.path.exists(simple_workflow_path):
    sys.path.insert(0, simple_workflow_path)

from simpleworkflow import IntentPoller
from executors import MLWorkflowExecutor

# Load environment variables
load_dotenv()

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Parse configuration from environment
dbos_db_url = os.getenv('DBOS_SYSTEM_DATABASE_URL',
                        'postgres://pas:pwd@localhost:5432/pas?sslmode=disable')
queue_name = os.getenv('DBOS_QUEUE_NAME', 'default')
concurrency = int(os.getenv('DBOS_QUEUE_CONCURRENCY', '4'))
app_version = os.getenv('DBOS_APPLICATION_VERSION', '')

logger.info("Initializing Python ML Worker")
logger.info(f"  Database: {dbos_db_url.split('@')[1]}")  # Hide password
logger.info(f"  Queue: {queue_name}")
logger.info(f"  Concurrency: {concurrency}")
if app_version:
    logger.info(f"  Application Version: {app_version}")

# Initialize DBOS with queue configuration
# Use shared app name and application version to be part of same DBOS application
dbos = DBOS(
    config={
        'name': 'content-pipeline',  # Shared with PAS and Go worker
        'system_database_url': dbos_db_url,
        'application_version': app_version if app_version else None,
        'queues': {
            queue_name: {
                'concurrency': concurrency
            }
        }
    }
)

logger.info("✓ Registered workflows:")
logger.info("  - detect_objects_workflow (OCR and object detection)")
logger.info("  - ocr_workflow (OCR text extraction)")
logger.info(f"Python ML worker ready to process workflows from queue: {queue_name}")

# Initialize intent poller for simple-workflow integration
workflow_db_url = os.getenv('WORKFLOW_DATABASE_URL', '')
intent_poller = None

if workflow_db_url:
    # Add search_path=workflow to connection string
    if '?' in workflow_db_url:
        workflow_db_url += '&search_path=workflow'
    else:
        workflow_db_url += '?search_path=workflow'

    # Create ML executor
    ml_executor = MLWorkflowExecutor()

    # Configure poller
    supported_workflows = ['content.ocr.v1', 'content.object_detection.v1']
    intent_poller = IntentPoller(
        db_url=workflow_db_url,
        supported_workflows=supported_workflows,
        worker_id='python-ml-worker',
        poll_interval=2
    )

    # Register executor for both workflows
    intent_poller.register_executor('content.ocr.v1', ml_executor)
    intent_poller.register_executor('content.object_detection.v1', ml_executor)

    # Start poller in background thread
    poller_thread = threading.Thread(target=intent_poller.start, daemon=True)
    poller_thread.start()

    logger.info("✓ Simple-workflow intent poller started")
    logger.info(f"  Supported workflows: {supported_workflows}")
    logger.info(f"  Worker ID: python-ml-worker")
else:
    logger.warning("⚠ WORKFLOW_DATABASE_URL not set, intent poller disabled (using DBOS queue fallback)")

if __name__ == '__main__':
    try:
        # Start DBOS worker (queue-based, no HTTP server)
        logger.info("Starting DBOS worker...")
        DBOS.launch()
    except KeyboardInterrupt:
        logger.info("Shutting down Python ML worker...")
        sys.exit(0)
    except Exception as e:
        logger.error(f"Fatal error: {e}", exc_info=True)
        sys.exit(1)
