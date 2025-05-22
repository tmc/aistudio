# Debugging Guide for aistudio

This guide helps debug connection freezes and other issues with aistudio.

## Common Connection Issues

### Symptoms
- CLI freezes/hangs during startup or operation
- No response from AI service
- Connection timeouts
- gRPC/WebSocket errors

### Quick Diagnosis with pprof

If the CLI is frozen and you started it with `-pprof-server=:6060`, you can diagnose the issue:

```bash
# Check goroutines to see what's blocking
curl -s http://localhost:6060/debug/pprof/goroutine?debug=2

# Get a quick overview
curl -s http://localhost:6060/debug/pprof/
```

Look for goroutines that are:
- Stuck in `IO wait` (network issues)
- Blocked on `chan receive` (waiting for responses)
- In `select` statements for extended periods

## Debugging Environment Variables

Enable detailed debugging with these environment variables:

```bash
# Enable connection debugging (logs connection state, timings)
export AISTUDIO_DEBUG_CONNECTION=true

# Enable stream debugging (logs stream operations)
export AISTUDIO_DEBUG_STREAM=true

# Set custom connection timeout (in seconds, default: 60)
export AISTUDIO_CONNECTION_TIMEOUT=30

# Run with debugging enabled
./aistudio -pprof-server=:6060
```

## Debug Output Interpretation

### Connection Initialization
```
[DEBUG] Created stream context with timeout: 1m0s
[DEBUG] Initializing StreamGenerateContent for model: ...
[DEBUG] Backend: Vertex AI, Project: my-project, Location: us-central1
[DEBUG] Creating Vertex AI genai client for project my-project in us-central1
[DEBUG] Vertex AI genai client created successfully in 2.3s
[DEBUG] Stream initialized successfully in 1.2s (total: 3.5s)
```

### Health Monitoring
```
[DEBUG] Starting connection health monitoring
[DEBUG] Connection health check - Uptime: 5m30s, Since last log: 30s
[DEBUG] Bidirectional stream is active
[DEBUG] App state: AppStateStreaming, Retry attempt: 0
```

### Error Patterns
```
[ERROR] Client initialization failed after 1m0s: context deadline exceeded
[ERROR] Client initialization timed out - this indicates network connectivity issues

[ERROR] Stream initialization failed after 45s: rpc error: code = Unavailable
[ERROR] Stream initialization timed out - this indicates gRPC/WebSocket connectivity issues
```

## Connection Timeout Settings

The application uses these timeout constants (can be overridden with environment variables):

- **DefaultConnectionTimeout**: 60s - Overall connection establishment
- **StreamOperationTimeout**: 30s - Individual stream operations  
- **HealthCheckInterval**: 5m - Health check frequency
- **DebuggingLogInterval**: 30s - Debug log frequency

## Troubleshooting Steps

### 1. Check Network Connectivity
```bash
# Test basic connectivity to Google services
curl -I https://generativelanguage.googleapis.com/
curl -I https://aiplatform.googleapis.com/

# Check DNS resolution
nslookup generativelanguage.googleapis.com
nslookup aiplatform.googleapis.com
```

### 2. Verify Authentication
```bash
# For Gemini API
echo $GEMINI_API_KEY | cut -c1-10  # Should show first 10 chars

# For Vertex AI
gcloud auth application-default print-access-token | head -c 20
```

### 3. Run with Increased Verbosity
```bash
# Enable all debugging
export AISTUDIO_DEBUG_CONNECTION=true
export AISTUDIO_DEBUG_STREAM=true
export AISTUDIO_CONNECTION_TIMEOUT=30

# Run and log output
./aistudio -pprof-server=:6060 2>&1 | tee debug.log
```

### 4. Test with Shorter Timeouts
```bash
# Test with 15-second timeout to fail fast
export AISTUDIO_CONNECTION_TIMEOUT=15
./aistudio
```

## Understanding Goroutine Dumps

Key goroutine patterns in frozen states:

### Normal Operation
```
goroutine 1 [select]:  # Main Bubble Tea event loop
goroutine 65 [syscall]: # Input reading
goroutine 105 [IO wait]: # gRPC frame reading
```

### Problematic States
```
goroutine 105 [IO wait, 2 minutes]: # gRPC stuck reading
goroutine 82 [chan receive, 2 minutes]: # UI waiting for updates
```

The duration (e.g., "2 minutes") indicates how long the goroutine has been blocked.

## Recovery Actions

### Graceful Recovery
```bash
# Send interrupt to allow graceful shutdown
kill -SIGINT <pid>
```

### Force Kill (if needed)
```bash
# Find and kill the process
ps aux | grep aistudio
kill -9 <pid>
```

## Prevention

1. **Use shorter timeouts** in unreliable network environments
2. **Enable debug logging** proactively in CI/automated environments  
3. **Monitor connection health** with the built-in health check system
4. **Use pprof server** (`-pprof-server=:6060`) for production debugging

## Reporting Issues

When reporting connection issues, include:

1. **Environment details**: OS, network setup, proxy configuration
2. **Debug logs**: Output with `AISTUDIO_DEBUG_CONNECTION=true`
3. **pprof output**: Goroutine dump from frozen state
4. **Timing information**: How long before freeze occurs
5. **Reproduction steps**: Exact commands and configuration used

## Advanced Debugging

### Network Tracing
```bash
# Trace network calls (macOS)
sudo dtruss -n aistudio 2>&1 | grep -i connect

# Monitor network connections
lsof -i TCP -a -p $(pgrep aistudio)
```

### Memory and CPU Profiling
```bash
# Get CPU profile while frozen
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=10

# Get heap profile
go tool pprof http://localhost:6060/debug/pprof/heap
```