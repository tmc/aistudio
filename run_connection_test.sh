#!/bin/bash
# Run the connection stability test
# This script requires an API key to be set in the environment

if [ -z "$GEMINI_API_KEY" ]; then
    echo "Error: GEMINI_API_KEY environment variable not set"
    echo "Usage: GEMINI_API_KEY=your_api_key ./run_connection_test.sh"
    exit 1
fi

# Run the connection test with verbose output
go test -v ./api -run TestConnectionStability

# Exit with the test's exit code
exit $?