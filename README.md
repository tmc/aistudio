# aistudio - Enhanced AI Interface with MCP, Voice & Video

This application provides a comprehensive terminal-based interface for AI interactions, featuring:

- **Real-time streaming** with Google's Gemini Live API
- **MCP (Model Context Protocol)** integration for tool sharing and interoperability
- **Voice streaming** with bidirectional speech-to-text and text-to-speech
- **Video streaming** with camera input and screen capture
- **Multimodal capabilities** combining text, voice, and video
- **Advanced tool calling** with approval workflows
- **Collaboration features** via MCP server/client architecture

Built on Gemini Live's bidirectional streaming capabilities and extended with modern AI interaction patterns.

## Features

### Core Capabilities
*   **Real-time streaming** with Gemini Live API (`BidiGenerateContent`)
*   **Advanced tool calling** with approval workflows and auto-approval options
*   **Session history** with automatic saving and restoration
*   **Rich terminal UI** with scrollable chat, settings panel, and status indicators

### MCP Integration
*   **MCP Server** - Expose aistudio tools via MCP protocol
*   **MCP Client** - Connect to external MCP servers and import their tools
*   **Multiple transports** - HTTP, WebSocket, and stdio support
*   **Tool bridging** - Seamless integration between aistudio and MCP tools

### Voice Streaming
*   **Bidirectional voice** - Simultaneous speech input and audio output
*   **Real-time STT** - Speech-to-text with voice activity detection
*   **Streaming TTS** - Text-to-speech with voice selection and effects
*   **Audio processing** - Noise reduction, echo cancellation, spatial audio
*   **Voice controls** - `Ctrl+I` (input), `Ctrl+O` (output), `Ctrl+B` (bidirectional)

### Video Streaming
*   **Camera input** - Live camera feed processing and frame extraction
*   **Screen capture** - Real-time screen sharing with privacy controls
*   **Video effects** - Real-time processing, overlays, and enhancement
*   **Object detection** - Face detection, motion tracking, object recognition
*   **Video controls** - `Ctrl+V` to cycle through camera/screen/off modes

### Multimodal Features
*   **Simultaneous modes** - Text + voice + video in real-time
*   **Context fusion** - Integrated processing of multiple input streams
*   **Adaptive quality** - Dynamic adjustment based on system capabilities
*   **Collaboration** - Multi-user sessions via MCP

## Setup

1.  **Go:** Ensure you have Go installed (version 1.21 or later recommended).
2.  **API Key:**
    *   Obtain a Gemini API key from [Google AI Studio](https://aistudio.google.com/).
    *   Set the `GEMINI_API_KEY` environment variable:
        ```bash
        export GEMINI_API_KEY="YOUR_API_KEY"
        ```
    *   Alternatively, pass the key directly using `aistudio.WithAPIKey("YOUR_API_KEY")`.
    *   If no API key is provided, the application will attempt Application Default Credentials (ADC). Run `gcloud auth application-default login` if needed.
3.  **Audio Player (Optional):**
    *   If you enable audio output (`WithAudioOutput(true)`), you need a command-line audio player capable of reading raw PCM audio from stdin.
    *   The application attempts to auto-detect `aplay` (Linux), `paplay` (Linux/PulseAudio), or `ffplay` (Linux/macOS/Windows - requires FFmpeg installation).
    *   Ensure one of these is installed and in your `PATH`, or specify a custom command using `WithAudioPlayerCommand()`.

## Installation

go install github.com/tmc/aistudio/cmd/aistudio

## Usage

### Basic Usage
```bash
# Standard text-based interaction
aistudio

# With voice input/output
aistudio --voice

# With camera input
aistudio --camera

# With screen capture
aistudio --screen

# Full multimodal mode
aistudio --multimodal

# As MCP server
aistudio --mcp-server --port 8080

# Connect to MCP clients
aistudio --mcp-clients config.yaml
```

### Advanced Usage
```bash
# Collaboration mode (MCP + voice + video)
aistudio --collaboration

# Voice-only mode with custom settings
aistudio --voice-input --voice-output --voice-bidirectional

# Video with effects and recording
aistudio --video --video-effects --video-record

# MCP server with specific transports
aistudio --mcp-server --mcp-transports http,stdio --mcp-port 8080

# Custom multimodal configuration
aistudio --multimodal --voice-provider gemini --video-quality high
```

### Keyboard Controls

#### Core Controls
- **Enter**: Send message
- **Ctrl+C**: Quit application
- **Ctrl+S**: Toggle settings panel
- **Ctrl+T**: Show available tools
- **Ctrl+A**: Toggle tool approval requirement
- **Ctrl+H**: Save chat history

#### Voice Controls
- **Ctrl+I**: Toggle voice input
- **Ctrl+O**: Toggle voice output  
- **Ctrl+B**: Toggle bidirectional voice
- **Ctrl+M**: Toggle microphone

#### Video Controls
- **Ctrl+V**: Cycle video input (camera → screen → off)

#### MCP Controls
- **Ctrl+E**: Show MCP integration status

### Integration Examples

#### Voice Interaction
1. Start with voice enabled: `aistudio --voice`
2. Press `Ctrl+B` to start bidirectional mode
3. Speak your questions - they'll be transcribed and sent
4. Listen to AI responses with text-to-speech

#### Video Interaction  
1. Start with camera: `aistudio --camera`
2. Press `Ctrl+V` to switch between camera/screen capture
3. Share your screen or camera feed with the AI
4. Get visual analysis and responses

#### MCP Server Mode
1. Start as MCP server: `aistudio --mcp-server --port 8080`
2. Connect external MCP clients to access aistudio's tools
3. Share capabilities across different AI applications
4. Use `Ctrl+E` to monitor connection status

#### Collaboration
1. Start collaboration mode: `aistudio --collaboration`
2. Multiple users can connect via MCP
3. Share screen, voice chat, and AI interactions
4. Coordinated AI assistance across team members


## Development

Tidy dependencies
```shell
go mod tidy
```

# Run tests
```shell
# Run regular tests
go test ./...

# Run end-to-end connectivity tests (requires API key)
GEMINI_API_KEY=your_key go test -v -run TestE2E
```

# Run linter/vet
```shell
go vet ./...
```

## Testing

### Unit Tests
The project includes unit tests for core functionality. Run them with:
```shell
go test ./...
```

### End-to-End Tests
End-to-end tests verify connectivity with the actual Gemini API:

```shell
# Run only E2E connection tests
GEMINI_API_KEY=your_key go test -v -run TestE2EConnection

# Skip specific stability tests that take longer
GEMINI_API_KEY=your_key go test -v -run TestE2EConnection/BinaryInitialization
```

The E2E tests include:
- Binary initialization test (verifies connection setup)
- Message sending test (verifies proper message exchange)
- Connection stability test (verifies keepalive during idle periods)
- Library connection test (verifies API client works directly)

