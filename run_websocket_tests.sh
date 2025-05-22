#!/bin/bash
# Script to run WebSocket tests for the Gemini live implementation

# Set default values
RUN_E2E_TESTS=${RUN_E2E_TESTS:-0}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  key="$1"
  case $key in
    --e2e)
      RUN_E2E_TESTS=1
      shift
      ;;
    --help)
      echo "Usage: $0 [--e2e] [--help]"
      echo ""
      echo "Options:"
      echo "  --e2e    Run end-to-end tests that require API credentials"
      echo "  --help   Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help to see available options"
      exit 1
      ;;
  esac
done

# Check if API key is set when running E2E tests
if [[ $RUN_E2E_TESTS -eq 1 ]]; then
  if [[ -z "$GEMINI_API_KEY" ]]; then
    echo "ERROR: GEMINI_API_KEY environment variable must be set for E2E tests"
    echo "Please set your Gemini API key with:"
    echo "  export GEMINI_API_KEY=your-api-key-here"
    exit 1
  fi
  
  echo "Running WebSocket tests with E2E integration (API key provided)"
  export AISTUDIO_RUN_E2E_TESTS=1
else
  echo "Running WebSocket tests without E2E integration (unit tests only)"
  export AISTUDIO_RUN_E2E_TESTS=0
fi

# Run unit tests for WebSocket implementation
echo "Running WebSocket unit tests..."
go test ./api -run "TestWebSocketProtocolSelection|TestLiveModelSelection|TestLiveModelIntegration" -v

# Print summary
echo ""
echo "Test Summary:"
echo "-------------"
if [[ $RUN_E2E_TESTS -eq 1 ]]; then
  echo "✅ Ran unit tests and E2E integration tests for WebSocket implementation"
else
  echo "✅ Ran unit tests for WebSocket implementation"
  echo "⚠️  E2E integration tests were skipped (use --e2e flag to run them)"
fi