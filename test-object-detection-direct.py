#!/usr/bin/env python3
"""
Direct test of object detection workflow
"""

import os
import sys
from dotenv import load_dotenv
from dbos import DBOS

# Add parent directory to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), 'python-worker'))

from workflows.object_detection import detect_objects_workflow

if __name__ == '__main__':
    # Load environment
    load_dotenv()
    load_dotenv('python-worker/.env')

    if len(sys.argv) < 2:
        print("Usage: python test-object-detection-direct.py <content-id>")
        sys.exit(1)

    content_id = sys.argv[1]

    # Initialize DBOS
    dbos_db_url = os.getenv('DBOS_SYSTEM_DATABASE_URL',
                            'postgres://pas:pwd@localhost:5432/pas?sslmode=disable')
    app_version = os.getenv('DBOS_APPLICATION_VERSION', '')

    dbos_instance = DBOS(
        config={
            'name': 'content-pipeline',
            'system_database_url': dbos_db_url,
            'application_version': app_version if app_version else None,
        }
    )

    # Launch DBOS (required before workflow invocation)
    dbos_instance.launch()

    print(f"Triggering object detection for content: {content_id}")

    # Invoke workflow directly with DBOS
    handle = DBOS.start_workflow(detect_objects_workflow, content_id)

    print(f"✓ Object detection workflow started")
    print(f"  Workflow ID: {handle.workflow_id}")
    print(f"\nWaiting for results...")

    # Wait for completion
    result = handle.get_result()

    print(f"\n✅ Workflow completed!")
    print(f"Result: {result}")
