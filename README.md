# aistudio - Gemini Live Streaming TUI

This application provides a terminal-based chat interface for interacting with Google's Generative Language API (specifically the `v1alpha` version supporting bidirectional streaming), aiming to replicate some aspects of the AI Studio "Stream Realtime" feature.

It allows you to have a live, streaming conversation with a Gemini model (defaults to `gemini-1.5-flash-latest`), displaying text responses and optionally playing back generated audio using an external command-line player. It also includes UI indicators for simulating microphone or camera input modes.

## Features

*   Real-time, bidirectional streaming chat with Gemini models via the `BidiGenerateContent` API.
*   Text-based input and output display in a scrollable viewport.
*   **Text-to-Audio Output:** Optionally streams generated audio responses to an external player (e.g., `aplay`, `ffplay`).
*   **Simulated Multimodal Input:** UI toggles (`Ctrl+M`, `Ctrl+V`) and status indicators for Microphone and Camera/Screen input modes (actual data capture/sending not implemented).
*   **Chat History Management:** Save and load chat sessions (`Ctrl+H` to save manually). [Learn more](docs/CHAT_HISTORY.md)
*   **Tool Calling Support:** Enable Gemini to call tools and functions defined in a JSON file. [Learn more](docs/TOOL_CALLING.md)
*   **System Prompts:** Set custom system prompts to guide the model's behavior. [Learn more](docs/SYSTEM_PROMPTS.md)
*   Automatic detection of common audio players (Linux/macOS).
*   Loading indicator while waiting for responses.
*   Basic error handling and display.
*   Configurable API Key, Model Name, Audio Output, Voice, and Player Command via functional options.

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

```bash
go install github.com/tmc/aistudio/cmd/aistudio
```

## Usage

```bash
aistudio [options]
```

### Command Line Options

```
Usage: aistudio [options]
Interactive chat with Gemini Live Streaming API.

Options:
  -api-key string
        Gemini API Key (overrides GEMINI_API_KEY env var).
  -audio
        Enable audio output. (default true)
  -filter-models string
        Filter models list (used with --list-models)
  -history
        Enable chat history. (default true)
  -history-dir string
        Directory for storing chat history. (default "./history")
  -list-models
        List available models and exit.
  -model string
        Gemini model ID to use. (default "gemini-1.5-flash-latest")
  -player string
        Override command for audio playback (e.g., 'ffplay ...'). Auto-detected if empty.
  -pprof-cpu file
        Write cpu profile to file
  -pprof-mem file
        Write memory profile to file
  -pprof-server string
        Enable pprof HTTP server on given address (e.g., 'localhost:6060')
  -pprof-trace file
        Write execution trace to file
  -system-prompt string
        System prompt to use for the conversation.
  -system-prompt-file string
        Load system prompt from a file.
  -tools
        Enable tool calling support. (default true)
  -tools-file string
        JSON file containing tool definitions to load.
  -voice string
        Voice for audio output (e.g., Puck, Amber). (default "Puck")

Environment Variables:
  GEMINI_API_KEY: API Key (used if --api-key is not set).
```

### Keyboard Controls

* **Enter**: Send the message.
* **Ctrl+M**: Toggle the (simulated) Microphone input state.
* **Ctrl+V**: Toggle the (simulated) Camera/Screen input state.
* **Ctrl+H**: Save chat history (when history is enabled).
* **Ctrl+T**: Show available tools (when tools are enabled).
* **Ctrl+C**: Quit the application.

### Documentation

For more detailed information on specific features, see:

* [Chat History](docs/CHAT_HISTORY.md)
* [Tool Calling](docs/TOOL_CALLING.md)
* [System Prompts](docs/SYSTEM_PROMPTS.md)

## Development

```shell
# Tidy dependencies
go mod tidy

# Run tests
go test ./...

# Run linter/vet
go vet ./...
```

## Profiling

The application includes built-in profiling capabilities:

```shell
# CPU profile
aistudio --pprof-cpu=cpu.prof

# Memory profile
aistudio --pprof-mem=mem.prof

# Execution trace
aistudio --pprof-trace=trace.out

# HTTP server for live profiling
aistudio --pprof-server=localhost:6060
# Then visit http://localhost:6060/debug/pprof/
```
