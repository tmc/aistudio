# aistudio - Gemini Live Streaming TUI

This application provides a terminal-based chat interface for interacting with Google's Generative Language API (specifically the `v1alpha` version supporting bidirectional streaming), aiming to replicate some aspects of the AI Studio "Stream Realtime" feature.

It allows you to have a live, streaming conversation with a Gemini model (defaults to `gemini-1.5-flash-latest`), displaying text responses and optionally playing back generated audio using an external command-line player. It also includes UI indicators for simulating microphone or camera input modes.

## Features

*   Real-time, bidirectional streaming chat with Gemini models via the `BidiGenerateContent` API.
*   Text-based input and output display in a scrollable viewport.
*   **Text-to-Audio Output:** Optionally streams generated audio responses to an external player (e.g., `aplay`, `ffplay`).
*   **Simulated Multimodal Input:** UI toggles (`Ctrl+M`, `Ctrl+V`) and status indicators for Microphone and Camera/Screen input modes (actual data capture/sending not implemented).
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

go install github.com/tmc/aistudio/cmd/aistudio

## Usage

```bash
aistudio
```
Interaction:

Type your message in the input area at the bottom.

Press Enter to send the message.

Press Ctrl+M to toggle the (simulated) Microphone input state.

Press Ctrl+V to toggle the (simulated) Camera/Screen input state.

Press Ctrl+C to quit.

Text responses appear in the chat history.

If audio output is enabled, generated speech plays automatically.

Notes on Interpretation


## Development

Tidy dependencies
```shell
go mod tidy
```

# Run tests
```shell
go test ./...
```

# Run linter/vet
```shell
go vet ./...
```

