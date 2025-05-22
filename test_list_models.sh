#!/bin/bash
set -e

# Build the application
echo "Building aistudio..."
go build

# Test the list_models tool in the application
echo "Testing list_models tool..."
echo "Use the list_models tool to list all available models and filter for gemini-2.0 models" | ./aistudio --tools --tool-approval=false

echo "Test completed!"