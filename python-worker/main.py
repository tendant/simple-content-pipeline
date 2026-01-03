#!/usr/bin/env python3
"""
Python ML Worker for simple-content-pipeline
Handles OCR and object detection workflows using DBOS
"""

import os
import sys
import logging
from dotenv import load_dotenv

from dbos import DBOS, SetWorkflowID
from workflows.object_detection import detect_objects_workflow

# Load environment variables
load_dotenv()

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

def main():
    """Initialize and run the Python ML worker"""

    # Get configuration from environment
    dbos_db_url = os.getenv('DBOS_SYSTEM_DATABASE_URL',
                            'postgres://pas:pwd@localhost:5432/pas?sslmode=disable')
    queue_name = os.getenv('DBOS_QUEUE_NAME', 'default')
    worker_addr = os.getenv('WORKER_HTTP_ADDR', ':8082')

    logger.info("Initializing Python ML Worker")
    logger.info(f"  Database: {dbos_db_url.split('@')[1]}")  # Hide password
    logger.info(f"  Queue: {queue_name}")
    logger.info(f"  Worker address: {worker_addr}")

    # Initialize DBOS
    DBOS(
        config={
            'database': {
                'hostname': 'localhost',
                'port': 5432,
                'username': 'pas',
                'password': 'pwd',
                'connectionTimeoutMillis': 3000,
            },
            'application': {
                'python_worker': {
                    'queue_name': queue_name,
                }
            }
        }
    )

    # Register workflows
    logger.info("✓ Registered workflow: ObjectDetectionWorkflow for job: object_detection")

    # Start HTTP server for async endpoints
    host, port = worker_addr.lstrip(':').split(':') if ':' in worker_addr else ('0.0.0.0', worker_addr.lstrip(':'))

    logger.info(f"Python ML worker starting on {worker_addr}")

    # Launch DBOS application
    DBOS.launch(port=int(port) if port else 8082)

if __name__ == '__main__':
    try:
        main()
    except KeyboardInterrupt:
        logger.info("Shutting down Python ML worker...")
        sys.exit(0)
    except Exception as e:
        logger.error(f"Fatal error: {e}", exc_info=True)
        sys.exit(1)
