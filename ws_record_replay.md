# WebSocket Recording and Replay

This document explains the WebSocket recording and replay system for Gemini live models.

## Overview

The WebSocket recording and replay system allows testing the Gemini live models WebSocket implementation without making real API calls. This is similar to gRPC recording but specifically designed for WebSocket connections.

## Key Components

1. **WSRecorder**: Records and replays WebSocket messages
2. **LiveClient Integration**: Modified to work with the recorder
3. **Test Scripts**: For creating and using recordings

## How It Works

### Recording Mode

1. Set the environment variable `WS_RECORD_MODE=1`
2. When a LiveClient is created, it checks for this environment variable
3. If enabled, it creates a WSRecorder in record mode
4. All messages sent to and received from the WebSocket are recorded to a JSON file
5. The recording includes metadata like timestamps and message types

### Replay Mode

1. Set the environment variable `WS_RECORD_MODE=0`
2. When a LiveClient is created, it checks for this environment variable
3. If enabled, it creates a WSRecorder in replay mode
4. The LiveClient doesn't connect to the real API
5. Instead, it reads messages from the recording file
6. It simulates delays between messages to mimic real API timing

## Using the System

### Creating Recordings

```bash
# Set your API key
export GEMINI_API_KEY=your-api-key-here

# Create a recording
export WS_RECORD_MODE=1
go run ./cmd/testlive/main.go --model gemini-2.0-flash-live-001 --ws

# Or use the helper script
./test_ws_record.sh --record --model gemini-2.0-flash-live-001
```

### Using Recordings

```bash
# Use a recording instead of real API
export WS_RECORD_MODE=0
go run ./cmd/testlive/main.go --model gemini-2.0-flash-live-001 --ws

# Or use the helper script
./test_ws_record.sh --model gemini-2.0-flash-live-001
```

### In Tests

The recording system is automatically used in tests when the environment variables are set:

```bash
# Run tests with recordings
export WS_RECORD_MODE=0
go test ./api -run TestLiveModel

# Create new recordings for tests
export GEMINI_API_KEY=your-api-key-here
export WS_RECORD_MODE=1
go test ./api -run TestLiveModelWithRecorder
```

## Recording Format

Recordings are stored as JSON files in the `api/testdata/ws_recordings` directory. Each recording is an array of message objects with the following fields:

- `direction`: "send" or "receive" indicating the message direction
- `payload`: The raw message payload (JSON)
- `timestamp`: Unix timestamp when the message was recorded
- `elapsed_ms`: Milliseconds elapsed since the first message
- `message_type`: WebSocket message type (usually 1 for text messages)

Example:

```json
[
  {
    "direction": "send",
    "payload": {"setup": {"model": "gemini-2.0-flash-live-001"}},
    "timestamp": 1715123456,
    "elapsed_ms": 0,
    "message_type": 1
  },
  {
    "direction": "receive",
    "payload": {"setupComplete": {}},
    "timestamp": 1715123457,
    "elapsed_ms": 150,
    "message_type": 1
  }
]
```

## Best Practices

1. **Create Stable Recordings**: Use simple, deterministic prompts when creating recordings
2. **Version Control**: Include recordings in version control for stable tests
3. **Update Regularly**: Update recordings when the API changes
4. **Test Both Modes**: Run tests in both record and replay modes

## Implementation Details

The recording and replay system is implemented in:

1. **api/ws_recorder.go**: Core recorder implementation
2. **api/live_client.go**: Modified to work with the recorder
3. **api/client.go**: Updates to initLiveStream method
4. **api/ws_recorder_test.go**: Tests for the recorder

## Differences from gRPC Recording

Unlike gRPC recording, which works with Google's existing gRPC recording library, the WebSocket recording system is a custom implementation designed specifically for this project. It works at the WebSocket message level rather than the HTTP/gRPC level.