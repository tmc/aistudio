#\!/bin/bash
# This script adds some test messages to the aistudio app
echo "Running aistudio with test data..."
./aistudio --model=gemini-1.5-pro --tools=testdata/tools-cc.json
