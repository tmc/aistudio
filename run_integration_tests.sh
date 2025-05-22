#!/bin/bash
# Comprehensive integration test script for aistudio
# This script requires an API key to be set in the environment

# Print colorful message
print_header() {
    echo -e "\033[1;34m======== $1 ========\033[0m"
}

# Print success message
print_success() {
    echo -e "\033[1;32m✓ $1\033[0m"
}

# Print error message
print_error() {
    echo -e "\033[1;31m✗ $1\033[0m"
}

# Print info message
print_info() {
    echo -e "\033[1;33m→ $1\033[0m"
}

# Check for API key
if [ -z "$GEMINI_API_KEY" ]; then
    print_error "GEMINI_API_KEY environment variable not set"
    echo "Usage: GEMINI_API_KEY=your_api_key ./run_integration_tests.sh"
    exit 1
fi

print_header "Building aistudio"
go build ./cmd/aistudio
if [ $? -ne 0 ]; then
    print_error "Build failed"
    exit 1
fi
print_success "Build succeeded"

print_header "Running Go integration tests"
print_info "This will take a few minutes..."
go test -v -count=1 -run TestIntegrationSuite
if [ $? -ne 0 ]; then
    print_error "Integration tests failed"
    exit 1
fi
print_success "Integration tests passed"

print_header "Connection stability test"
print_info "Testing with actual aistudio binary..."

# Run aistudio with timeout for 30 seconds
timeout 30s ./aistudio --api-key=$GEMINI_API_KEY || TIMEOUT_STATUS=$?

# Check the result
if [ -z "$TIMEOUT_STATUS" ]; then
    print_error "aistudio exited before timeout (should stay running)"
    exit 1
elif [ "$TIMEOUT_STATUS" -eq 124 ]; then
    # Exit code 124 means timeout expired, which is what we want
    print_success "aistudio maintained connection for full 30 seconds"
else
    print_error "aistudio exited with error code $TIMEOUT_STATUS"
    exit 1
fi

print_header "All tests completed successfully"
print_success "aistudio connection stability verified"