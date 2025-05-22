#!/bin/bash
# Script to test WebSocket recording and replay functionality

# Check for API key
if [[ -z "$GEMINI_API_KEY" ]]; then
  echo "ERROR: GEMINI_API_KEY environment variable must be set"
  echo "Please set your Gemini API key with:"
  echo "  export GEMINI_API_KEY=your-api-key-here"
  exit 1
fi

# Default settings
MODEL="gemini-2.0-flash-live-001"
PROMPT="Hello, please respond with a brief message that includes the word 'recorded'."
MODE="replay"  # Default is replay mode
CREATE_DIRS=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --record)
      MODE="record"
      shift
      ;;
    --model)
      MODEL=$2
      shift 2
      ;;
    --prompt)
      PROMPT=$2
      shift 2
      ;;
    --clean)
      CREATE_DIRS=true
      shift
      ;;
    --help)
      echo "Usage: $0 [--record] [--model MODEL] [--prompt PROMPT] [--clean] [--help]"
      echo ""
      echo "Options:"
      echo "  --record     Run in recording mode (creates new recordings)"
      echo "  --model      Model to use (default: gemini-2.0-flash-live-001)"
      echo "  --prompt     Prompt to send (default: brief greeting with 'recorded')"
      echo "  --clean      Clean previous recordings and create new directories"
      echo "  --help       Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help to see available options"
      exit 1
      ;;
  esac
done

# Create or clean directories if needed
if $CREATE_DIRS; then
  echo "Creating/cleaning WebSocket recording directories..."
  mkdir -p api/testdata/ws_recordings
  rm -f api/testdata/ws_recordings/*.wsrec
fi

# Set the mode
if [[ $MODE == "record" ]]; then
  echo "Running in RECORD mode - will create new WebSocket recordings"
  export WS_RECORD_MODE=1
else
  echo "Running in REPLAY mode - will use existing WebSocket recordings"
  export WS_RECORD_MODE=0
fi

# Print test information
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║                WebSocket Record/Replay Test                     ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo "Model: $MODEL"
echo "Mode: $MODE"
echo "Prompt: $PROMPT"
echo ""

# Run the test
echo "Running test with WebSocket $MODE mode..."
go run ./cmd/testlive/main.go --model "$MODEL" --prompt "$PROMPT" --ws

# Show recording information
if [[ $MODE == "record" ]]; then
  echo ""
  echo "Recording details:"
  MODEL_SAFE=$(echo "$MODEL" | tr '/' '_' | tr ':' '_')
  RECORD_FILE="api/testdata/ws_recordings/${MODEL_SAFE}.wsrec"
  
  if [[ -f "$RECORD_FILE" ]]; then
    SIZE=$(ls -lh "$RECORD_FILE" | awk '{print $5}')
    COUNT=$(grep -c "direction" "$RECORD_FILE")
    echo "  - Created recording: $RECORD_FILE"
    echo "  - Size: $SIZE"
    echo "  - Message count: $COUNT"
  else
    echo "  - Recording file not found: $RECORD_FILE"
  fi
  
  echo ""
  echo "To replay this recording:"
  echo "  ./test_ws_record.sh --model $MODEL"
fi