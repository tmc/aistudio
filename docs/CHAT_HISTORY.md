# Chat History in AIStudio

AIStudio includes a chat history management system that allows you to save and load conversation sessions.

## Enabling Chat History

Chat history is enabled by default. You can explicitly control it with the `--history` flag:

```bash
# Enable chat history (default)
aistudio --history

# Disable chat history
aistudio --history=false
```

## History Storage Location

By default, chat history is stored in the `./history` directory. You can specify a different location with the `--history-dir` flag:

```bash
aistudio --history-dir=/path/to/history
```

## Saving Chat History

Chat history is automatically saved in the following situations:

1. When you send a new message
2. When you receive a response from the model
3. When you exit the application

You can also manually save the current session by pressing `Ctrl+H`.

## Session Format

Each chat session is stored as a JSON file in the history directory. The filename includes a timestamp and a generated title based on the conversation content.

## Privacy Considerations

Chat history is stored locally on your machine. No conversation data is sent to external servers beyond what's necessary for the Gemini API interaction.

If you're concerned about privacy, you can:

1. Disable chat history with `--history=false`
2. Periodically clear your history directory
3. Use a history directory on an encrypted volume

## Implementation Details

The chat history system in AIStudio:

1. Creates a new session when the application starts
2. Adds messages to the session as they are sent and received
3. Periodically saves the session to disk
4. Saves the session when the application exits

## Future Enhancements

Planned enhancements for the chat history feature include:

- Ability to browse and load previous sessions
- Search functionality for finding specific conversations
- Export/import capabilities for sharing conversations
- Session tagging and organization
