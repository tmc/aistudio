#!/bin/bash
# Script to run gRPC replay tests for WebSocket implementation

# Default to replay mode
RECORD_MODE=${RECORD_MODE:-0}
CREATE_GOLDEN=${CREATE_GOLDEN:-0}

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --record)
      RECORD_MODE=1
      shift
      ;;
    --golden)
      CREATE_GOLDEN=1
      shift
      ;;
    --help)
      echo "Usage: $0 [--record] [--golden]"
      echo ""
      echo "Options:"
      echo "  --record    Run in record mode to create new gRPC recordings"
      echo "  --golden    Create golden test data (requires API key)"
      echo "  --help      Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help for usage information"
      exit 1
      ;;
  esac
done

# Check if API key is set when in record mode
if [[ $RECORD_MODE -eq 1 || $CREATE_GOLDEN -eq 1 ]]; then
  if [[ -z "$GEMINI_API_KEY" ]]; then
    echo "ERROR: GEMINI_API_KEY environment variable must be set for record mode"
    echo "Please set your Gemini API key with:"
    echo "  export GEMINI_API_KEY=your-api-key-here"
    exit 1
  fi
fi

# Show current mode
if [[ $RECORD_MODE -eq 1 ]]; then
  echo "Running in RECORD mode - will create new gRPC recordings"
  export RECORD_GRPC=1
else
  echo "Running in REPLAY mode - will use existing gRPC recordings"
  export RECORD_GRPC=0
fi

if [[ $CREATE_GOLDEN -eq 1 ]]; then
  echo "Will create golden test data"
  export CREATE_GOLDEN_DATA=1
else
  export CREATE_GOLDEN_DATA=0
fi

# Create directories if needed
mkdir -p api/testdata/grpc_recordings
mkdir -p api/testdata/grpc_replay

# Run the gRPC replay tests
go test ./api -run "TestWithGRPCReplay|TestLiveModelWithRecordedSessions|TestCreateGoldenTestData" -v

# Show summary
if [[ $RECORD_MODE -eq 1 ]]; then
  echo ""
  echo "Recordings created. You can now run the tests in replay mode."
  echo "Run: $0"
fi

if [[ $CREATE_GOLDEN -eq 1 ]]; then
  echo ""
  echo "Golden test data created."
fi