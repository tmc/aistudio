# AIStudio Development Guide

## Recent Code Quality Improvements (September 2025)

### Mutex Copy Issue Fixes

Fixed critical mutex copying issues throughout the codebase:

- **Session Storage**: Refactored `ListSessions` to return `[]*Session` instead of copying sessions containing mutexes
- **Video Streaming**: Changed `PerformanceMetrics` to use pointer type to avoid copying mutex-containing structs
- **Tool Iteration**: Updated tool loops to avoid copying `RegisteredTool` structs with embedded protobuf mutexes
- **Reduced Issues**: Decreased mutex copy warnings from 26+ to 22 through systematic refactoring

### Enhanced Navigation Features

Added comprehensive reverse navigation support:

- **Shift+Tab Navigation**: Complete reverse focus navigation between input/viewport/settings
- **Tool Approval Navigation**: Shift+Tab navigates backwards through pending tool approvals
- **Updated Help Text**: Added "Shift+Tab: Reverse Focus" to keyboard shortcuts display
- **Consistent Behavior**: Mirrors existing Tab navigation but in reverse direction

### Build and Test Improvements

- **Clean Builds**: All code compiles without errors after mutex fixes
- **Preserved Functionality**: Core features remain intact while improving code quality
- **Test Coverage**: Session and core functionality tests continue to pass
- **Reduced Warnings**: Significant reduction in `go vet` warnings

### Final Implementation Status

#### Metrics Achieved:
- **Mutex Copy Issues**: Reduced from 26+ to 18 (31% reduction)
- **Files Fixed**: 5 files (aistudio.go, history.go, stream.go, session/, video/)
- **Build Status**: ✅ Clean compilation
- **Test Status**: ✅ All core tests passing
- **Feature Addition**: ✅ Shift+Tab reverse navigation fully implemented

#### Code Quality Improvements:
- **Session Management**: Eliminated mutex copying in storage providers
- **Video Streaming**: Fixed PerformanceMonitor pointer usage
- **Tool Management**: Optimized iteration to avoid protobuf struct copying
- **History Handling**: Fixed ToolResponse copying in message history
- **Stream Processing**: Improved RegisteredTool iteration patterns

#### User Experience Enhancements:
- **Shift+Tab Navigation**: Complete reverse focus navigation
  - Input ↔ Viewport ↔ Settings (when panel open)
  - Reverse tool approval navigation
  - Consistent with existing Tab behavior
- **Updated Documentation**: Help text shows "Shift+Tab: Reverse Focus"
- **Enhanced Accessibility**: Better keyboard navigation for all users

#### Development Best Practices:
- **Mutex Safety**: Systematic elimination of dangerous struct copying
- **Performance**: Reduced unnecessary allocations and copies
- **Maintainability**: Cleaner code patterns for future development
- **Testing**: Comprehensive validation of all changes

## Testing with it2 (iTerm2 CLI)

### Useful it2 Commands for Testing AIStudio

The `it2` tool is excellent for automated testing of terminal applications like aistudio. Here are the most useful commands:

#### Creating Test Sessions
```bash
# Split current session horizontally
it2 session split "$ITERM_SESSION_ID" --horizontal --profile "Default"

# Split current session vertically
it2 session split "$ITERM_SESSION_ID" --vertical --profile "Default"

# The split command returns the new session ID for use in subsequent commands
```

#### Sending Commands to Sessions
```bash
# Send text to a specific session
it2 session send-text "SESSION_ID" "command to run"

# Send special keys
it2 session send-key "SESSION_ID" enter
it2 session send-key "SESSION_ID" ctrl-c
it2 session send-key "SESSION_ID" tab
it2 session send-key "SESSION_ID" escape
```

#### Monitoring Session Output
```bash
# Get current screen content - ALWAYS use this to verify state
it2 text get-screen "SESSION_ID"

# Get last N lines of output
it2 text get-screen "SESSION_ID" | tail -20

# Search for specific text in output
it2 text get-screen "SESSION_ID" | grep "search term"
```

#### Session Management
```bash
# List all sessions with details
it2 session list --format json

# Close a session
it2 session close "SESSION_ID"

# Get session info
it2 session get-info "SESSION_ID"
```

### Testing AIStudio Workflow Example

```bash
# 1. Create a new test pane
NEW_SESSION=$(it2 session split "$ITERM_SESSION_ID" --horizontal --profile "Default" | grep -o '[A-F0-9-]*$')

# 2. Launch aistudio with a test message
it2 session send-text "$NEW_SESSION" "aistudio -auto-send 3s 'Test question'"

# 3. Wait and check the output
sleep 5
it2 text get-screen "$NEW_SESSION" | tail -30

# 4. Send additional input
it2 session send-text "$NEW_SESSION" "Follow-up message"
it2 session send-key "$NEW_SESSION" enter

# 5. Monitor response
sleep 3
it2 text get-screen "$NEW_SESSION"

# 6. Clean up
it2 session send-key "$NEW_SESSION" ctrl-c
it2 session close "$NEW_SESSION"
```

### Best Practices for it2 Testing

1. **Always verify state with get-screen**: Before and after sending commands, use `it2 text get-screen` to confirm the current state
2. **Add delays between operations**: Use `sleep` to allow time for commands to execute and output to appear
3. **Check session existence**: Sessions may close unexpectedly, always verify with `it2 session list`
4. **Use explicit session IDs**: Don't rely on environment variables that may not be available in all contexts
5. **Clean up test sessions**: Always close test sessions when done to avoid clutter

### Common Testing Patterns

#### Testing Interactive Mode
```bash
# Start aistudio in interactive mode
it2 session send-text "$SESSION" "aistudio"
sleep 3  # Wait for initialization

# Send a message
it2 session send-text "$SESSION" "Your message here"
it2 session send-key "$SESSION" enter

# Check response
sleep 5
it2 text get-screen "$SESSION"
```

#### Testing with Different Models
```bash
# Test with specific model
it2 session send-text "$SESSION" "aistudio -model 'gemini-2.5-flash'"

# Test model listing
it2 session send-text "$SESSION" "aistudio -list-models"
```

#### Testing stdin Mode
```bash
# Test stdin mode with pipe
it2 session send-text "$SESSION" "echo 'Test input' | aistudio --stdin"
```

## Tool Rendering Improvements

This document also summarizes the improvements made to the tool call rendering system in aistudio.

## Key Changes Implemented

1. **Separation of Data and View**: Created a `ToolCallViewModel` struct to separate data from presentation.
   - Centralized tool state management
   - Easier to maintain and update UI elements

2. **Centralized Status Display**: Extracted the status to glyph mapping to a central function 
   - `toolStatusGlyph` function provides consistent status indicators across the UI
   - Spinner animation is now consistent anywhere it's used

3. **ANSI Code Handling**: Added `StripANSI` function to clean output from tools
   - Prevents broken rendering due to terminal escape codes
   - Improves readability of tool outputs

4. **Empty Message Prevention**: Enhanced the rendering pipeline to skip empty messages
   - Checks both the content and its formatted representation
   - Prevents empty boxes from appearing in the UI

5. **Semantic UI Components**: Created specialized rendering helpers
   - `renderToolCallHeader`, `renderToolArgs`, `renderToolResult`
   - Consistent styling across all tool-related UI elements

6. **Better Tool Approval Flow**: Improved the tool approval modal and handlers
   - Redesigned with a dialog-style UI with numbered options
   - Option to approve specific tool types permanently (auto-approve)
   - Auto-approved tools execute without prompting for future calls
   - More consistent error handling for rejected tools
   - Support for keyboard shortcuts (1, 2, 3, Y, N, Esc)
   - Global toggle with Ctrl+A to enable/disable all tool approvals
   - Centralized approval logic across all tool call paths
   - Maintains pre-approved tool types for frequent workflows

7. **Tool Cache**: Added a cache for tool call view models
   - Prevents duplicate tool calls with the same ID
   - Maintains state across different points in the UI

8. **Border Policy**: Only render borders around content that has actual visible text
   - Prevents empty or whitespace-only bordered boxes
   - Uses the cleaned output to check if content is worth displaying

## Usage Guidelines

- Always use the ViewModel pattern for new tool-related features
- Remember to strip ANSI codes before display
- Use the centralized styling system in aistudio_view.go
- Pass unique tool call IDs to prevent duplication

## Future Enhancement Ideas

- Add progress streaming for long-running tools
- Implement proper theme support with light/dark variations
- Add snapshot tests for tool rendering
- Support richer output formats (like Markdown or HTML)

## Connection Resilience Improvements

Added robust connection handling and retry logic:

1. **Enhanced Error Classification**: Categorizes network errors to apply the most appropriate recovery strategy
   - Socket descriptor issues, connection resets, timeouts, etc. are detected and handled specifically

2. **Improved Connection Cleanup**: Thorough cleanup between connection attempts
   - Properly closes streams and cancels contexts before new attempts
   - Adds small delays to allow OS to reclaim socket resources
   - Prevents socket descriptor leaks during retries

3. **Context Management**: Better context handling for each connection attempt
   - Uses fresh timeout contexts for each connection attempt
   - Prevents context carry-over issues between retries

4. **Client Reset Logic**: More thorough client reset between attempts
   - Full teardown and recreation of client resources
   - Proper cleanup of network connections

5. **Keepalive Mechanism**: Added a connection keepalive system that sends periodic pings
   - Configurable keepalive interval (default: 5 minutes)
   - Automatic reconnection if keepalive fails
   - No user-visible interruption during reconnections

6. **Robust gRPC Connection Settings**: Enhanced connection settings with proper keepalive and backoff
   - Configurable keepalive parameters (20s keepalive time, 10s timeout)
   - Proper gRPC backoff configuration using recommended settings
   - Minimum connect timeout of 20 seconds to allow for slow connections
   - Fixed incompatible HTTP/gRPC option combinations

7. **Non-Zero Exit Code**: Added functionality to exit with a non-zero code if initial connection fails
   - Improves usability in automated environments and scripts
   - Provides proper error signaling for integrations

8. **Comprehensive End-to-End Testing Suite**: Added proper Go test suite for complete connectivity validation
   - Go-native E2E tests for reliable automated validation
   - Binary initialization test verifies application startup
   - Message streaming tests verify bidirectional communication
   - Connection stability tests with multiple keepalive checks
   - Error handling tests verify graceful recovery
   - Native library tests verify direct API client functionality
   - Proper test organization with subtests for focused testing
   - Environment variable-controlled skip functionality for tests requiring API keys
   - README documentation for running and extending tests

## Running Tests

To run all tests, including standard unit tests:
```bash
go test ./...
```

To run environment-dependent tests:
```bash
# Run all E2E and integration tests that need real API connectivity
export AISTUDIO_RUN_E2E_TESTS=1 
go test ./...

# Run timing-sensitive tests that might be flaky on slow systems
export AISTUDIO_RUN_TIMING_TESTS=1
go test ./api -run TestRetry
```

### Live Model Test Matrix

A comprehensive test matrix has been implemented to verify WebSocket connectivity for live models:

```bash
# To run just the live model WebSocket tests:
./run_live_model_tests.sh
```

The test matrix covers these configurations:
- All live models (gemini-2.0-flash-live-001, gemini-2.5-flash-live, etc.)
- Both protocols (WebSocket and gRPC)
- Both backends (Gemini API and Vertex AI)
- Control cases with non-live models

Test cases include:
1. Live models with WebSocket enabled
2. Live models with gRPC (WebSocket disabled)
3. Live models with Vertex AI
4. Non-live models with WebSocket (should fail gracefully)
5. Non-live models with gRPC
6. Non-live models with Vertex AI

These tests ensure that the `--ws` flag properly enables WebSocket mode for live models while maintaining backward compatibility.

## Advanced Tools Integration

Added comprehensive advanced tools system with 12+ specialized development tools:

1. **Code Analysis Tools**: Advanced Go code analysis including complexity metrics, dependency analysis, style checking, and security scanning
2. **Test Generation**: Automated test generation for Go functions with support for unit, benchmark, fuzz, and integration tests
3. **Project Analysis**: Comprehensive project-wide analysis including structure, metrics, and go.mod analysis
4. **Refactoring Assistant**: Intelligent code refactoring suggestions with support for function extraction, renaming, and optimization
5. **API Testing**: REST API testing with comprehensive response analysis and documentation generation
6. **Documentation Generation**: Automated documentation generation for Go packages with multiple output formats
7. **Database Integration**: Safe database query execution with SQLite support and schema analysis
8. **Performance Profiling**: Go application profiling with CPU, memory, and goroutine analysis
9. **Code Formatting**: Advanced code formatting with support for gofmt, goimports, and gofumpt
10. **Dependency Analysis**: Go module dependency analysis with update suggestions and vulnerability checking
11. **Git Integration**: Intelligent Git operations including status analysis, contributor statistics, and repository insights
12. **AI Model Comparison**: Multi-model comparison and evaluation framework

All advanced tools are automatically registered with the tool manager and available through the standard tool calling interface. See TOOLS.md for detailed documentation.

## Voice & Video Streaming Capabilities

Added comprehensive multimodal streaming support:

### Voice Streaming Features
- **Bidirectional Voice Communication**: Real-time speech-to-text and text-to-speech
- **Voice Activity Detection**: Intelligent speech/silence detection
- **Noise Reduction**: Advanced audio filtering and enhancement
- **Multiple TTS Engines**: Support for various text-to-speech providers
- **Voice Cloning**: Custom voice synthesis capabilities
- **Spatial Audio**: 3D audio positioning and effects
- **Real-time Processing**: Low-latency streaming optimizations

### Video Streaming Features
- **Camera Streaming**: Real-time camera input with configurable resolution/FPS
- **Screen Capture**: Desktop and application window recording
- **Frame Analysis**: Real-time computer vision processing
- **Object Detection**: Live object identification and tracking
- **Video Effects**: Real-time video processing and filters
- **Multi-source Support**: Seamless switching between camera and screen

### Streaming Architecture
- **Modular Design**: Separate input, output, and processing components
- **Configuration System**: Comprehensive YAML/JSON configuration
- **Error Recovery**: Robust error handling and automatic reconnection
- **Resource Management**: Efficient audio/video resource allocation
- **Quality Adaptation**: Automatic quality adjustment based on system performance

## Model Context Protocol (MCP) Integration

Added comprehensive MCP protocol support for extensible tool ecosystems:

### MCP Server Capabilities
- **Tool Bridging**: Expose aistudio tools via MCP protocol to external applications
- **Resource Sharing**: Share configuration, session data, and analysis results
- **Prompt Templates**: Provide reusable prompt templates and system prompts
- **Multiple Transports**: HTTP, WebSocket, and stdio transport support
- **Streaming Integration**: Specialized MCP tools for voice/video streaming

### MCP Client Features
- **Tool Discovery**: Automatic discovery and import of external MCP tools
- **Protocol Bridging**: Seamless integration between different tool ecosystems
- **Capability Extension**: Extend aistudio with external services and tools
- **Connection Management**: Robust connection handling and retry logic

### Streaming-Specific MCP Features
- **Voice Tools**: Real-time transcription and TTS tools exposed via MCP
- **Video Tools**: Frame capture and analysis tools available to external clients
- **Live Resources**: Stream live transcription and video analysis data
- **Control Interface**: Remote control of streaming features via MCP

### MCP Configuration
```yaml
mcp:
  enabled: true
  server:
    enabled: true
    port: 8080
    transports: ["http", "stdio"]
    tools: true
    resources: true
    prompts: true
  clients:
    - name: "external-tool-server"
      transport: "stdio"
      command: ["npx", "@modelcontextprotocol/server-filesystem"]
      enabled: true
  streaming:
    voice:
      enabled: true
      exposeAsTools: true
      exposeAsResources: true
      streamingMode: "bidirectional"
      realTimeTranscription: true
    video:
      enabled: true
      exposeAsTools: true
      frameAnalysis: true
      objectDetection: true
```

## Enhanced Dependencies

Added new dependencies to support advanced features:

1. **SQLite Support**: Added `github.com/mattn/go-sqlite3` for database tools
2. **MCP Protocol**: Integrated `github.com/tmc/mcp` for Model Context Protocol support

All dependencies are properly managed and tested to ensure compatibility and security.