package aistudio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
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
	stream generativelanguagepb.GenerativeService_StreamGenerateContentClient
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
		log.Println("[DEBUG] initStreamCmd: Starting connection attempt")
		// Reuse the root context if it exists, otherwise create a new one
		if m.rootCtx == nil {
			m.rootCtx, m.rootCtxCancel = context.WithCancel(context.Background())
		}

		// Always create a fresh stream context for each connection attempt
		// This ensures we don't reuse potentially problematic contexts
		if m.streamCtxCancel != nil {
			m.streamCtxCancel()
			m.streamCtx = nil
			m.streamCtxCancel = nil
			// Give some time for the context to fully cancel and clean up
			time.Sleep(100 * time.Millisecond)
		}

		// Create a new context with timeout to prevent hanging connections
		connectionTimeout := 60 * time.Second // Longer timeout for bidirectional streaming
		if timeoutStr := os.Getenv(EnvConnectionTimeout); timeoutStr != "" {
			if parsedTimeout, err := strconv.Atoi(timeoutStr); err == nil {
				connectionTimeout = time.Duration(parsedTimeout) * time.Second
				log.Printf("[DEBUG] Using custom connection timeout: %v", connectionTimeout)
			}
		}

		m.streamCtx, m.streamCtxCancel = context.WithTimeout(m.rootCtx, connectionTimeout)

		// Initialize the client configuration
		clientConfig := api.StreamClientConfig{
			ModelName:    m.modelName,
			EnableAudio:  m.enableAudio,
			VoiceName:    m.voiceName,
			SystemPrompt: m.systemPrompt,
			// Add generation parameters
			Temperature:     m.temperature,
			TopP:            m.topP,
			TopK:            m.topK,
			MaxOutputTokens: m.maxOutputTokens,
			// Feature flags
			EnableWebSocket: m.enableWebSocket,
		}

		if m.enableTools && m.toolManager != nil {
			// Log tools being sent to the API
			toolCount := len(m.toolManager.RegisteredToolDefs)
			if toolCount > 0 {
				log.Printf("Sending %d registered tools to API", toolCount)
			}

			// Convert registered tools to the format expected by the client config
			var apiToolDefs []*api.ToolDefinition
			for name := range m.toolManager.RegisteredTools {
				registeredTool := m.toolManager.RegisteredTools[name]
				if registeredTool.IsAvailable {
					log.Printf("Adding tool definition for API: %s", name)
					apiToolDefs = append(apiToolDefs, &registeredTool.ToolDefinition)
				}
			}
			clientConfig.ToolDefinitions = m.toolManager.RegisteredToolDefs[:]
		}

		// Add feature flags
		clientConfig.EnableWebSearch = m.enableWebSearch // Grounding
		clientConfig.EnableCodeExecution = m.enableCodeExecution
		clientConfig.DisplayTokenCounts = m.displayTokenCounts
		clientConfig.ResponseMimeType = m.responseMimeType
		clientConfig.ResponseSchemaFile = m.responseSchemaFile

		// Initialize the stream
		start := time.Now()
		log.Printf("[DEBUG] Initializing StreamGenerateContent for model: %s with API key (%d chars)",
			m.modelName, len(m.apiKey))

		// Log connection debugging info if enabled
		if os.Getenv(EnvDebugConnection) == "true" {
			log.Printf("[DEBUG] Backend: %s, Project: %s, Location: %s", m.backend.String(), m.projectID, m.location)
			log.Printf("[DEBUG] WebSocket enabled: %v, Audio enabled: %v", m.enableWebSocket, m.enableAudio)
		}

		// Ensure client is initialized
		if m.client == nil {
			m.client = &api.Client{}
		}

		// Set API authentication and service selection
		m.client.APIKey = m.apiKey
		if m.backend == BackendVertexAI {
			m.client.Backend = api.BackendVertexAI
			m.client.ProjectID = m.projectID
			m.client.Location = m.location
		} else if m.backend == BackendGrok {
			m.client.Backend = api.BackendGrok
		} else {
			m.client.Backend = api.BackendGeminiAPI
		}

		// Initialize client with timeout monitoring
		log.Printf("[DEBUG] About to call m.client.InitClient for model: %s", m.modelName)
		initStart := time.Now()
		err := m.client.InitClient(m.streamCtx)
		if err != nil {
			elapsed := time.Since(initStart)
			log.Printf("[ERROR] Client initialization failed after %v: %v", elapsed, err)
			// Check if it was a timeout
			if m.streamCtx.Err() == context.DeadlineExceeded {
				log.Printf("[ERROR] Client initialization timed out - this indicates network connectivity issues")
			}
			return initErrorMsg{err: fmt.Errorf("client init failed: %w", err)}
		}
		initElapsed := time.Since(initStart)
		log.Printf("[DEBUG] Client initialized successfully in %v", initElapsed)

		// Initialize client stream with timeout monitoring
		streamStart := time.Now()

		// Choose the appropriate streaming method based on model capabilities
		if m.useBidi {
			log.Printf("[DEBUG] Using bidirectional streaming for model: %s", m.modelName)
			stream, err := m.client.InitBidiStream(m.streamCtx, &clientConfig)
			if err != nil {
				streamElapsed := time.Since(streamStart)
				log.Printf("[ERROR] Bidirectional stream initialization failed after %v: %v", streamElapsed, err)
				// Check if it was a timeout
				if m.streamCtx.Err() == context.DeadlineExceeded {
					log.Printf("[ERROR] Stream initialization timed out - this indicates gRPC/WebSocket connectivity issues")
				}
				return initErrorMsg{err: fmt.Errorf("bidi stream init failed: %w", err)}
			}
			m.bidiStream = stream // Store the bidirectional connection
			m.stream = nil        // Clear regular stream
		} else {
			log.Printf("[DEBUG] Using regular streaming for model: %s", m.modelName)
			stream, err := m.client.InitStreamGenerateContent(m.streamCtx, &clientConfig)
			if err != nil {
				streamElapsed := time.Since(streamStart)
				log.Printf("[ERROR] Regular stream initialization failed after %v: %v", streamElapsed, err)
				// Check if it was a timeout
				if m.streamCtx.Err() == context.DeadlineExceeded {
					log.Printf("[ERROR] Stream initialization timed out - this indicates gRPC connectivity issues")
				}
				return initErrorMsg{err: fmt.Errorf("stream init failed: %w", err)}
			}
			m.stream = stream     // Store the regular connection
			m.bidiStream = nil    // Clear bidirectional stream
		}

		streamElapsed := time.Since(streamStart)
		totalElapsed := time.Since(start)
		log.Printf("[DEBUG] Stream initialized successfully in %v (total: %v)", streamElapsed, totalElapsed)

		// Start connection health monitoring if debug is enabled
		if os.Getenv(EnvDebugConnection) == "true" {
			go m.monitorConnection()
		}

		// Start receiving from the stream
		log.Printf("[DEBUG] About to send initClientCompleteMsg - bidiStream: %v, stream: %v", m.bidiStream != nil, m.stream != nil)
		return initClientCompleteMsg{}
	}
}

// receiveStreamCmd returns a command that receives messages from a stream.
func (m *Model) receiveStreamCmd() tea.Cmd {
	return func() tea.Msg {
		// Double-check we have a valid stream before attempting to receive
		if m.stream == nil {
			log.Println("receiveStreamCmd: Stream is nil")
			return streamClosedMsg{}
		}

		// If the stream context has been canceled, don't attempt to receive
		if m.streamCtx == nil || m.streamCtx.Err() != nil {
			log.Printf("Stream context canceled or nil before receiving, aborting receive")
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

// receiveBidiStreamCmd returns a command that receives messages from a stream.
func (m *Model) receiveBidiStreamCmd() tea.Cmd {
	return func() tea.Msg {
		log.Printf("[DEBUG] receiveBidiStreamCmd: Starting receive attempt")

		// Double-check we have a valid stream and context before attempting to receive
		if m.bidiStream == nil {
			log.Println("[ERROR] receiveBidiStreamCmd: Bidi stream is nil")
			return streamClosedMsg{}
		}

		// If the stream context has been canceled, don't attempt to receive
		if m.streamCtx == nil || m.streamCtx.Err() != nil {
			log.Printf("[ERROR] Stream context canceled or nil before receiving, aborting receive")
			return streamClosedMsg{}
		}

		log.Printf("[DEBUG] receiveBidiStreamCmd: About to call bidiStream.Recv() on stream %p", m.bidiStream)
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
			return streamErrorMsg{err: fmt.Errorf("stream receive failed: %w", err)}
		}

		output := api.ExtractOutput(resp)

		// Check if there's a function call in the output that needs to be processed
		if output.FunctionCall != nil {
			log.Printf("Detected function call in stream response: %s", output.FunctionCall.Name)
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
		// Handle Grok API differently since it uses HTTP streaming
		if m.backend == BackendGrok {
			return m.sendToGrokStreamCmd(text)()
		}

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

// sendToGrokStreamCmd returns a command that sends a message to Grok API stream.
func (m *Model) sendToGrokStreamCmd(text string) tea.Cmd {
	return func() tea.Msg {
		log.Printf("Sending message to Grok: %s", text)

		// Stop any currently playing audio
		m.StopCurrentAudio()

		// Add user message to history
		userMsg := Message{
			Sender:    senderNameUser,
			Content:   text,
			Timestamp: time.Now(),
		}
		m.messages = append(m.messages, userMsg)

		// Create Grok request with conversation history
		grokMessages := []api.GrokMessage{}

		// Add system prompt if available
		if m.systemPrompt != "" {
			grokMessages = append(grokMessages, api.GrokMessage{
				Role:    "system",
				Content: m.systemPrompt,
			})
		}

		// Convert message history to Grok format
		for _, msg := range m.messages {
			role := "user"
			if msg.Sender == senderNameModel {
				role = "assistant"
			} else if msg.Sender == senderNameSystem {
				role = "system"
			}

			grokMessages = append(grokMessages, api.GrokMessage{
				Role:    role,
				Content: msg.Content,
			})
		}

		// Create Grok request
		req := &api.GrokChatRequest{
			Model:    m.modelName,
			Messages: grokMessages,
			Stream:   true,
		}

		// Add generation parameters if set
		if m.temperature > 0 {
			req.Temperature = &m.temperature
		}
		if m.maxOutputTokens > 0 {
			req.MaxTokens = &m.maxOutputTokens
		}

		// Create context for request
		ctx := m.streamCtx
		if ctx == nil {
			ctx = context.Background()
		}

		// Start streaming
		chunkChan, errChan := m.client.GrokChatStream(ctx, req)

		// Start collecting response
		var responseText strings.Builder
		responseMsg := Message{
			Sender:    senderNameModel,
			Content:   "",
			Timestamp: time.Now(),
		}

		// Process stream chunks
		go func() {
			for {
				select {
				case chunk, ok := <-chunkChan:
					if !ok {
						// Stream ended, finalize message
						if responseText.Len() > 0 {
							responseMsg.Content = responseText.String()
							m.messages = append(m.messages, responseMsg)
						}
						return
					}

					// Process chunk
					if len(chunk.Choices) > 0 && chunk.Choices[0].Delta != nil {
						deltaText := chunk.Choices[0].Delta.Content
						responseText.WriteString(deltaText)

						// Send incremental update
						output := api.StreamOutput{
							Text:         deltaText,
							TurnComplete: false,
						}
						m.uiUpdateChan <- streamResponseMsg{output: output}
					}

				case err, ok := <-errChan:
					if !ok {
						return
					}
					if err != nil {
						log.Printf("Grok stream error: %v", err)
						m.uiUpdateChan <- streamErrorMsg{err: err}
						return
					}
				}
			}
		}()

		return sentMsg{}
	}
}

// sendToBidiStreamCmd returns a command that sends a message to a new stream.
// This function creates a new stream for each message, mimicking bidirectional capability.
func (m *Model) sendToBidiStreamCmd(text string) tea.Cmd {
	log.Printf("sendToBidiStreamCmd: Creating new stream with message: %s", text)
	return func() tea.Msg {
		// Stop any currently playing audio
		m.StopCurrentAudio()

		// Cancel previous context if it exists
		if m.streamCtxCancel != nil {
			log.Println("Canceling previous stream context before sending new message")
			m.streamCtxCancel()
		}

		// Create a new context with cancellation, using the root context as parent
		if m.rootCtx == nil {
			log.Println("Warning: Root context is nil, creating new background context")
			m.rootCtx, m.rootCtxCancel = context.WithCancel(context.Background())
		}
		m.streamCtx, m.streamCtxCancel = context.WithCancel(m.rootCtx)

		// Initialize the client configuration
		clientConfig := api.StreamClientConfig{
			ModelName:    m.modelName,
			EnableAudio:  m.enableAudio,
			VoiceName:    m.voiceName,
			SystemPrompt: m.systemPrompt,
			// Add generation parameters
			Temperature:     m.temperature,
			TopP:            m.topP,
			TopK:            m.topK,
			MaxOutputTokens: m.maxOutputTokens,
			// Feature flags
			EnableWebSocket: m.enableWebSocket,
		}

		if m.enableTools && m.toolManager != nil {
			var apiToolDefs []*api.ToolDefinition
			for name := range m.toolManager.RegisteredTools {
				registeredTool := m.toolManager.RegisteredTools[name]
				if registeredTool.IsAvailable {
					log.Printf("Adding tool definition for API: %s", name)
					apiToolDefs = append(apiToolDefs, &registeredTool.ToolDefinition)
				}
			}
			clientConfig.ToolDefinitions = m.toolManager.RegisteredToolDefs[:]
		}

		// Create a request with the user message
		request := &generativelanguagepb.GenerateContentRequest{
			Model: m.modelName,
			Contents: []*generativelanguagepb.Content{
				{
					Parts: []*generativelanguagepb.Part{
						{
							Data: &generativelanguagepb.Part_Text{
								Text: text,
							},
						},
					},
					Role: "user",
				},
			},
		}

		// Set up GenerationConfig
		genConfig := &generativelanguagepb.GenerationConfig{}
		genConfig.Temperature = &m.temperature
		if m.topP > 0 {
			genConfig.TopP = &m.topP
		}
		if m.topK > 0 {
			genConfig.TopK = &m.topK
		}
		if m.maxOutputTokens > 0 {
			genConfig.MaxOutputTokens = &m.maxOutputTokens
		}

		request.GenerationConfig = genConfig

		log.Printf("Sending new StreamGenerateContent request: %s", text)
		stream, err := m.client.GenerativeClient.StreamGenerateContent(m.streamCtx, request)
		if err != nil {
			log.Printf("Stream Init Error: %v", err)
			return sendErrorMsg{err: fmt.Errorf("stream creation failed: %w", err)}
		}

		// Update the stream in the model
		m.bidiStream = stream
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

// closeBidiStreamCmd returns a command that closes a stream.
func (m *Model) closeBidiStreamCmd() tea.Cmd {
	return func() tea.Msg {
		log.Println("Closing bidirectional stream and canceling context")

		// Properly close the stream before setting it to nil
		if m.bidiStream != nil {
			if err := m.bidiStream.CloseSend(); err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "transport is closing") {
				log.Printf("Error during CloseSend for bidi stream: %v", err)
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
