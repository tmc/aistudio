package aistudio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"cloud.google.com/go/ai/generativelanguage/apiv1alpha/generativelanguagepb"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tmc/aistudio/api"
)

// --- Stream Messages ---
type streamResponseMsg struct {
	output api.StreamOutput
}

type streamErrorMsg struct {
	err error
}

type streamClosedMsg struct{}

type sentMsg struct{}

type sendErrorMsg struct {
	err error
}

type initStreamMsg struct{}
type initClientCompleteMsg struct{}
type initErrorMsg struct {
	err error
}

type streamReadyMsg struct {
	stream generativelanguagepb.GenerativeService_StreamGenerateContentClient
}

type bidiStreamReadyMsg struct {
	stream generativelanguagepb.GenerativeService_BidiGenerateContentClient
}

type bidiStreamResponseMsg struct {
	output api.StreamOutput
}

// formatExecutableCodeMessage creates a Message from an ExecutableCode response
func formatExecutableCodeMessage(execCode *generativelanguagepb.ExecutableCode) Message {
	return Message{
		Sender:           "Model",
		Content:          fmt.Sprintf("Executable %s code:", execCode.GetLanguage()),
		IsExecutableCode: true,
		ExecutableCode: &ExecutableCode{
			Language: fmt.Sprint(execCode.GetLanguage()),
			Code:     execCode.GetCode(),
		},
		Timestamp: time.Now(),
	}
}

// formatExecutableCodeResultMessage creates a Message from an ExecutableCodeResult response
func formatExecutableCodeResultMessage(execResult *generativelanguagepb.CodeExecutionResult) Message {
	return Message{
		Sender:                 "System",
		Content:                "Code Execution Result:",
		IsExecutableCodeResult: true,
		ExecutableCodeResult:   execResult,
		Timestamp:              time.Now(),
	}
}

// --- Stream Commands ---

// initStreamCmd returns a command that initializes a stream.
func (m *Model) initStreamCmd() tea.Cmd {
	return func() tea.Msg {
		log.Println("initStreamCmd")
		// Reuse the root context if it exists, otherwise create a new one
		if m.rootCtx == nil {
			log.Println("Warning: Root context is nil, creating new background context")
			m.rootCtx, m.rootCtxCancel = context.WithCancel(context.Background())
		}

		// Only cancel existing stream context if it's a retry attempt
		if m.streamCtxCancel != nil && m.streamRetryAttempt > 0 {
			log.Println("Canceling previous stream context")
			m.streamCtxCancel()
			m.streamCtx = nil
			m.streamCtxCancel = nil
			// Small delay to ensure context is properly canceled
			time.Sleep(100 * time.Millisecond)
		}

		// Create a new context with cancellation from the root context
		if m.streamCtx == nil {
			m.streamCtx, m.streamCtxCancel = context.WithCancel(m.rootCtx)
		}

		clientConfig := api.ClientConfig{
			ModelName:    m.modelName,
			EnableAudio:  m.enableAudio,  // Pass audio preference
			VoiceName:    m.voiceName,    // Pass voice name
			SystemPrompt: m.systemPrompt, // Pass system prompt
		}

		if m.enableTools && m.toolManager != nil {
			// Log tools being sent to the API
			toolCount := len(m.toolManager.RegisteredToolDefs)
			if toolCount > 0 {
				log.Printf("Sending %d registered tools to API", toolCount)
			}

			// Convert registered tools to the format expected by the client config
			var apiToolDefs []*api.ToolDefinition
			for name, registeredTool := range m.toolManager.RegisteredTools {
				if registeredTool.IsAvailable {
					log.Printf("Adding tool definition for API: %s", name)
					// Make a copy to ensure we have a stable pointer
					defCopy := registeredTool.ToolDefinition
					apiToolDefs = append(apiToolDefs, &defCopy)
				}
			}
			clientConfig.ToolDefinitions = m.toolManager.RegisteredToolDefs[:]
		}

		if err := m.client.InitClient(m.streamCtx); err != nil {
			log.Printf("Client Init Error: %v", err)
			return streamErrorMsg{err: fmt.Errorf("client init failed: %w", err)}
		}

		if m.useBidi {
			// Use BidiGenerateContent for true bidirectional streaming
			bidiStream, err := m.client.InitBidiStream(m.streamCtx, clientConfig)
			if err != nil {
				log.Printf("Bidi Stream Init Error: %v", err)
				return streamErrorMsg{err: fmt.Errorf("bidi connection failed: %w", err)}
			}
			log.Println("Bidirectional stream established successfully")
			return bidiStreamReadyMsg{stream: bidiStream}
		} else {
			// Use StreamGenerateContent for one-way streaming
			stream, err := m.client.InitStreamGenerateContent(m.streamCtx, clientConfig)
			if err != nil {
				log.Printf("Stream Init Error: %v", err)
				return streamErrorMsg{err: fmt.Errorf("connection failed: %w", err)}
			}
			log.Println("One-way stream established successfully")
			return streamReadyMsg{stream: stream}
		}
	}
}

// receiveStreamCmd returns a command that receives messages from a stream.
func (m *Model) receiveStreamCmd() tea.Cmd {
	return func() tea.Msg {
		if m.stream == nil {
			log.Println("receiveStreamCmd: Stream is nil")
			return streamClosedMsg{}
		}
		resp, err := m.stream.Recv()
		if err != nil {
			errStr := err.Error()
			if errors.Is(err, io.EOF) || strings.Contains(errStr, "transport is closing") ||
				strings.Contains(errStr, "EOF") || strings.Contains(errStr, "connection closed") {
				log.Println(err)
				log.Println("receiveStreamCmd: Received stream closed signal.")
				return streamClosedMsg{}
			}
			log.Printf("Stream Recv Error: %v", err)
			return streamErrorMsg{err: fmt.Errorf("receive failed: %w", err)}
		}
		output := api.ExtractOutput(resp)
		return streamResponseMsg{output: output}
	}
}

// receiveBidiStreamCmd returns a command that receives messages from a bidirectional stream.
func (m *Model) receiveBidiStreamCmd() tea.Cmd {
	return func() tea.Msg {
		// Double-check we have a valid stream and context before attempting to receive
		if m.bidiStream == nil {
			log.Println("receiveBidiStreamCmd: Bidi stream is nil")
			return streamClosedMsg{}
		}

		// If the stream context has been canceled, don't attempt to receive
		if m.streamCtx == nil || m.streamCtx.Err() != nil {
			log.Printf("Stream context canceled or nil before receiving, aborting receive")
			return streamClosedMsg{}
		}

		resp, err := m.bidiStream.Recv()
		if err != nil {
			errStr := err.Error()
			if errors.Is(err, io.EOF) || strings.Contains(errStr, "transport is closing") ||
				strings.Contains(errStr, "EOF") || strings.Contains(errStr, "connection closed") ||
				strings.Contains(errStr, "context canceled") {
				log.Println(err)

				// Don't return streamClosedMsg during initialization as it could cause early exit
				if m.currentState == AppStateInitializing {
					log.Println("receiveBidiStreamCmd: Ignoring stream closed during initialization.")
					return initClientCompleteMsg{}
				}

				log.Println("receiveBidiStreamCmd: Received stream closed signal.")
				return streamClosedMsg{}
			}
			log.Printf("Bidi Stream Recv Error: %v", err)
			return streamErrorMsg{err: fmt.Errorf("bidirectional receive failed: %w", err)}
		}

		output := api.ExtractBidiOutput(resp)

		// Check if there's a function call in the output that needs to be processed
		if output.FunctionCall != nil {
			log.Printf("Detected function call in bidi response: %s", output.FunctionCall.Name)
		}

		// Log message received for debugging
		if output.Text != "" {
			log.Printf("Stream received message with text (%d chars): %q", len(output.Text), output.Text)
		}

		return bidiStreamResponseMsg{output: output}
	}
}

// sendToStreamCmd returns a command that sends a message to a stream.
func (m *Model) sendToStreamCmd(text string) tea.Cmd {
	return func() tea.Msg {
		// Since we're using StreamGenerateContent, we can't send data after the stream is created
		// Instead, we'll need to close the current stream and create a new one with the user's message

		// Stop any currently playing audio
		m.StopCurrentAudio()

		// Properly close the current stream if it exists
		if m.stream != nil {
			err := m.stream.CloseSend()
			if err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "transport is closing") {
				log.Printf("Error during CloseSend: %v", err)
			}
		}

		// Cancel previous context
		if m.streamCtxCancel != nil {
			log.Println("Canceling previous stream context before sending new message")
			m.streamCtxCancel()
		}

		// Create a new context with cancellation, using the root context as parent if it exists
		// Ensure root context exists first
		if m.rootCtx == nil {
			log.Println("Warning: Root context is nil, creating new background context")
			m.rootCtx, m.rootCtxCancel = context.WithCancel(context.Background())
		}
		m.streamCtx, m.streamCtxCancel = context.WithCancel(m.rootCtx)

		log.Printf("Sending new message to %s via StreamGenerateContent: %s", m.modelName, text)

		// Create content parts
		contents := []*generativelanguagepb.Content{}

		// Add system prompt if defined
		if m.systemPrompt != "" {
			contents = append(contents, &generativelanguagepb.Content{
				Parts: []*generativelanguagepb.Part{
					{
						Data: &generativelanguagepb.Part_Text{
							Text: m.systemPrompt,
						},
					},
				},
				Role: "system",
			})
		}

		// Add user message
		contents = append(contents, &generativelanguagepb.Content{
			Parts: []*generativelanguagepb.Part{
				{
					Data: &generativelanguagepb.Part_Text{
						Text: text,
					},
				},
			},
			Role: "user",
		})

		// Create the request
		request := &generativelanguagepb.GenerateContentRequest{
			Model:    m.modelName,
			Contents: contents,
		}

		log.Println("genclient:")
		log.Println(m.client.GenerativeClient)
		// Start a new stream with the request
		stream, err := m.client.GenerativeClient.StreamGenerateContent(m.streamCtx, request)
		if err != nil {
			log.Printf("Stream Init Error: %v", err)
			return sendErrorMsg{err: fmt.Errorf("stream creation failed: %w", err)}
		}

		// Update the stream in the model
		m.stream = stream
		return sentMsg{}
	}
}

// sendToBidiStreamCmd returns a command that sends a message to a bidirectional stream.
func (m *Model) sendToBidiStreamCmd(text string) tea.Cmd {
	log.Printf("sendToBidiStreamCmd: Sending message to Bidi stream: %s", text)
	return func() tea.Msg {
		if m.bidiStream == nil {
			log.Println("sendToBidiStreamCmd: Bidi stream is nil, cannot send message")
			return sendErrorMsg{err: errors.New("bidirectional stream not initialized")}
		}

		// Stop any currently playing audio
		m.StopCurrentAudio()

		// Send to existing bidirectional stream
		log.Printf("Sending message to bidirectional stream: %s", text)
		if err := m.client.SendMessageToBidiStream(m.bidiStream, text); err != nil {
			log.Printf("Bidi stream send error: %v", err)
			return sendErrorMsg{err: fmt.Errorf("bidirectional stream send failed: %w", err)}
		}

		return sentMsg{}
	}
}

// closeStreamCmd returns a command that closes a stream.
func (m *Model) closeStreamCmd() tea.Cmd {
	return func() tea.Msg {
		log.Println("Closing stream and canceling context")
		if m.stream != nil {
			err := m.stream.CloseSend()
			if err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "transport is closing") {
				log.Printf("Error during CloseSend: %v", err)
			}
			m.stream = nil
		}

		// Cancel the context
		if m.streamCtxCancel != nil {
			m.streamCtxCancel()
			m.streamCtxCancel = nil
		}

		return streamClosedMsg{}
	}
}

// closeBidiStreamCmd returns a command that closes a bidirectional stream.
func (m *Model) closeBidiStreamCmd() tea.Cmd {
	return func() tea.Msg {
		log.Println("Closing bidirectional stream and canceling context")
		if m.bidiStream != nil {
			err := m.bidiStream.CloseSend()
			if err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "transport is closing") {
				log.Printf("Error during Bidi CloseSend: %v", err)
			}
			m.bidiStream = nil
		}

		// Cancel the context
		if m.streamCtxCancel != nil {
			m.streamCtxCancel()
			m.streamCtxCancel = nil
		}

		return streamClosedMsg{}
	}
}

// waitForFocusCmd waits briefly before potentially blocking operations.
func waitForFocusCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg { return nil })
}
