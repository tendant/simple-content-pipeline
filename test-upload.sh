#!/bin/bash
set -e

# Simple test using the built-in /v1/test endpoint
# This endpoint does upload + process + verify in one call

echo "=== Pipeline Quick Test ==="
echo ""

# Configuration
PIPELINE_URL="${PIPELINE_URL:-http://localhost:8080}"

echo "Testing: ${PIPELINE_URL}/v1/test"
echo ""

# Run the built-in test endpoint
RESPONSE=$(curl -s "${PIPELINE_URL}/v1/test" 2>/dev/null)

if [ $? -ne 0 ]; then
    echo "❌ Test failed. Is pipeline-standalone running on ${PIPELINE_URL}?"
    echo "   Start it with: ./pipeline-standalone"
    exit 1
fi

TEST_STATUS=$(echo "$RESPONSE" | jq -r '.test_status // empty')

if [ "$TEST_STATUS" = "success" ]; then
    echo "✓ Test PASSED"
    echo ""
    echo "Results:"
    echo "$RESPONSE" | jq '{
      test_status,
      content_id,
      run_id,
      derived_count,
      derived_contents: [.derived_contents[] | {
        type: .derivation_type,
        variant: .variant,
        status: .status
      }]
    }'
else
    echo "❌ Test FAILED"
    echo ""
    echo "$RESPONSE" | jq .
    exit 1
fi

echo ""
echo "=== Test Complete ==="
echo ""
echo "For file upload testing with custom files, see:"
echo "  examples/upload-test/main.go"
