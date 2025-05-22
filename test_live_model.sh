#!/bin/bash

# Test script to verify connectivity to Gemini live models

# Check if GEMINI_API_KEY is set
if [ -z "$GEMINI_API_KEY" ]; then
    echo "ERROR: Please set the GEMINI_API_KEY environment variable"
    exit 1
fi

# Default model to test
LIVE_MODEL=${1:-"gemini-2.5-pro-live"}

# Print test information
echo "========================================="
echo "Testing Gemini Live Model Connectivity"
echo "========================================="
echo "Model: $LIVE_MODEL"
echo "Time: $(date)"
echo "-----------------------------------------"

# Run aistudio with the specified live model in stdin mode
echo "Sending test prompt to $LIVE_MODEL..."
TEST_PROMPT="Hello, please confirm you are a live model. Include the current date and time in your response."
OUTPUT=$(echo "$TEST_PROMPT" | go run cmd/aistudio/main.go \
    --model "$LIVE_MODEL" \
    --api-key "$GEMINI_API_KEY" \
    --stdin 2>&1)

# Check exit status
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
    echo "ERROR: Test failed with exit code $EXIT_CODE"
    echo "Output:"
    echo "$OUTPUT"
    exit 1
fi

# Check if response mentions being a live model
if echo "$OUTPUT" | grep -i "live model" > /dev/null; then
    echo "SUCCESS: Live model responded correctly"
    echo "-----------------------------------------"
    echo "Response excerpt:"
    echo "$OUTPUT" | grep -i -A 3 -B 3 "live model"
    echo "-----------------------------------------"
    echo "Test completed successfully"
    exit 0
else
    echo "WARNING: Response doesn't explicitly confirm being a live model"
    echo "-----------------------------------------"
    echo "Response:"
    echo "$OUTPUT"
    echo "-----------------------------------------"
    echo "Test completed with warnings"
    exit 0
fi