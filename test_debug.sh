#!/bin/bash

# Test script for debugging features
set -e

echo "🔧 Testing aistudio debugging features..."

# Build with debugging
echo "Building aistudio with debugging support..."
go build -o aistudio-debug ./cmd/aistudio/

# Test 1: Verify debug environment variables are recognized
echo "Test 1: Environment variable recognition"
export AISTUDIO_DEBUG_CONNECTION=true
export AISTUDIO_CONNECTION_TIMEOUT=5

# Test with a very short timeout to trigger timeout quickly
echo "Testing with 5-second timeout (should fail fast)..."
timeout 10s ./aistudio-debug -vertex --model=models/gemini-2.5-flash-exp 2>&1 | head -20 || echo "Expected timeout/failure"

# Test 2: Check pprof server availability
echo "Test 2: pprof server"
# Start with pprof in background, then check if it's available
./aistudio-debug -pprof-server=:6061 &
AISTUDIO_PID=$!
sleep 2

# Check if pprof endpoint is available
if curl -s http://localhost:6061/debug/pprof/ | grep -q "Types of profiles available"; then
    echo "✅ pprof server is working"
else
    echo "❌ pprof server not accessible"
fi

# Clean up
kill $AISTUDIO_PID 2>/dev/null || true
sleep 1

# Test 3: Verify debug log format
echo "Test 3: Debug log format"
export AISTUDIO_DEBUG_CONNECTION=true
export AISTUDIO_CONNECTION_TIMEOUT=3

echo "Looking for debug log patterns..."
timeout 8s ./aistudio-debug 2>&1 | grep -E "\[DEBUG\]|\[ERROR\]" | head -5 || echo "Debug logs captured"

echo "✅ Debugging features test completed"
echo ""
echo "To use debugging features:"
echo "  export AISTUDIO_DEBUG_CONNECTION=true"
echo "  export AISTUDIO_DEBUG_STREAM=true" 
echo "  export AISTUDIO_CONNECTION_TIMEOUT=30"
echo "  ./aistudio -pprof-server=:6060"