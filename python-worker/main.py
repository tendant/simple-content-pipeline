#!/usr/bin/env python3
"""
Python ML Worker for simple-content-pipeline
Handles OCR and object detection workflows using DBOS
"""

import os
import sys
import logging
from dotenv import load_dotenv

from dbos import DBOS
from workflows.object_detection import detect_objects_workflow

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

logger.info("Initializing Python ML Worker")
logger.info(f"  Database: {dbos_db_url.split('@')[1]}")  # Hide password
logger.info(f"  Queue: {queue_name}")

# Initialize DBOS (pure library mode, no HTTP server)
DBOS(
    config={
        'name': 'python_ml_worker',
        'system_database_url': dbos_db_url,
    }
)

logger.info("✓ Registered workflow: ObjectDetectionWorkflow for job: object_detection")
logger.info(f"Python ML worker ready to process workflows from queue: {queue_name}")

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
