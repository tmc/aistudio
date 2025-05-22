# WebSocket Implementation for Gemini Live Models

This document describes the WebSocket implementation for Gemini live models in aistudio. The implementation adds support for Gemini 2.0 and 2.5 live models, which use WebSocket for communication instead of gRPC.

## Overview

The WebSocket implementation consists of:

1. **Live model detection** - Identifies models that should use WebSocket (models with "live" in their name)
2. **WebSocket client** - Implements the WebSocket protocol for Gemini Live API
3. **Adapter** - Converts WebSocket messages to/from gRPC format
4. **WebSocket flag** - Controls whether to use WebSocket for live models (--ws flag)

## Using WebSocket Mode

WebSocket mode is disabled by default. To enable it:

```bash
# Use WebSocket for live models (e.g., gemini-2.0-flash-live-001, gemini-2.5-flash-live)
aistudio --model gemini-2.5-flash-live --ws
```

When WebSocket mode is enabled:
- Live models will use WebSocket protocol
- Non-live models will continue to use gRPC
- If a live model is used without the --ws flag, it will use gRPC

## Programmatic Usage

When using the aistudio library programmatically, you can enable WebSocket mode with the `WithWebSocket` option:

```go
// Enable WebSocket mode for live models
model, err := aistudio.NewModel(modelName, apiKey, aistudio.WithWebSocket(true))
```

## Testing

The implementation includes several test files:

1. **live_ws_adapter_test.go** - Tests WebSocket adapter functionality
2. **live_model_test.go** - Tests live model detection
3. **grpc_replay_test.go** - Tests with recorded gRPC sessions

To run all WebSocket-related tests:

```bash
# Run all WebSocket-related tests
go test ./api -run "TestLiveStream|TestLiveModel|TestWithGRPCReplay|TestWebSocket"
```

To run gRPC replay tests specifically:

```bash
# Run in replay mode (default)
./run_grpc_replay_tests.sh

# Run in record mode (requires API key)
export GEMINI_API_KEY=your-api-key-here
./run_grpc_replay_tests.sh --record
```

## Implementation Details

### Live Model Detection

The `IsLiveModel` function in `api/live_client.go` detects live models:

```go
func IsLiveModel(modelName string) bool {
    modelName = strings.ToLower(modelName)
    return strings.Contains(modelName, "live") &&
        (strings.Contains(modelName, "gemini-2.0") || 
         strings.Contains(modelName, "gemini-2.5"))
}
```

### WebSocket Client

The WebSocket client in `api/live_client.go` handles:
- Connection establishment
- Authentication
- Message serialization/deserialization
- Error handling

### Protocol Selection

The protocol selection logic in `api/client.go` checks both the model name and WebSocket flag:

```go
// Check if WebSocket mode is enabled AND if this is a live model
if config.EnableWebSocket && IsLiveModel(config.ModelName) {
    // Use WebSocket
    return c.initLiveStream(ctx, config)
}
// Use gRPC
```

## Troubleshooting

If you encounter issues with WebSocket mode:

1. Ensure you're using a live model (e.g., gemini-2.0-flash-live-001, gemini-2.5-flash-live)
2. Verify you've enabled the --ws flag
3. Check for network connectivity issues
4. Look for error messages in the console output

## Known Limitations

- gRPC replay testing doesn't support WebSocket mode directly
- WebSocket doesn't support all gRPC features (e.g., metadata)