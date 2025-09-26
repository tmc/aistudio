# AIStudio API Documentation

## Overview
AIStudio provides both a Go library API and a command-line interface. This document covers the programmatic API for embedding AIStudio in your applications.

## Installation
```go
import "github.com/tmc/aistudio"
```

## Core Types

### Model
The main application model implementing the Bubble Tea Model interface.

```go
type Model struct {
    // Configuration
    modelName       string
    temperature     float32
    topP            float32
    topK            int32
    maxOutputTokens int32
    systemPrompt    string

    // Features
    enableTools     bool
    enableHistory   bool
    enableAudio     bool
    requireApproval bool

    // State
    currentState    AppState
    messages        []Message

    // Managers
    toolManager     *ToolManager
    historyManager  *HistoryManager
}
```

### Message
Represents a single message in the conversation.

```go
type Message struct {
    Sender         string          // "user", "model", "system", "tool"
    Content        string          // Text content
    Timestamp      time.Time       // When message was created
    TokenCount     int             // Token usage
    AudioData      []byte          // Optional audio data
    HasAudio       bool            // Audio presence flag
    ToolCall       *ToolCall       // Associated tool call
    ToolResponse   *ToolResponse   // Tool execution result
    ExecutableCode *ExecutableCode // Code to execute
}
```

### AppState
Application state enumeration.

```go
type AppState string

const (
    AppStateInitializing  AppState = "initializing"
    AppStateReady        AppState = "ready"
    AppStateWaiting      AppState = "waiting"
    AppStateProcessingTool AppState = "processing_tool"
    AppStateError        AppState = "error"
    AppStateQuitting     AppState = "quitting"
)
```

## Creating an AIStudio Instance

### Basic Usage
```go
package main

import (
    "github.com/tmc/aistudio"
)

func main() {
    // Create with default settings
    model := aistudio.New()

    // Create with options
    model := aistudio.New(
        aistudio.WithModel("gemini-2.0-flash-latest"),
        aistudio.WithAPIKey("your-api-key"),
        aistudio.WithTemperature(0.7),
        aistudio.WithToolsEnabled(true),
    )
}
```

### Available Options

#### Model Configuration
```go
// Set the model name
WithModel(name string) Option

// Set temperature (0.0-1.0)
WithTemperature(temp float32) Option

// Set top-p value
WithTopP(p float32) Option

// Set top-k value
WithTopK(k int32) Option

// Set max output tokens
WithMaxOutputTokens(tokens int32) Option

// Set system prompt
WithSystemPrompt(prompt string) Option
```

#### Feature Flags
```go
// Enable/disable tools
WithToolsEnabled(enabled bool) Option

// Enable/disable history
WithHistoryEnabled(enabled bool) Option

// Enable/disable audio
WithAudioOutput(enabled bool) Option

// Require tool approval
WithToolApprovalRequired(required bool) Option

// Enable web search
WithWebSearch(enabled bool) Option

// Enable code execution
WithCodeExecution(enabled bool) Option
```

#### Backend Configuration
```go
// Use Vertex AI
WithVertexAI(projectID, location string) Option

// Set API key
WithAPIKey(key string) Option

// Use custom backend
WithBackend(backend string) Option

// Enable WebSocket for live models
WithWebSocket(enabled bool) Option
```

#### Auto-send Configuration
```go
// Auto-send message after duration
WithAutoSend(duration time.Duration, message string) Option
```

## Tool Management

### Registering Tools
```go
// Get the tool manager
tm := model.GetToolManager()

// Register a custom tool
err := tm.RegisterTool(
    "my_tool",                    // Name
    "Description of my tool",      // Description
    toolParameterSchema,           // Parameters schema
    myToolHandler,                 // Handler function
)

// Tool handler signature
type ToolHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)
```

### Tool Parameter Schema
```go
import "google.golang.org/protobuf/types/known/structpb"

// Create parameter schema
params, _ := structpb.NewStruct(map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "input": map[string]interface{}{
            "type":        "string",
            "description": "Input parameter",
        },
    },
    "required": []string{"input"},
})
```

### Example Tool Implementation
```go
func weatherToolHandler(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    location, ok := args["location"].(string)
    if !ok {
        return nil, fmt.Errorf("location parameter required")
    }

    // Fetch weather data
    weather := fetchWeather(location)

    return map[string]interface{}{
        "temperature": weather.Temp,
        "conditions":  weather.Conditions,
        "humidity":    weather.Humidity,
    }, nil
}

// Register the tool
tm.RegisterTool(
    "get_weather",
    "Get current weather for a location",
    weatherParams,
    weatherToolHandler,
)
```

## Message Handling

### Sending Messages
```go
// Create a message
msg := aistudio.Message{
    Sender:  "user",
    Content: "Hello, AI!",
}

// Add to model (in Update method)
model.AddMessage(msg)
```

### Processing Responses
```go
// In your Update method
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case aistudio.StreamResponseMsg:
        // Handle AI response
        m.AddMessage(aistudio.Message{
            Sender:  "model",
            Content: msg.Text,
        })
    }
    return m, nil
}
```

## Session Management

### Working with History
```go
// Get history manager
hm := model.GetHistoryManager()

// Create new session
session := hm.NewSession("Chat about Go", model.GetModelName())

// Add messages to session
hm.AddMessage(message)

// Save session
err := hm.SaveSession(session)

// Load session
loaded, err := hm.LoadSessionFromFile("path/to/session.json")

// List sessions
sessions, err := hm.ListSessions("user-id")
```

## Error Handling

### Retry Logic
```go
// Check if error is retryable
if err, ok := err.(*aistudio.RetryableError); ok {
    if err.ShouldRetry() {
        // Retry the operation
        time.Sleep(err.BackoffDuration())
        // Retry...
    }
}
```

### Error Types
```go
// Connection errors
type ConnectionError struct {
    Err error
}

// API errors
type APIError struct {
    Code    int
    Message string
}

// Tool execution errors
type ToolError struct {
    ToolName string
    Err      error
}
```

## Streaming

### Bidirectional Streaming
```go
// Initialize streaming
streamCmd := model.InitStreamCmd()

// Handle stream messages
switch msg := msg.(type) {
case aistudio.InitStreamMsg:
    // Stream initialized
case aistudio.StreamResponseMsg:
    // Handle response chunk
case aistudio.StreamClosedMsg:
    // Stream closed
case aistudio.StreamErrorMsg:
    // Handle error
}
```

## Audio Support

### Audio Configuration
```go
// Enable audio output
model := aistudio.New(
    aistudio.WithAudioOutput(true),
    aistudio.WithVoice("Puck"),
    aistudio.WithAudioPlayerCommand("ffplay -nodisp -autoexit -"),
)
```

### Audio Messages
```go
// Check if message has audio
if msg.HasAudio {
    // Play audio
    cmd := model.PlayAudioCmd(msg.AudioData)
}
```

## Testing

### Mocking the Model
```go
type MockModel struct {
    *aistudio.Model
    // Override methods as needed
}

func (m *MockModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // Custom update logic for testing
    return m, nil
}
```

### Testing Tools
```go
func TestCustomTool(t *testing.T) {
    model := aistudio.New(aistudio.WithToolsEnabled(true))
    tm := model.GetToolManager()

    // Register test tool
    err := tm.RegisterTool("test", "Test tool", params, handler)
    assert.NoError(t, err)

    // Execute tool
    result, err := handler(context.Background(), args)
    assert.NoError(t, err)
    assert.NotNil(t, result)
}
```

## Best Practices

### 1. Error Handling
Always check for errors and handle them appropriately:
```go
if err != nil {
    // Log error
    log.Printf("Error: %v", err)

    // Update UI state
    model.SetError(err)

    // Return error command if needed
    return model, aistudio.ShowErrorCmd(err)
}
```

### 2. Context Management
Use context for cancellation and timeouts:
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// Use ctx in operations
result, err := operation(ctx)
```

### 3. Resource Cleanup
Always clean up resources:
```go
defer func() {
    if model != nil {
        model.Close()
    }
}()
```

### 4. Tool Safety
Validate tool inputs:
```go
func safeToolHandler(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    // Validate required parameters
    if _, ok := args["required_param"]; !ok {
        return nil, fmt.Errorf("missing required parameter")
    }

    // Sanitize inputs
    input := sanitize(args["input"].(string))

    // Execute with timeout
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    return executeToolSafely(ctx, input)
}
```

## Examples

### Complete Application
```go
package main

import (
    "log"
    "os"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/tmc/aistudio"
)

func main() {
    // Create model with configuration
    model := aistudio.New(
        aistudio.WithModel("gemini-2.0-flash-latest"),
        aistudio.WithAPIKey(os.Getenv("GEMINI_API_KEY")),
        aistudio.WithToolsEnabled(true),
        aistudio.WithHistoryEnabled(true),
        aistudio.WithTemperature(0.7),
    )

    // Create Bubble Tea program
    p := tea.NewProgram(model, tea.WithAltScreen())

    // Run the program
    if _, err := p.Run(); err != nil {
        log.Fatal(err)
    }
}
```

### Custom Tool Integration
```go
// Define tool parameters
params, _ := structpb.NewStruct(map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "query": map[string]interface{}{
            "type": "string",
            "description": "Search query",
        },
    },
    "required": []string{"query"},
})

// Create handler
searchHandler := func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    query := args["query"].(string)
    results := performSearch(query)
    return results, nil
}

// Register tool
model.GetToolManager().RegisterTool(
    "search",
    "Search the web",
    params,
    searchHandler,
)
```

## API Reference

For complete API reference, run:
```bash
go doc github.com/tmc/aistudio
```

Or view online at:
https://pkg.go.dev/github.com/tmc/aistudio

---

*Version: 1.0.0*
*Last Updated: September 2025*