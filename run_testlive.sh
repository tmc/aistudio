#\!/bin/bash
# Script to test live models with and without WebSocket

set -e  # Exit immediately if a command exits with a non-zero status

# Check if API key is set
if [ -z "$GEMINI_API_KEY" ]; then
    echo "ERROR: GEMINI_API_KEY environment variable not set"
    echo "Please set your Gemini API key with: export GEMINI_API_KEY=your_key_here"
    exit 1
fi

# Build the test script
go build -o test_live_model ./cmd/testlive/main.go

# Check for Vertex AI configuration
vertex_args=""
if [ -n "$GOOGLE_CLOUD_PROJECT" ]; then
    vertex_args="--vertex --project-id=$GOOGLE_CLOUD_PROJECT"
fi

# Test live models
models=("gemini-2.0-flash-live-001" "gemini-2.5-flash-live")

# Print timestamp
echo "=== Starting Live Model Tests ($(date)) ==="

for model in "${models[@]}"; do
    echo -e "\n-----------------------------------"
    echo "Testing model $model with WebSocket: "
    ./test_live_model --model $model --ws -v
    
    echo -e "\n-----------------------------------"
    echo "Testing model $model with gRPC: "
    ./test_live_model --model $model -v
    
    # Test with Vertex AI if configured
    if [ -n "$vertex_args" ]; then
        echo -e "\n-----------------------------------"
        echo "Testing model $model with Vertex AI: "
        ./test_live_model --model $model $vertex_args -v
    fi
done

echo -e "\n=== All tests completed\! ==="
