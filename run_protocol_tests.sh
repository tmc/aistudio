#!/bin/bash
# Script to run protocol test matrix (WebSocket and gRPC)
# This script supports both recording and replay modes for both protocols

set -e  # Exit immediately if a command exits with a non-zero status

# Default settings
PROTOCOLS="ws,grpc"  # Run both protocols by default
MODE="replay"        # Default to replay mode
MODELS="gemini-2.0-flash-live-001,gemini-2.5-flash-live" # Default models
PROMPT="Hello, include the word 'testing' in your response."
CLEAN=false
E2E_TESTS=true
UNIT_TESTS=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --record)
      MODE="record"
      shift
      ;;
    --protocols)
      PROTOCOLS=$2
      shift 2
      ;;
    --models)
      MODELS=$2
      shift 2
      ;;
    --prompt)
      PROMPT=$2
      shift 2
      ;;
    --clean)
      CLEAN=true
      shift
      ;;
    --unit-only)
      E2E_TESTS=false
      UNIT_TESTS=true
      shift
      ;;
    --e2e-only)
      E2E_TESTS=true
      UNIT_TESTS=false
      shift
      ;;
    --all-tests)
      E2E_TESTS=true
      UNIT_TESTS=true
      shift
      ;;
    --help)
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Run protocol test matrix with WebSocket and/or gRPC recording/replay"
      echo ""
      echo "Options:"
      echo "  --record            Run in record mode (default: replay)"
      echo "  --protocols PROTO   Comma-separated list of protocols to test (ws,grpc)"
      echo "  --models MODELS     Comma-separated list of models to test"
      echo "  --prompt PROMPT     Prompt to use for testing"
      echo "  --clean             Clean previous recordings"
      echo "  --unit-only         Only run unit tests"
      echo "  --e2e-only          Only run E2E tests"
      echo "  --all-tests         Run both unit and E2E tests"
      echo "  --help              Show this help message"
      echo ""
      echo "Example:"
      echo "  $0 --record --protocols ws --models gemini-2.0-flash-live-001"
      echo "  $0 --protocols grpc,ws --e2e-only"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help to see available options"
      exit 1
      ;;
  esac
done

# Check for API key if in record mode
if [[ $MODE == "record" ]]; then
  if [[ -z "$GEMINI_API_KEY" ]]; then
    echo "ERROR: GEMINI_API_KEY environment variable must be set for record mode"
    echo "Please set your Gemini API key with:"
    echo "  export GEMINI_API_KEY=your-api-key-here"
    exit 1
  fi
fi

# Function to test a model with a protocol
test_protocol() {
  local protocol=$1
  local model=$2
  local mode=$3
  local prompt="$4"
  
  echo "╔════════════════════════════════════════════════════════════════╗"
  echo "║ Testing ${model} with ${protocol} (${mode} mode)"
  echo "╚════════════════════════════════════════════════════════════════╝"
  
  if [[ $protocol == "ws" ]]; then
    # Set WebSocket recording mode
    export WS_RECORD_MODE=$([ "$mode" == "record" ] && echo "1" || echo "0")
    
    # Run with WebSocket
    go run ./cmd/testlive/main.go --model "$model" --prompt "$prompt" --ws
  elif [[ $protocol == "grpc" ]]; then
    # Set gRPC recording mode
    export RECORD_GRPC=$([ "$mode" == "record" ] && echo "1" || echo "0")
    
    # Run with gRPC
    go run ./cmd/testlive/main.go --model "$model" --prompt "$prompt"
  else
    echo "Unknown protocol: $protocol"
    return 1
  fi
}

# Create or clean recording directories
if $CLEAN; then
  echo "Cleaning recording directories..."
  mkdir -p api/testdata/ws_recordings
  mkdir -p api/testdata/grpc_recordings
  
  # Only clean files in record mode to avoid losing recordings unexpectedly
  if [[ $MODE == "record" ]]; then
    rm -f api/testdata/ws_recordings/*.wsrec
    rm -f api/testdata/grpc_recordings/*.replay
  fi
fi

# Ensure directories exist
mkdir -p api/testdata/ws_recordings
mkdir -p api/testdata/grpc_recordings

# Split the protocols and models into arrays
IFS=',' read -r -a PROTOCOL_ARRAY <<< "$PROTOCOLS"
IFS=',' read -r -a MODEL_ARRAY <<< "$MODELS"

# Print test matrix info
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║                    Protocol Test Matrix                         ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo "Mode: $MODE"
echo "Protocols: ${PROTOCOLS}"
echo "Models: ${MODELS}"
echo "Prompt: $PROMPT"
echo ""

# Run unit tests if requested
if $UNIT_TESTS; then
  echo "╔════════════════════════════════════════════════════════════════╗"
  echo "║                     Running Unit Tests                          ║"
  echo "╚════════════════════════════════════════════════════════════════╝"
  
  # Set environment variables for testing
  export AISTUDIO_RUN_E2E_TESTS=0
  
  # Run WebSocket tests if included in protocols
  if [[ $PROTOCOLS == *"ws"* ]]; then
    echo "Running WebSocket unit tests..."
    go test ./api -run "TestWSRecorder|TestLiveModelSelection|TestWebSocketProtocolSelection" -v
  fi
  
  # Run gRPC tests if included in protocols
  if [[ $PROTOCOLS == *"grpc"* ]]; then
    echo "Running gRPC unit tests..."
    go test ./api -run "TestLiveModelProtocolSelection" -v
  fi
  
  echo ""
fi

# Run E2E tests if requested
if $E2E_TESTS; then
  echo "╔════════════════════════════════════════════════════════════════╗"
  echo "║                     Running E2E Tests                           ║"
  echo "╚════════════════════════════════════════════════════════════════╝"
  
  # Set environment variables for E2E testing
  export AISTUDIO_RUN_E2E_TESTS=1
  
  # Run the protocol test matrix
  for model in "${MODEL_ARRAY[@]}"; do
    for protocol in "${PROTOCOL_ARRAY[@]}"; do
      test_protocol "$protocol" "$model" "$MODE" "$PROMPT"
      echo ""
    done
  done
fi

# Print summary
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║                      Test Matrix Complete                       ║"
echo "╚════════════════════════════════════════════════════════════════╝"

# Show recording info if in record mode
if [[ $MODE == "record" ]]; then
  echo "Recordings created:"
  
  if [[ $PROTOCOLS == *"ws"* ]]; then
    echo "WebSocket recordings:"
    ls -lh api/testdata/ws_recordings/*.wsrec 2>/dev/null || echo "  No WebSocket recordings found"
  fi
  
  if [[ $PROTOCOLS == *"grpc"* ]]; then
    echo "gRPC recordings:"
    ls -lh api/testdata/grpc_recordings/*.replay 2>/dev/null || echo "  No gRPC recordings found"
  fi
  
  echo ""
  echo "To replay these recordings:"
  echo "./run_protocol_tests.sh --protocols $PROTOCOLS --models $MODELS"
fi