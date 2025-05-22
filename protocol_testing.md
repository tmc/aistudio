# Protocol Testing with Recording and Replay

This document explains how to test the WebSocket and gRPC implementations for Gemini models using recording and replay.

## Overview

The aistudio project supports two protocols for communicating with Gemini models:

1. **gRPC**: The default protocol used for all models
2. **WebSocket**: An alternative protocol available for live models

Both protocols have recording and replay capabilities, allowing tests to run without making real API calls.

## Protocol Test Matrix

The protocol test matrix runs tests across different combinations of:

- Models (standard and live models)
- Protocols (WebSocket and gRPC)
- Modes (record and replay)

This comprehensive testing ensures all combinations work correctly.

## Using the Protocol Test Script

The `run_protocol_tests.sh` script provides an easy way to run the protocol test matrix:

```bash
# Run the full test matrix in replay mode
./run_protocol_tests.sh

# Record new WebSocket sessions
export GEMINI_API_KEY=your-api-key-here
./run_protocol_tests.sh --record --protocols ws

# Record new gRPC sessions
export GEMINI_API_KEY=your-api-key-here
./run_protocol_tests.sh --record --protocols grpc

# Record both protocols
export GEMINI_API_KEY=your-api-key-here
./run_protocol_tests.sh --record --protocols ws,grpc

# Test specific models
./run_protocol_tests.sh --models gemini-2.0-flash-live-001,gemini-2.5-flash-live

# Run unit tests only
./run_protocol_tests.sh --unit-only
```

## Recording Formats

### WebSocket Recordings

WebSocket recordings are stored in the `api/testdata/ws_recordings` directory as `.wsrec` files. Each recording is a JSON file containing an array of WebSocket messages with metadata.

### gRPC Recordings

gRPC recordings are stored in the `api/testdata/grpc_recordings` directory as `.replay` files. These are binary files created by the gRPC replay library.

## Environment Variables

The protocol test system uses environment variables to control its behavior:

- `WS_RECORD_MODE=1`: Enable WebSocket recording mode (0 for replay)
- `RECORD_GRPC=1`: Enable gRPC recording mode (0 for replay)
- `AISTUDIO_RUN_E2E_TESTS=1`: Enable end-to-end tests
- `GEMINI_API_KEY`: Your Gemini API key (required for recording)
- `PROTOCOL_MATRIX_RUN_ALL=1`: Run all protocol combinations even if not configured

## Test Files

The protocol testing system consists of:

- **api/protocol_matrix_test.go**: Test matrix for different protocol combinations
- **api/ws_recorder.go**: WebSocket recording and replay implementation
- **api/ws_recorder_test.go**: Tests for the WebSocket recorder
- **api/grpc_replay_test.go**: gRPC recording and replay tests

## CI/CD Integration

For CI/CD pipelines, use the replay mode to run tests without API keys:

```bash
# In CI pipeline
./run_protocol_tests.sh --protocols ws,grpc
```

To update recordings before a release:

```bash
# Update recordings for a release
export GEMINI_API_KEY=your-api-key-here
./run_protocol_tests.sh --record --protocols ws,grpc --clean
```

## Best Practices

1. **Include recordings in version control** for stable CI/CD tests
2. **Update recordings** when the API changes
3. **Use specific prompts** when recording to get consistent responses
4. **Test with both protocols** to ensure compatibility
5. **Use the protocol test matrix** to cover all combinations