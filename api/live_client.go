package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// LiveModelEndpoint is the WebSocket endpoint for Gemini Live API
	LiveModelEndpoint = "wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1alpha.GenerativeService.BidiGenerateContent"
)

// LiveClient manages communication with Gemini live models through WebSockets
type LiveClient struct {
	// WebSocket connection
	conn        *websocket.Conn
	connMutex   sync.Mutex
	initialized bool
	closed      bool

	// Context for managing the connection
	ctx    context.Context
	cancel context.CancelFunc

	// Authentication and model config
	apiKey string
	model  string

	// Stream configuration
	temperature     float32
	topP            float32
	topK            int32
	maxOutputTokens int32
	systemPrompt    string
	enableAudio     bool
	voiceName       string

	// Recording and replay support
	recorder    *WSRecorder
	replayMode  bool
	replayIndex int
}

// LiveSetupRequest represents the initial setup message for the Live API
type LiveSetupRequest struct {
	Setup LiveSetupConfig `json:"setup"`
}

// LiveSetupConfig contains configuration for the Live API session
type LiveSetupConfig struct {
	Model             string               `json:"model"`
	GenerationConfig  LiveGenerationConfig `json:"generationConfig,omitempty"`
	SystemInstruction *LiveContent         `json:"systemInstruction,omitempty"`
	Tools             []LiveTool           `json:"tools,omitempty"`
}

// LiveGenerationConfig contains generation parameters for the Live API
type LiveGenerationConfig struct {
	Temperature        *float32          `json:"temperature,omitempty"`
	TopP               *float32          `json:"topP,omitempty"`
	TopK               *int32            `json:"topK,omitempty"`
	MaxOutputTokens    *int32            `json:"maxOutputTokens,omitempty"`
	ResponseModalities []string          `json:"responseModalities,omitempty"`
	SpeechConfig       *LiveSpeechConfig `json:"speechConfig,omitempty"`
}

// LiveSpeechConfig configures speech output for audio responses
type LiveSpeechConfig struct {
	VoiceConfig *LiveVoiceConfig `json:"voiceConfig,omitempty"`
}

// LiveVoiceConfig configures the voice for audio responses
type LiveVoiceConfig struct {
	PrebuiltVoiceConfig *LivePrebuiltVoiceConfig `json:"prebuiltVoiceConfig,omitempty"`
}

// LivePrebuiltVoiceConfig specifies a prebuilt voice
type LivePrebuiltVoiceConfig struct {
	VoiceName string `json:"voiceName"`
}

// LiveContent represents content in a message
type LiveContent struct {
	Role  string     `json:"role,omitempty"`
	Parts []LivePart `json:"parts"`
}

// LivePart represents a part of a message
type LivePart struct {
	Text string `json:"text,omitempty"`
}

// LiveTool represents a tool definition for the Live API
type LiveTool struct {
	FunctionDeclarations []LiveFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

// LiveFunctionDeclaration represents a function declaration for the Live API
type LiveFunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// LiveClientMessageRequest represents a client message to the Live API
type LiveClientMessageRequest struct {
	ClientContent *LiveClientContent `json:"clientContent,omitempty"`
}

// LiveClientContent contains the content of a client message
type LiveClientContent struct {
	Turns        []LiveContent `json:"turns"`
	TurnComplete bool          `json:"turnComplete"`
}

// LiveServerResponse represents a response from the Live API
type LiveServerResponse struct {
	ServerContent *LiveServerContent `json:"serverContent,omitempty"`
	SetupComplete *struct{}          `json:"setupComplete,omitempty"`
	UsageMetadata *LiveUsageMetadata `json:"usageMetadata,omitempty"`
}

// LiveServerContent contains the content of a server response
type LiveServerContent struct {
	ModelTurn          *LiveContent `json:"modelTurn,omitempty"`
	TurnComplete       bool         `json:"turnComplete"`
	GenerationComplete bool         `json:"generationComplete"`
	Interrupted        bool         `json:"interrupted"`
}

// LiveUsageMetadata contains usage information
type LiveUsageMetadata struct {
	PromptTokenCount   int32 `json:"promptTokenCount"`
	ResponseTokenCount int32 `json:"responseTokenCount"`
	TotalTokenCount    int32 `json:"totalTokenCount"`
}

// NewLiveClient creates a new client for the Gemini Live API
func NewLiveClient(ctx context.Context, apiKey string, config *StreamClientConfig, recorder *WSRecorder) (*LiveClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required for Live API")
	}

	if config.ModelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	// Create a context with cancel function
	ctxWithCancel, cancel := context.WithCancel(ctx)

	// Ensure model name has "models/" prefix - WebSocket API v1alpha requires it
	modelName := config.ModelName
	if !strings.HasPrefix(modelName, "models/") {
		modelName = "models/" + modelName
	}

	// Create the client
	client := &LiveClient{
		apiKey:          apiKey,
		model:           modelName,
		ctx:             ctxWithCancel,
		cancel:          cancel,
		temperature:     config.Temperature,
		topP:            config.TopP,
		topK:            config.TopK,
		maxOutputTokens: config.MaxOutputTokens,
		systemPrompt:    config.SystemPrompt,
		enableAudio:     config.EnableAudio,
		voiceName:       config.VoiceName,
		recorder:        recorder,
	}

	// If recorder is provided and in replay mode, set replay mode
	if recorder != nil && !recorder.RecordMode {
		client.replayMode = true
	}

	return client, nil
}

// Initialize connects to the WebSocket endpoint and performs initial setup
func (c *LiveClient) Initialize() error {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	if c.initialized {
		return nil
	}

	if c.closed {
		return fmt.Errorf("client has been closed")
	}

	// Skip real connection if in replay mode
	if c.replayMode && c.recorder != nil {
		c.initialized = true
		log.Printf("Using replay mode for Live API model: %s", c.model)
		return nil
	}

	// Connect to the WebSocket endpoint
	log.Printf("Connecting to Live API endpoint: %s", LiveModelEndpoint)
	header := http.Header{}
	header.Add("x-goog-api-key", c.apiKey)

	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
	}

	conn, resp, err := dialer.DialContext(c.ctx, LiveModelEndpoint, header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to connect to Live API: %v (HTTP status: %d)", err, resp.StatusCode)
		}
		return fmt.Errorf("failed to connect to Live API: %v", err)
	}
	c.conn = conn

	// Send setup message
	if err := c.sendSetupMessage(); err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to send setup message: %v", err)
	}

	// Wait for setup complete message
	if err := c.waitForSetupComplete(); err != nil {
		c.conn.Close()
		return fmt.Errorf("setup failed: %v", err)
	}

	c.initialized = true
	log.Printf("Live API session established successfully for model: %s", c.model)
	return nil
}

// sendSetupMessage sends the initial setup message to the server
func (c *LiveClient) sendSetupMessage() error {
	// Skip if in replay mode
	if c.replayMode && c.recorder != nil {
		// We'll get the setup complete from the recording
		return nil
	}

	// Create generation config
	genConfig := LiveGenerationConfig{}

	// Add parameters if provided
	if c.temperature > 0 {
		genConfig.Temperature = &c.temperature
	}

	if c.topP > 0 {
		genConfig.TopP = &c.topP
	}

	if c.topK > 0 {
		genConfig.TopK = &c.topK
	}

	if c.maxOutputTokens > 0 {
		genConfig.MaxOutputTokens = &c.maxOutputTokens
	}

	// Set response modalities
	genConfig.ResponseModalities = []string{"TEXT"}

	// Add audio config if enabled
	if c.enableAudio && c.voiceName != "" {
		genConfig.ResponseModalities = []string{"AUDIO"}
		genConfig.SpeechConfig = &LiveSpeechConfig{
			VoiceConfig: &LiveVoiceConfig{
				PrebuiltVoiceConfig: &LivePrebuiltVoiceConfig{
					VoiceName: c.voiceName,
				},
			},
		}
	}

	// Create setup config
	setupConfig := LiveSetupConfig{
		Model:            c.model,
		GenerationConfig: genConfig,
	}

	// Add system instruction if provided
	if c.systemPrompt != "" {
		setupConfig.SystemInstruction = &LiveContent{
			Parts: []LivePart{
				{
					Text: c.systemPrompt,
				},
			},
		}
	}

	// Create setup request
	setupReq := LiveSetupRequest{
		Setup: setupConfig,
	}

	// Marshal to JSON
	setupJSON, err := json.Marshal(setupReq)
	if err != nil {
		return fmt.Errorf("failed to marshal setup message: %v", err)
	}

	log.Printf("Sending setup message: %s", string(setupJSON))

	// Record the message if recording is enabled
	if c.recorder != nil && c.recorder.RecordMode {
		if err := c.recorder.RecordSend(setupJSON, websocket.TextMessage); err != nil {
			log.Printf("Warning: Failed to record setup message: %v", err)
		}
	}

	return c.conn.WriteMessage(websocket.TextMessage, setupJSON)
}

// waitForSetupComplete waits for the setup complete message from the server
func (c *LiveClient) waitForSetupComplete() error {
	// Skip in replay mode - we'll get responses from the recording
	if c.replayMode && c.recorder != nil {
		return nil
	}

	// Create a context with a timeout for setup
	setupCtx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	// Set up a channel for the result
	setupCompleteCh := make(chan struct{}, 1)
	errorCh := make(chan error, 1)

	// Set up a goroutine to wait for setup complete message
	go func() {
		defer close(setupCompleteCh)
		defer close(errorCh)

		// Read messages until setup complete is received or error occurs
		for {
			select {
			case <-setupCtx.Done():
				errorCh <- fmt.Errorf("setup timed out")
				return
			default:
				// Continue reading
			}

			messageType, message, err := c.conn.ReadMessage()
			if err != nil {
				errorCh <- fmt.Errorf("failed to read from WebSocket: %v", err)
				return
			}

			log.Printf("Received message: %s", string(message))

			// Record the message if recording is enabled
			if c.recorder != nil && c.recorder.RecordMode {
				if err := c.recorder.RecordReceive(message, messageType); err != nil {
					log.Printf("Warning: Failed to record received message: %v", err)
				}
			}

			// Parse the message
			var response LiveServerResponse
			if err := json.Unmarshal(message, &response); err != nil {
				errorCh <- fmt.Errorf("failed to parse server message: %v", err)
				return
			}

			// Check if it's a setupComplete message
			if response.SetupComplete != nil {
				setupCompleteCh <- struct{}{}
				return
			}
		}
	}()

	// Wait for result or timeout
	select {
	case <-setupCompleteCh:
		return nil
	case err := <-errorCh:
		return err
	case <-setupCtx.Done():
		return fmt.Errorf("setup timed out waiting for server response")
	}
}

// SendMessage sends a message to the server
func (c *LiveClient) SendMessage(message string) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}

	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	// Create message request
	msgReq := LiveClientMessageRequest{
		ClientContent: &LiveClientContent{
			Turns: []LiveContent{
				{
					Role: "user",
					Parts: []LivePart{
						{
							Text: message,
						},
					},
				},
			},
			TurnComplete: true,
		},
	}

	// Marshal to JSON
	msgJSON, err := json.Marshal(msgReq)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	log.Printf("Sending message: %s", string(msgJSON))

	// Record the message if recording is enabled
	if c.recorder != nil && c.recorder.RecordMode {
		if err := c.recorder.RecordSend(msgJSON, websocket.TextMessage); err != nil {
			log.Printf("Warning: Failed to record message: %v", err)
		}
	}

	// In replay mode, don't actually send
	if c.replayMode && c.recorder != nil {
		return nil
	}

	return c.conn.WriteMessage(websocket.TextMessage, msgJSON)
}

// ReceiveMessage receives a message from the server
func (c *LiveClient) ReceiveMessage() (*StreamOutput, error) {
	if err := c.ensureInitialized(); err != nil {
		return nil, err
	}

	var message []byte
	var messageType int
	var err error

	// In replay mode, get message from recorder
	if c.replayMode && c.recorder != nil {
		direction, msg, msgType, err := c.recorder.GetNextMessage()
		if err != nil {
			return nil, fmt.Errorf("failed to get message from recording: %v", err)
		}

		// Skip send messages
		if direction == "send" {
			return c.ReceiveMessage() // Recursively call to get the next message
		}

		message = msg
		messageType = msgType
	} else {
		// Normal live API - read message from WebSocket
		messageType, message, err = c.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return nil, fmt.Errorf("connection closed")
			}
			return nil, fmt.Errorf("failed to read from WebSocket: %v", err)
		}

		// Record the message if recording is enabled
		if c.recorder != nil && c.recorder.RecordMode {
			if err := c.recorder.RecordReceive(message, messageType); err != nil {
				log.Printf("Warning: Failed to record received message: %v", err)
			}
		}
	}

	// Parse the message
	var response LiveServerResponse
	if err := json.Unmarshal(message, &response); err != nil {
		return nil, fmt.Errorf("failed to parse server message: %v", err)
	}

	// Convert to StreamOutput
	output := &StreamOutput{}

	if response.ServerContent != nil {
		if response.ServerContent.ModelTurn != nil && len(response.ServerContent.ModelTurn.Parts) > 0 {
			// Get text from parts
			for _, part := range response.ServerContent.ModelTurn.Parts {
				output.Text += part.Text
			}
		}

		output.TurnComplete = response.ServerContent.TurnComplete
	}

	// Add usage information if available
	if response.UsageMetadata != nil {
		output.PromptTokenCount = response.UsageMetadata.PromptTokenCount
		output.CandidateTokenCount = response.UsageMetadata.ResponseTokenCount
		output.TotalTokenCount = response.UsageMetadata.TotalTokenCount
	}

	return output, nil
}

// Close closes the WebSocket connection
func (c *LiveClient) Close() error {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	c.cancel()

	// Close recorder if present
	if c.recorder != nil {
		if err := c.recorder.Close(); err != nil {
			log.Printf("Warning: Error closing recorder: %v", err)
		}
	}

	// In replay mode, don't need to close any real connection
	if c.replayMode {
		return nil
	}

	if c.conn != nil {
		// Send close message
		err := c.conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		if err != nil {
			log.Printf("Error sending close message: %v", err)
		}

		// Close the connection
		return c.conn.Close()
	}

	return nil
}

// ensureInitialized ensures the client is initialized
func (c *LiveClient) ensureInitialized() error {
	if c.closed {
		return fmt.Errorf("client has been closed")
	}

	if !c.initialized {
		return c.Initialize()
	}

	return nil
}

// IsLiveModel checks if a model name corresponds to a live model
func IsLiveModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	return strings.Contains(modelName, "live") &&
		(strings.Contains(modelName, "gemini-2.0") ||
			strings.Contains(modelName, "gemini-2.5"))
}
