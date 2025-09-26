# AIStudio User Guide

## Table of Contents
1. [Getting Started](#getting-started)
2. [Installation](#installation)
3. [Configuration](#configuration)
4. [Basic Usage](#basic-usage)
5. [Advanced Features](#advanced-features)
6. [Keyboard Shortcuts](#keyboard-shortcuts)
7. [Tool Usage](#tool-usage)
8. [Troubleshooting](#troubleshooting)
9. [FAQ](#faq)

## Getting Started

AIStudio is a powerful terminal-based AI interface that provides real-time interaction with Google's Gemini models. It supports text, voice, and video inputs with advanced features like tool calling, session history, and multimodal capabilities.

### Prerequisites
- Go 1.21 or later
- Gemini API key or Google Cloud credentials
- Terminal with UTF-8 support
- Optional: Audio player for voice output (aplay, paplay, or ffplay)

## Installation

### From Source
```bash
# Clone the repository
git clone https://github.com/tmc/aistudio.git
cd aistudio

# Install the binary
go install ./cmd/aistudio
```

### Using Go Install
```bash
go install github.com/tmc/aistudio/cmd/aistudio@latest
```

### Verify Installation
```bash
aistudio --version
```

## Configuration

### API Key Setup

#### Option 1: Environment Variable (Recommended)
```bash
export GEMINI_API_KEY="your-api-key-here"
```

#### Option 2: Command Line Flag
```bash
aistudio --api-key "your-api-key-here"
```

#### Option 3: Google Cloud ADC
```bash
gcloud auth application-default login
aistudio --vertex --project-id=your-project
```

### Configuration File

Create `~/.aistudio/config.yaml`:

```yaml
# Model Configuration
model: gemini-2.0-flash-latest
temperature: 0.7
top_p: 0.95
top_k: 40
max_output_tokens: 2048

# Features
enable_tools: true
enable_history: true
enable_audio: false
enable_web_search: false

# UI Settings
require_approval: true
display_token_counts: false

# System Prompt
system_prompt: |
  You are a helpful AI assistant.
  Provide clear, concise, and accurate responses.
```

## Basic Usage

### Starting AIStudio
```bash
# Start in interactive mode
aistudio

# Start with specific model
aistudio --model gemini-2.0-flash-latest

# Start with custom prompt
aistudio --system-prompt "You are a Python expert"
```

### Sending Messages
1. Type your message in the input area
2. Press `Enter` to send
3. Wait for the AI response
4. Continue the conversation

### Multi-line Input
- Press `Alt+Enter` to add a new line without sending
- Useful for code snippets or formatted text

## Advanced Features

### Tool Calling
Tools allow the AI to perform actions like file operations, web searches, and code execution.

```bash
# Enable tools
aistudio --tools

# With tool approval (safer)
aistudio --tools --require-approval

# List available tools
aistudio --list-tools
```

#### Available Default Tools
- **get_weather**: Get weather information
- **search_web**: Search the web (requires --web-search flag)
- **execute_code**: Run code snippets (requires --code-execution flag)

### Session History
Sessions are automatically saved for later reference.

```bash
# Enable history (default)
aistudio --history

# Specify history directory
aistudio --history-dir ~/my-chats

# Disable history
aistudio --no-history
```

### Voice Input/Output
```bash
# Enable audio output
aistudio --audio

# Specify voice
aistudio --audio --voice Puck

# Enable voice input (experimental)
aistudio --voice-input
```

### Live Models with WebSocket
```bash
# Use live model with WebSocket
aistudio --model gemini-2.0-flash-live-001 --ws

# Live model with gRPC (default)
aistudio --model gemini-2.0-flash-live-001
```

### Stdin Mode
Process input from pipes or files:

```bash
# Process file content
cat document.txt | aistudio --stdin

# Process command output
ls -la | aistudio --stdin "Explain these files"

# Chain commands
echo "What is 2+2?" | aistudio --stdin
```

### Auto-send Mode
Automatically send a message after startup:

```bash
# Send after 3 seconds
aistudio --auto-send 3s "Hello, how are you?"

# Immediate send
aistudio --auto-send 0s "Quick question: what's the weather?"
```

## Keyboard Shortcuts

### Navigation
| Key | Action |
|-----|--------|
| `Tab` | Switch focus forward (Input → Messages → Settings) |
| `Shift+Tab` | Switch focus backward (reverse navigation) |
| `↑/↓` | Scroll messages |
| `PgUp/PgDn` | Page up/down in messages |
| `Home/End` | Go to beginning/end of messages |

### Input Controls
| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Alt+Enter` | New line (multi-line input) |
| `Ctrl+U` | Clear input |
| `Ctrl+W` | Delete word |
| `Ctrl+A` | Move to beginning |
| `Ctrl+E` | Move to end |

### Application Controls
| Key | Action |
|-----|--------|
| `Ctrl+C` | Quit application |
| `Ctrl+L` | Clear screen |
| `Ctrl+S` | Toggle settings panel |
| `Ctrl+H` | Save history |
| `Ctrl+T` | Toggle tools |
| `Ctrl+A` | Toggle tool approval |

### Audio Controls (when enabled)
| Key | Action |
|-----|--------|
| `Ctrl+P` | Play/Pause audio |
| `Ctrl+R` | Replay last audio |
| `Ctrl+M` | Mute/Unmute |

## Tool Usage

### Creating Custom Tools

1. Create a JSON file `my_tools.json`:
```json
{
  "tools": [
    {
      "name": "calculate",
      "description": "Perform calculations",
      "parameters": {
        "type": "object",
        "properties": {
          "expression": {
            "type": "string",
            "description": "Mathematical expression"
          }
        },
        "required": ["expression"]
      }
    }
  ]
}
```

2. Load the tools:
```bash
aistudio --tools --tools-file my_tools.json
```

### Tool Approval Flow
When `--require-approval` is enabled:

1. AI requests tool execution
2. Modal appears with tool details
3. Options:
   - `1` or `Y`: Approve once
   - `2`: Approve and auto-approve this tool type
   - `3` or `N` or `Esc`: Deny

## Troubleshooting

### Connection Issues

#### "Context deadline exceeded"
- Check your internet connection
- Verify API key is valid
- Try increasing timeout: `--global-timeout 60s`

#### "Invalid API key"
- Ensure GEMINI_API_KEY is set correctly
- Check for typos in the key
- Verify key has not expired

### Audio Issues

#### No audio output
- Install audio player: `apt install aplay` or `brew install ffmpeg`
- Check audio permissions
- Verify `--audio` flag is set

#### Audio cutting off
- Ensure audio player supports PCM format
- Try different audio player with `--audio-player-command`

### UI Issues

#### Garbled display
- Ensure terminal supports UTF-8
- Try different terminal emulator
- Check TERM environment variable

#### Input not working
- Press `Tab` to focus input area
- Check if modal dialog is open (`Esc` to close)

## FAQ

### Q: Can I use AIStudio with proxy?
A: Yes, set standard proxy environment variables:
```bash
export HTTPS_PROXY=http://proxy.example.com:8080
export HTTP_PROXY=http://proxy.example.com:8080
```

### Q: How do I use different models?
A: List available models and select one:
```bash
# List models
aistudio --list-models

# Use specific model
aistudio --model gemini-1.5-pro
```

### Q: Can I export conversations?
A: Yes, sessions are saved as JSON in the history directory:
```bash
# Default location
ls ~/.aistudio/history/

# Export specific session
cat ~/.aistudio/history/session_*.json > conversation.json
```

### Q: Is my data secure?
A:
- API keys are never logged or saved
- History is stored locally only
- Use `--no-history` for sensitive conversations
- See SECURITY.md for best practices

### Q: Can I use AIStudio in scripts?
A: Yes, use stdin mode:
```bash
#!/bin/bash
result=$(echo "$1" | aistudio --stdin --model gemini-2.0-flash-latest)
echo "AI says: $result"
```

### Q: How do I report bugs?
A:
1. Check existing issues: https://github.com/tmc/aistudio/issues
2. Create new issue with:
   - AIStudio version (`aistudio --version`)
   - Go version (`go version`)
   - Error message
   - Steps to reproduce

### Q: Can I contribute?
A: Yes! See CONTRIBUTING.md for guidelines:
- Fork the repository
- Create feature branch
- Add tests
- Submit pull request

## Support

- **Documentation**: https://github.com/tmc/aistudio/wiki
- **Issues**: https://github.com/tmc/aistudio/issues
- **Discussions**: https://github.com/tmc/aistudio/discussions
- **Security**: See SECURITY.md for reporting vulnerabilities

---

*Last Updated: September 2025*
*Version: 1.0.0*