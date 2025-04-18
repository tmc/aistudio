# Tool Calling in AIStudio

AIStudio supports Gemini's tool calling capabilities, allowing the model to invoke external functions and tools during a conversation.

## Enabling Tool Calling

Tool calling is enabled by default. You can explicitly control it with the `--tools` flag:

```bash
# Enable tool calling (default)
aistudio --tools

# Disable tool calling
aistudio --tools=false
```

## Defining Custom Tools

You can define custom tools in a JSON file and load them with the `--tools-file` option:

```bash
aistudio --tools-file=my_tools.json
```

### Tool Definition Format

The tool definition file should contain an array of function declarations in the format expected by the Gemini API. Each function declaration should include:

- `name`: The name of the function
- `description`: A description of what the function does
- `parameters`: The parameters the function accepts, defined in JSON Schema format

Example tool definition file:

```json
[
  {
    "name": "get_weather",
    "description": "Get the current weather for a location",
    "parameters": {
      "type": "object",
      "properties": {
        "location": {
          "type": "string",
          "description": "The city and state, e.g., 'San Francisco, CA'"
        },
        "unit": {
          "type": "string",
          "enum": ["celsius", "fahrenheit"],
          "description": "The unit of temperature"
        }
      },
      "required": ["location"]
    }
  }
]
```

## Viewing Available Tools

During a conversation, you can press `Ctrl+T` to see a list of available tools.

## Implementation Details

Tool calling in AIStudio works as follows:

1. When the model generates a tool call, AIStudio intercepts it and extracts the function name and arguments.
2. If a matching tool is registered, AIStudio executes it with the provided arguments.
3. The results are sent back to the model, which can then incorporate them into its response.

## Built-in Tools

AIStudio comes with several built-in tools that can be enabled in your tool definition file:

- `search_web`: Search the web for information
- `get_current_time`: Get the current time
- `calculate`: Perform mathematical calculations

To use these built-in tools, include them in your tool definition file with the appropriate parameters.

## Developing Custom Tool Handlers

For advanced users who want to develop custom tool handlers, you can extend the AIStudio codebase to add your own tool implementations. This requires modifying the source code and rebuilding the application.

1. Define your tool handler function in the codebase
2. Register it with the tool manager
3. Include the tool definition in your tool definition file

See the existing tool implementations in the source code for examples.
