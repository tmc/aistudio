#!/bin/bash
# Script to test live models with both WebSocket and gRPC protocols

set -e  # Exit immediately if a command exits with a non-zero status

# Check for API key
if [[ -z "$GEMINI_API_KEY" ]]; then
  echo "ERROR: GEMINI_API_KEY environment variable must be set"
  echo "Please set your Gemini API key with:"
  echo "  export GEMINI_API_KEY=your-api-key-here"
  exit 1
fi

# Set default values
MODELS=("gemini-2.0-flash-live-001" "gemini-2.5-flash-live")
PROMPT="Hello! Please reply with a very brief greeting to demonstrate that you're working. Include the word 'testing' in your response."
VERBOSE=""
RUN_UNIT_TESTS=1
RUN_E2E_TESTS=1

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --prompt)
      PROMPT="$2"
      shift 2
      ;;
    --verbose)
      VERBOSE="-v"
      shift
      ;;
    --unit-only)
      RUN_E2E_TESTS=0
      shift
      ;;
    --e2e-only)
      RUN_UNIT_TESTS=0
      shift
      ;;
    --help)
      echo "Usage: $0 [--prompt PROMPT] [--verbose] [--unit-only] [--e2e-only] [--help]"
      echo ""
      echo "Options:"
      echo "  --prompt PROMPT   Prompt to send (default: a simple greeting)"
      echo "  --verbose         Enable verbose output"
      echo "  --unit-only       Only run unit tests (no API calls)"
      echo "  --e2e-only        Only run E2E tests (with API calls)"
      echo "  --help            Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help to see available options"
      exit 1
      ;;
  esac
done

# Build the test command if needed
if [[ $RUN_E2E_TESTS -eq 1 ]]; then
  echo "Building test command..."
  go build -o testlive ./cmd/testlive/main.go
fi

# Print test information
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║                 WebSocket Implementation Test                   ║"
echo "╚════════════════════════════════════════════════════════════════╝"

# Run unit tests if requested
if [[ $RUN_UNIT_TESTS -eq 1 ]]; then
  echo "Running WebSocket unit tests..."
  ./run_websocket_tests.sh
  echo ""
fi

# Run E2E tests if requested
if [[ $RUN_E2E_TESTS -eq 1 ]]; then
  echo "Running WebSocket E2E tests with live models..."
  
  for model in "${MODELS[@]}"; do
    echo "╔════════════════════════════════════════════════════════════════╗"
    echo "║ Testing model: $model"
    echo "╚════════════════════════════════════════════════════════════════╝"
    
    # Run with gRPC protocol (default)
    echo "✓ Testing with gRPC protocol:"
    ./testlive --model "$model" --prompt "$PROMPT" $VERBOSE
    echo ""
    
    # Run with WebSocket protocol
    echo "✓ Testing with WebSocket protocol:"
    ./testlive --model "$model" --prompt "$PROMPT" --ws $VERBOSE
    echo ""
  done
fi

# Print summary
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║                      All tests completed                        ║"
echo "╚════════════════════════════════════════════════════════════════╝"
