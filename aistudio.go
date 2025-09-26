package aistudio

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"time"

	"cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tmc/aistudio/api"
	"github.com/tmc/aistudio/audioplayer"
	"github.com/tmc/aistudio/internal/helpers"
	"github.com/tmc/aistudio/settings"
	"golang.org/x/term"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	initialBackoffDuration = 1 * time.Second
	maxBackoffDuration     = 120 * time.Second      // Increased max backoff
	backoffFactor          = 1.5                    // Slightly reduced to allow more attempts before hitting max
	jitterFactor           = 0.1                    // Reduced to +/- 10% for more predictable backoffs
	maxStreamRetries       = 10                     // Doubled max retries
	connectionResetDelay   = 100 * time.Millisecond // Short delay for connection reset
	keepaliveInterval      = 5 * time.Minute        // Send keepalive every 5 minutes
	connectionTimeout      = 60 * time.Second       // Timeout for initial connection
)

// New creates a new Model instance with default settings and applies options.
// Returns nil if initialization fails.
func New(opts ...Option) *Model {
	// Error handling for required dependencies
	if len(opts) == 0 {
		log.Println("Error: No options provided to New()")
		fmt.Fprintf(os.Stderr, "Error: No configuration options provided\n")
		return nil
	}

	ta := textarea.New()
	ta.Placeholder = "Type something and press Enter..."
	ta.Focus()
	ta.Prompt = "┃ "
	ta.CharLimit = 0
	ta.SetWidth(50) // Will be updated by WindowSizeMsg
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)

	// Create a viewport that fills most of the screen
	width, height, _ := term.GetSize(0)
	if width < 20 {
		width = 80 // Default if we can't get terminal size
	}
	if height < 10 {
		height = 24 // Default if we can't get terminal size
	}

	viewportHeight := height - 10 // Reserve space for input, status, etc.
	if viewportHeight < 5 {
		viewportHeight = 5 // Minimum viewport height
	}

	vp := viewport.New(width, viewportHeight)
	vp.SetContent("Initializing...")
	vp.KeyMap.PageDown.SetEnabled(true)
	vp.KeyMap.PageUp.SetEnabled(true)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Create settings panel
	settingsPanel := settings.New()

	// Create audio player
	activeAudioPlayer := audioplayer.New()

	m := &Model{
		textarea:               ta,
		viewport:               vp,
		spinner:                s,
		messages:               []Message{},
		client:                 &api.Client{},
		modelName:              DefaultModel, // Use default model known to work
		voiceName:              DefaultVoice,
		showLogo:               true,                       // Default logo off
		logMessages:            []string{},                 // Initialize empty log messages
		maxLogMessages:         10,                         // Default to storing 10 log messages
		showLogMessages:        false,                      // Default log messages display off
		useBidi:                true,                       // Must use bidi (only streaming type supported)
		width:                  width,                      // Initialize width
		height:                 height,                     // Initialize height
		audioChannel:           make(chan AudioChunk, 100), // Buffer for up to 100 audio chunks
		audioQueue:             []AudioChunk{},
		showAudioStatus:        true,                    // Default to showing audio status
		audioPlaybackMode:      AudioPlaybackOnDiskFIFO, // Default to on-disk FIFO for smoother playback
		bufferMessageIdx:       -1,                      // No message index yet
		currentBufferWindow:    initialAudioBufferingWindow,
		recentChunkSizes:       make([]int, 0, 10),
		recentChunkTimes:       make([]time.Time, 0, 10),
		consecutiveSmallChunks: 0,
		uiUpdateChan:           make(chan tea.Msg, 10), // Initialize channel with a small buffer

		// New UI components
		settingsPanel:     &settingsPanel,
		activeAudioPlayer: &activeAudioPlayer,

		// Focus management
		focusedComponent:  "input", // Start with input focused
		showSettingsPanel: false,   // Settings panel hidden by default

		// History and tools default to disabled, enabled via options
		historyEnabled:    false,
		enableTools:       false,
		approvedToolTypes: make(map[string]bool), // Initialize empty map of approved tool types

		// Initialize retry state
		currentStreamBackoff: initialBackoffDuration,

		// Initial state
		currentState: AppStateInitializing,
	}

	for _, opt := range opts {
		if err := opt(m); err != nil {
			log.Printf("Warning: Error applying option: %v", err)
			fmt.Fprintf(os.Stderr, "Warning: Error applying option: %v\n", err)
			time.Sleep(1 * time.Second)
		}
	}

	if m.enableAudio && m.playerCmd == "" {
		m.playerCmd = detectAudioPlayer()
		if m.playerCmd == "" {
			log.Println("Warning: Could not auto-detect audio player. Audio output may fail.")
		}
	}

	// Initialize AfplayPlayer if audio is enabled and player is afplay
	if m.enableAudio && m.playerCmd == "afplay" {
		log.Println("Initializing AfplayPlayer during startup")
		m.afplayPlayer = audioplayer.NewAfplayPlayer(audioplayer.Config{
			SampleRate:    audioSampleRate,
			Channels:      1,
			BitsPerSample: 16,
			Format:        "s16le",
		})
		log.Printf("AfplayPlayer initialized: %v", m.afplayPlayer != nil)
	}

	return m
}

// InitModel initializes the Bubble Tea model details before starting the program.
func (m *Model) InitModel() (tea.Model, error) {
	if m.client == nil {
		m.client = &api.Client{}
	}
	m.client.APIKey = m.apiKey

	// Create root context with timeout if specified
	if m.rootCtx == nil {
		if m.globalTimeout > 0 {
			m.rootCtx, m.rootCtxCancel = context.WithTimeout(context.Background(), m.globalTimeout)
			log.Printf("Global timeout set to %s", m.globalTimeout)
		} else {
			m.rootCtx, m.rootCtxCancel = context.WithCancel(context.Background())
		}
	}

	// Set up log interceptor if log messages are enabled
	if m.showLogMessages {
		interceptor := &logInterceptor{
			model:    m,
			original: log.Writer(),
		}
		log.SetOutput(interceptor)
		log.Println("Log messages display enabled")
	}

	// Make sure textarea has focus from the beginning
	m.textarea.Focus()
	m.focusedComponent = "input"

	// Log configuration details
	log.Printf("Using model: %s", m.modelName)
	log.Printf("Bidirectional streaming enabled: %t", m.useBidi)
	log.Printf("Audio output enabled: %t", m.enableAudio)
	if m.globalTimeout > 0 {
		log.Printf("Global timeout: %s", m.globalTimeout)
	}
	if m.enableAudio {
		log.Printf("Audio voice: %s", m.voiceName)
		log.Printf("Audio player command: %q", m.playerCmd)
		log.Printf("Audio playback mode: %d", m.audioPlaybackMode)
	}
	log.Printf("Show logo: %t", m.showLogo)
	log.Printf("Show log messages: %t (max %d)", m.showLogMessages, m.maxLogMessages)
	log.Printf("Show audio status: %t", m.showAudioStatus)
	log.Printf("History enabled: %t", m.historyEnabled)
	log.Printf("Tool calling enabled: %t", m.enableTools)

	// Enhanced tools information
	if m.enableTools && m.toolManager != nil {
		toolCount := len(m.toolManager.RegisteredToolDefs)
		log.Printf("Available tools: %d", toolCount)
		log.Printf("Tools info: Press Ctrl+T to list all available tools")

		// List the names of available tools
		if toolCount > 0 {
			toolNames := []string{}
			for name, tool := range m.toolManager.RegisteredTools {
				if tool.IsAvailable {
					toolNames = append(toolNames, name)
				}
			}
			log.Printf("Tool names: %s", strings.Join(toolNames, ", "))
		}
	}

	return m, nil
}

// Update implements the BubbleTea model interface
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	log.Println("Update called with message:", msg)
	// Check for quitting state immediately
	if m.currentState == AppStateQuitting {
		log.Println("In quitting state, exiting application")
		return m, tea.Quit
	}

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	// Handle audio playback ticker for progress updates
	if m.tickerRunning && m.showAudioStatus && m.currentAudio != nil {
		cmds = append(cmds, playbackTickCmd())
	}

	// Special handling if waiting for tool approval (prioritized UI state)
	if m.showToolApproval && len(m.pendingToolCalls) > 0 {
		// Only process key messages for tool approval dialog, ignore other updates
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			return m.handleKeyMsg(keyMsg)
		}
	}

	// Handle different message types
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Process common keyboard shortcuts (quit, toggle settings, etc.)
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		// Handle window resizing
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Height = msg.Height - 8 // Reserve space for input and status
		m.viewport.Width = msg.Width
		m.textarea.SetWidth(msg.Width)

		// UI will be updated automatically via View() call
		return m, nil

	case tea.MouseMsg:
		// Handle mouse events if needed
		return m, nil

	case playbackTickMsg:
		// Update audio playback status
		// Note: UI will be updated automatically via View() call
		return m, playbackTickCmd() // Continue ticker

	case audioPlaybackStartedMsg:
		// Handle audio playback starting
		m.tickerRunning = true
		if idx := msg.chunk.MessageIndex; idx >= 0 && idx < len(m.messages) {
			m.messages[idx].IsPlaying = true
		}
		m.currentAudio = &msg.chunk
		return m, playbackTickCmd() // Start ticker for progress updates

	case audioPlaybackCompletedMsg:
		// Handle audio playback ending
		m.tickerRunning = false
		if idx := msg.chunk.MessageIndex; idx >= 0 && idx < len(m.messages) {
			m.messages[idx].IsPlaying = false
			m.messages[idx].IsPlayed = true
		}
		return m, nil

	case audioPlaybackErrorMsg:
		// Handle audio playback errors
		m.tickerRunning = false
		m.messages = append(m.messages, formatError(fmt.Errorf("audio error: %v", msg.err)))
		return m, nil

	case func() tea.Msg:
		// Handle deferred function messages (from uiUpdateChan)
		return m, tea.Batch(msg) // Execute the function

	case toolCallSentMsg:
		// Tool call sent successfully
		return m, m.receiveBidiStreamCmd() // Continue receiving from stream

	case autoSendMsg:
		// Auto-send test message
		testMessage := fmt.Sprintf("Test message sent automatically after %v delay for debugging connection stability.", m.autoSendDelay)
		log.Printf("[DEBUG] Auto-sending test message: %s", testMessage)

		// Set the message in the textarea and trigger send
		m.textarea.SetValue(testMessage)
		m.currentState = AppStateResponding

		// Clear the textarea after sending
		m.textarea.SetValue("")

		// Send the message
		return m, m.sendToStreamCmd(testMessage)

	// Multimodal streaming messages
	case MultimodalStreamingStartedMsg:
		// Handle multimodal streaming started
		m.messages = append(m.messages, Message{
			Sender:  "System",
			Content: "🎤📷 Multimodal streaming started - audio input and screen capture active",
		})
		return m, nil

	case MultimodalStreamingStoppedMsg:
		// Handle multimodal streaming stopped
		stopMsg := fmt.Sprintf("🛑 Multimodal streaming stopped - Duration: %v, Audio chunks: %d, Images: %d, Data: %d bytes",
			msg.Duration, msg.AudioChunksSent, msg.ImageFramesSent, msg.BytesStreamed)
		m.messages = append(m.messages, Message{
			Sender:  "System",
			Content: stopMsg,
		})
		return m, nil

	case MultimodalResponseMsg:
		// Handle multimodal response from live API
		switch msg.Type {
		case "text":
			if msg.Content != "" {
				m.messages = append(m.messages, Message{
					Sender:  "Gemini",
					Content: msg.Content,
				})
			}
		case "audio":
			if msg.AudioData != nil && len(msg.AudioData) > 0 {
				// Play the audio response
				cmds = append(cmds, m.playAudioCmd(msg.AudioData))
			}
		case "function_call":
			if msg.FunctionCall != nil {
				// Handle function call
				m.messages = append(m.messages, Message{
					Sender:  "Gemini",
					Content: fmt.Sprintf("🔧 Function call: %s", msg.FunctionCall.Name),
				})
			}
		}
		return m, tea.Batch(cmds...)

	case AudioInputStartedMsg:
		// Handle audio input started
		m.messages = append(m.messages, Message{
			Sender:  "System",
			Content: "🎤 Audio input started - listening for voice commands",
		})
		return m, nil

	case AudioInputStoppedMsg:
		// Handle audio input stopped
		m.messages = append(m.messages, Message{
			Sender:  "System",
			Content: "🔇 Audio input stopped",
		})
		return m, nil

	case AudioInputChunkMsg:
		// Handle audio input chunk - this is processed by the multimodal manager
		// Just log for debugging
		log.Printf("[AUDIO_INPUT] Received audio chunk: %d bytes, voice=%v", 
			len(msg.Chunk.Data), msg.Chunk.IsVoice)
		return m, nil

	case AudioInputErrorMsg:
		// Handle audio input error
		m.messages = append(m.messages, Message{
			Sender:  "System",
			Content: fmt.Sprintf("❌ Audio input error: %v", msg.Error),
		})
		return m, nil

	case ImageCaptureStartedMsg:
		// Handle image capture started
		m.messages = append(m.messages, Message{
			Sender:  "System",
			Content: "📷 Screen capture started - capturing screen at regular intervals",
		})
		return m, nil

	case ImageCaptureStoppedMsg:
		// Handle image capture stopped
		m.messages = append(m.messages, Message{
			Sender:  "System",
			Content: "📷 Screen capture stopped",
		})
		return m, nil

	case ImageFrameMsg:
		// Handle image frame captured - processed by multimodal manager
		log.Printf("[IMAGE_CAPTURE] Received image frame: %d bytes, format=%s", 
			len(msg.Frame.Data), msg.Frame.Format)
		return m, nil

	case ImageCaptureErrorMsg:
		// Handle image capture error
		m.messages = append(m.messages, Message{
			Sender:  "System",
			Content: fmt.Sprintf("❌ Image capture error: %v", msg.Error),
		})
		return m, nil

	default:
		// Handle stream-related messages
		return m.handleStreamMsg(msg)
	}

	// Textarea updates are handled in handleKeyMsg to avoid double processing

	// Update viewport
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	// Handle settings panel update if showing
	if m.showSettingsPanel && m.settingsPanel != nil {
		*m.settingsPanel, cmd = m.settingsPanel.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Listen for more UI updates
	cmds = append(cmds, m.listenForUIUpdatesCmd())

	return m, tea.Batch(cmds...)
}

// Init implements the BubbleTea model interface
func (m *Model) Init() tea.Cmd {
	log.Println("Init called - starting the application")

	// Create a batch of commands to initialize the application
	var cmds []tea.Cmd

	// --- Initialize model details (previously in InitModel) ---
	if m.client == nil {
		m.client = &api.Client{}
	}
	m.client.APIKey = m.apiKey

	// Create root context with timeout if specified
	if m.rootCtx == nil {
		if m.globalTimeout > 0 {
			m.rootCtx, m.rootCtxCancel = context.WithTimeout(context.Background(), m.globalTimeout)
			log.Printf("Global timeout set to %s", m.globalTimeout)
		} else {
			m.rootCtx, m.rootCtxCancel = context.WithCancel(context.Background())
		}
	}

	// Set up log interceptor if log messages are enabled
	if m.showLogMessages {
		interceptor := &logInterceptor{
			model:    m,
			original: log.Writer(),
		}
		log.SetOutput(interceptor)
		log.Println("Log messages display enabled")
	}

	// Make sure textarea has focus from the beginning
	m.textarea.Focus()
	m.focusedComponent = "input"

	// Log configuration details
	log.Printf("Using model: %s", m.modelName)
	log.Printf("Bidirectional streaming enabled: %t", m.useBidi)
	log.Printf("Audio output enabled: %t", m.enableAudio)
	if m.globalTimeout > 0 {
		log.Printf("Global timeout: %s", m.globalTimeout)
	}
	if m.enableAudio {
		log.Printf("Audio voice: %s", m.voiceName)
		log.Printf("Audio player command: %q", m.playerCmd)
		log.Printf("Audio playback mode: %d", m.audioPlaybackMode)
	}
	log.Printf("Show logo: %t", m.showLogo)
	log.Printf("Show log messages: %t (max %d)", m.showLogMessages, m.maxLogMessages)
	log.Printf("Show audio status: %t", m.showAudioStatus)
	log.Printf("History enabled: %t", m.historyEnabled)
	log.Printf("Tool calling enabled: %t", m.enableTools)

	// Enhanced tools information
	if m.enableTools && m.toolManager != nil {
		toolCount := len(m.toolManager.RegisteredToolDefs)
		log.Printf("Available tools: %d", toolCount)
		log.Printf("Tool approval required: %t", m.requireApproval)
		log.Printf("Tools info: Press Ctrl+T to list all available tools")

		// List the names of available tools
		if toolCount > 0 {
			toolNames := []string{}
			for name, tool := range m.toolManager.RegisteredTools {
				if tool.IsAvailable {
					toolNames = append(toolNames, name)
				}
			}
			log.Printf("Tool names: %s", strings.Join(toolNames, ", "))
		}
	}

	// Initialize multimodal streaming if enabled
	if m.enableMultimodal {
		log.Printf("Multimodal streaming enabled: audio=%t, images=%t", 
			m.multimodalConfig.EnableAudio, m.multimodalConfig.EnableImages)
		
		// Initialize multimodal manager
		m.multimodalManager = NewMultimodalStreamingManager(m.multimodalConfig, m.client, m.uiUpdateChan)
		
		if m.multimodalConfig.EnableAudio {
			log.Printf("Audio input configured: device=%s, sample_rate=%d, channels=%d", 
				m.multimodalConfig.AudioConfig.InputDevice, 
				m.multimodalConfig.AudioConfig.SampleRate,
				m.multimodalConfig.AudioConfig.Channels)
		}
		
		if m.multimodalConfig.EnableImages {
			log.Printf("Image capture configured: interval=%v, format=%s, quality=%d", 
				m.multimodalConfig.ImageConfig.CaptureInterval,
				m.multimodalConfig.ImageConfig.OutputFormat,
				m.multimodalConfig.ImageConfig.CaptureQuality)
		}
	}

	// --- Continue with the original Init behavior ---
	// Start with ready state
	//m.currentState = AppStateReady

	cmds = append(cmds, m.setupInitialUICmd())
	cmds = append(cmds, m.initStreamCmd())

	return tea.Batch(cmds...)
}

// UpdateHistory adds a message to history and saves if history is enabled
func (m *Model) UpdateHistory(message Message) {
	if !m.historyEnabled || m.historyManager == nil {
		return
	}

	m.historyManager.AddMessage(message)

	// Queue a save but don't wait for it
	go func() {
		if m.historyManager != nil && m.historyManager.CurrentSession != nil {
			log.Printf("Auto-saving session %s after adding message from %s",
				m.historyManager.CurrentSession.ID, message.Sender)

			// Create a tea.Msg for the UI to handle asynchronously
			if m.uiUpdateChan != nil {
				m.uiUpdateChan <- func() tea.Msg {
					return m.saveSessionCmd()()
				}
			}
		}
	}()
}

// ExitCode returns the exit code that should be used when the program terminates
func (m *Model) ExitCode() int {
	return m.exitCode
}

// ToolManager returns the model's tool manager
func (m *Model) ToolManager() *ToolManager {
	return m.toolManager
}

// ProcessStdinMode processes messages from stdin without running the TUI
// This is useful for scripting or non-interactive usage
func (m *Model) ProcessStdinMode(ctx context.Context) error {
	if m.client == nil {
		m.client = &api.Client{}
	}
	m.client.APIKey = m.apiKey

	// Create root context with timeout if specified or use the provided context
	if ctx == nil {
		if m.globalTimeout > 0 {
			m.rootCtx, m.rootCtxCancel = context.WithTimeout(context.Background(), m.globalTimeout)
			log.Printf("Global timeout set to %s", m.globalTimeout)
		} else {
			m.rootCtx, m.rootCtxCancel = context.WithCancel(context.Background())
		}
	} else {
		m.rootCtx = ctx
	}

	// Initialize client
	if err := m.client.InitClient(m.rootCtx); err != nil {
		return fmt.Errorf("failed to initialize client: %w", err)
	}

	// Init tools if enabled
	if m.enableTools && m.toolManager == nil {
		m.toolManager = NewToolManager()
		// Register default tools
		if err := m.toolManager.RegisterDefaultTools(); err != nil {
			log.Printf("Warning: Failed to register default tools: %v", err)
		}
	}

	// Initialize the bidi stream
	config := api.StreamClientConfig{
		ModelName:    m.modelName,
		EnableAudio:  false, // Disable audio in stdin mode
		SystemPrompt: m.systemPrompt,
		// Add other config options as needed
		Temperature:     m.temperature,
		TopP:            m.topP,
		TopK:            m.topK,
		MaxOutputTokens: m.maxOutputTokens,
		// Feature flags
		EnableWebSocket: m.enableWebSocket,
	}

	// Add tool definitions if enabled
	if m.enableTools && m.toolManager != nil {
		config.ToolDefinitions = m.toolManager.GetAvailableTools()
	}

	// Initialize bidirectional stream
	var err error
	m.bidiStream, err = m.client.InitBidiStream(m.rootCtx, &config)
	if err != nil {
		return fmt.Errorf("failed to initialize bidirectional stream: %w", err)
	}
	defer m.bidiStream.CloseSend()

	// Create scanner to read from stdin
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Fprintln(os.Stderr, "aistudio stdin mode - Type messages and press Enter. Use Ctrl+D to exit.")

	// Process messages from stdin
	for scanner.Scan() {
		message := scanner.Text()
		if message == "" {
			continue
		}

		// Create a user message
		userMsg := formatMessage("You", message)
		m.messages = append(m.messages, userMsg)

		// Add to history if enabled
		if m.historyEnabled && m.historyManager != nil {
			m.historyManager.AddMessage(userMsg)
		}

		// Send message to stream
		if err := m.client.SendMessageToBidiStream(m.bidiStream, message); err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}

		// Receive response
		var responseText strings.Builder

		for {
			resp, err := m.bidiStream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("stream error: %w", err)
			}

			// Extract text from response
			output := api.ExtractBidiOutput(resp)
			responseText.WriteString(output.Text)

			// Process tool calls if enabled
			if m.enableTools && output.FunctionCall != nil {
				// Extract and process tool calls
				toolCalls := ExtractToolCalls(&output)
				if len(toolCalls) > 0 {
					// Use processToolCalls which handles approval logic
					results, err := m.processToolCalls(toolCalls)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error processing tool calls: %v\n", err)
					} else if len(results) > 0 {
						// Convert ToolResult slice to []*generativelanguagepb.FunctionResponse
						var fnResults []*generativelanguagepb.FunctionResponse
						for i := range results {
							fnResults = append(fnResults, (*generativelanguagepb.FunctionResponse)(&results[i]))
						}

						// Send function responses back to model
						if err := m.client.SendToolResultsToBidiStream(m.bidiStream, fnResults...); err != nil {
							fmt.Fprintf(os.Stderr, "Error sending tool results: %v\n", err)
						}
					}
				}
			}
			// Process grounding, safety, tokens
			m.ProcessGenerativeLanguageResponse(output)

			if output.TurnComplete {
				log.Println("Turn complete, exiting loop.")
				break
			}
		}

		// Create a model response message
		modelMsg := formatMessage("Gemini", responseText.String())
		m.messages = append(m.messages, modelMsg)

		// Add to history if enabled
		if m.historyEnabled && m.historyManager != nil {
			m.historyManager.AddMessage(modelMsg)

			// Save the history
			if err := m.historyManager.SaveSession(m.historyManager.CurrentSession); err != nil {
				log.Printf("Error saving history: %v", err)
			}
		}

		// Output response
		fmt.Println("rt:", responseText.String())
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin read error: %w", err)
	}

	return nil
}

// playbackTickCmd creates a command for the playback UI ticker.
func playbackTickCmd() tea.Cmd {
	// Send a tick message roughly 10 times per second
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return playbackTickMsg(t) // Defined in types.go
	})
}

// listenForUIUpdatesCmd returns a command that listens on the uiUpdateChan
// and forwards messages to the main Bubble Tea update loop.
func (m *Model) listenForUIUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		// Blocks until a message is available on the channel
		msg := <-m.uiUpdateChan
		return msg
	}
}

// setupInitialUICmd prepares the initial UI state
func (m *Model) setupInitialUICmd() tea.Cmd {
	// Ensure the textarea has focus from the very beginning
	// This is critical for the input prompt to be visible
	m.textarea.Focus()

	cmds := []tea.Cmd{
		m.spinner.Tick,
		m.listenForUIUpdatesCmd(), // Starts listening for background messages
	}

	// Start the audio processor goroutine if audio is enabled
	if m.enableAudio && m.audioChannel != nil {
		m.startAudioProcessor() // Defined in audio_player.go
	}

	// Start auto-send timer if enabled
	if m.autoSendEnabled {
		cmds = append(cmds, m.autoSendCmd())
	}

	return tea.Batch(cmds...)
}

// handleKeyMsg handles keyboard input messages.
func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var (
		cmds    []tea.Cmd
		sendCmd tea.Cmd
		// startPlaybackTicker bool // Flag is handled in the main Update loop
	)

	// Check if settings panel is focused first
	if m.showSettingsPanel && m.settingsPanel != nil && m.focusedComponent == "settings" {
		// Update the settings panel
		var settingsCmd tea.Cmd
		*m.settingsPanel, settingsCmd = m.settingsPanel.Update(msg)
		cmds = append(cmds, settingsCmd)

		// If settings panel no longer focused, return to input
		if !m.settingsPanel.Focused {
			m.focusedComponent = "input"
			m.textarea.Focus()
		}
		// Return early as settings panel handled the key
		return m, tea.Batch(cmds...)
	}
	// Update textarea if input is focused (for typing to work)
	if m.focusedComponent == "input" {
		var taCmd tea.Cmd
		m.textarea, taCmd = m.textarea.Update(msg)
		cmds = append(cmds, taCmd)
	}

	// Check if this is a regular printable character that should bypass special handling
	msgStr := msg.String()
	if len(msgStr) == 1 && msgStr[0] >= 32 && msgStr[0] <= 126 {
		// This is a regular printable character - just return with textarea update
		return m, tea.Batch(cmds...)
	}

	switch msgStr {
	// Add handlers for numbered dialog options for tool approval
	case "y", "Y", "1": // Approve tool call (Y key or option 1)
		if m.showToolApproval && len(m.pendingToolCalls) > 0 && m.approvalIndex < len(m.pendingToolCalls) {
			// Get the current tool call
			approvedCall := m.pendingToolCalls[m.approvalIndex]
			approvedCalls := []ToolCall{approvedCall} // Keep for executeToolCalls

			// Add formatted message for executing the tool call
			m.messages = append(m.messages, formatToolCallMessage(approvedCall, "Executing...")) // Use helper
			log.Printf("Tool call approved and executing: %s", approvedCall.Name)

			// Process the tool call
			results, err := m.executeToolCalls(approvedCalls)
			if err != nil {
				log.Printf("Error executing tool call: %v", err)
				// Add formatted message for the error result (optional, could use formatError)
				// m.messages = append(m.messages, helpers.formatToolResultMessage(approvedCall.ID, approvedCall.Name, json.RawMessage(fmt.Sprintf(`{"error": "%v"}`, err))))
				m.messages = append(m.messages, formatError(fmt.Errorf("error executing tool call '%s': %w", approvedCall.Name, err)))
			} else if len(results) > 0 {
				// Add formatted message for the successful result
				// Assuming one result for the one approved call
				if len(results) == 1 {
					m.messages = append(m.messages, formatToolResultMessage(results[0].Id, results[0].Name, results[0].Response, ToolCallStatusCompleted))
				} else {
					// Handle unexpected multiple results if necessary
					log.Printf("Warning: Expected 1 result for tool call %s, got %d", approvedCall.Name, len(results))
					for _, res := range results {
						m.messages = append(m.messages, formatToolResultMessage(res.Id, res.Name, res.Response, ToolCallStatusUnknown))
					}
				}
				// Send results back to model
				cmds = append(cmds, m.sendToolResultsCmd(results))
			} else {
				// Handle case where execution succeeded but returned no results (if applicable)
				log.Printf("Tool call %s executed successfully but returned no results.", approvedCall.Name)
				// Optionally add a message indicating no results were returned
				// m.messages = append(m.messages, helpers.formatToolResultMessage(approvedCall.ID, approvedCall.Name, json.RawMessage(`{"info": "No results returned"}`)))
			}

			// Move to next tool call or close modal
			m.approvalIndex++
			if m.approvalIndex >= len(m.pendingToolCalls) {
				// All tool calls processed, close the modal
				m.showToolApproval = false
				m.pendingToolCalls = nil
				m.approvalIndex = 0
			}

			// UI will update automatically
			return m, tea.Batch(cmds...)
		}
	case "2": // Approve tool call and don't ask again for this tool type
		if m.showToolApproval && len(m.pendingToolCalls) > 0 && m.approvalIndex < len(m.pendingToolCalls) {
			// Get the current tool call
			approvedCall := m.pendingToolCalls[m.approvalIndex]
			approvedCalls := []ToolCall{approvedCall} // Keep for executeToolCalls

			// Mark this tool type as pre-approved for future calls
			m.approvedToolTypes[approvedCall.Name] = true
			log.Printf("Tool type '%s' marked as pre-approved for future calls", approvedCall.Name)

			// Add formatted message for executing the tool call
			m.messages = append(m.messages, formatToolCallMessage(approvedCall, "Executing..."))
			m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Tool type '%s' will be auto-approved in the future", approvedCall.Name)))
			log.Printf("Tool call approved and executing: %s", approvedCall.Name)

			// Process the tool call
			results, err := m.executeToolCalls(approvedCalls)
			if err != nil {
				log.Printf("Error executing tool call: %v", err)
				m.messages = append(m.messages, formatError(fmt.Errorf("error executing tool call '%s': %w", approvedCall.Name, err)))
			} else if len(results) > 0 {
				// Add formatted message for the successful result
				if len(results) == 1 {
					m.messages = append(m.messages, formatToolResultMessage(results[0].Id, results[0].Name, results[0].Response, ToolCallStatusCompleted))
				} else {
					// Handle unexpected multiple results if necessary
					log.Printf("Warning: Expected 1 result for tool call %s, got %d", approvedCall.Name, len(results))
					for _, res := range results {
						m.messages = append(m.messages, formatToolResultMessage(res.Id, res.Name, res.Response, ToolCallStatusUnknown))
					}
				}
				// Send results back to model
				cmds = append(cmds, m.sendToolResultsCmd(results))
			}

			// Move to next tool call or close modal
			m.approvalIndex++
			if m.approvalIndex >= len(m.pendingToolCalls) {
				// All tool calls processed, close the modal
				m.showToolApproval = false
				m.pendingToolCalls = nil
				m.approvalIndex = 0
			}

			// UI will update automatically
			return m, tea.Batch(cmds...)
		}
	case "n", "N", "3", "esc": // Deny tool call (N key, option 3, or escape key)
		if m.showToolApproval && len(m.pendingToolCalls) > 0 && m.approvalIndex < len(m.pendingToolCalls) {
			// Get the current tool call
			deniedCall := m.pendingToolCalls[m.approvalIndex]

			// Log the denial
			log.Printf("Tool call denied: %s", deniedCall.Name)

			// Create a message about the denial
			m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Tool call to '%s' was denied by the user.", deniedCall.Name)))

			// Create an error function response
			var fnResponse generativelanguagepb.FunctionResponse
			fnResponse.Id = deniedCall.ID
			fnResponse.Response, _ = structpb.NewStruct(map[string]any{
				"error": "Tool call denied by user",
			})

			// Send the error result back to the model
			cmds = append(cmds, func() tea.Msg {
				if m.bidiStream == nil {
					return sendErrorMsg{err: fmt.Errorf("bidirectional stream not initialized")}
				}

				err := m.client.SendToolResultsToBidiStream(m.bidiStream, &fnResponse)
				if err != nil {
					return sendErrorMsg{err: fmt.Errorf("failed to send tool denial: %w", err)}
				}

				return toolCallSentMsg{}
			})

			// Move to next tool call or close modal
			m.approvalIndex++
			if m.approvalIndex >= len(m.pendingToolCalls) {
				// All tool calls processed, close the modal
				m.showToolApproval = false
				m.pendingToolCalls = nil
				m.approvalIndex = 0
			}

			// Update UI
			// UI will update automatically
			m.viewport.GotoBottom()
			// Update UI
			// UI will update automatically
			m.viewport.GotoBottom()
			// Transition state after processing
			// if m.approvalIndex >= len(m.pendingToolCalls) {
			// 	m.currentState = AppStateReady // Back to ready state if all done
			// } else {
			// 	log.Println("More tool calls pending, staying in waiting state.")
			// 	m.currentState = AppStateWaiting // Stay waiting if more tools
			// }
			return m, tea.Batch(cmds...)
		}
	case "ctrl+c":
		m.currentState = AppStateQuitting
		log.Println("Ctrl+C pressed, entering Quitting state.")

		// Immediately cancel contexts to stop hanging goroutines
		if m.rootCtxCancel != nil {
			log.Println("Ctrl+C: Canceling root context immediately")
			m.rootCtxCancel()
		}

		if m.streamCtxCancel != nil {
			log.Println("Ctrl+C: Canceling stream context immediately")
			m.streamCtxCancel()
		}

		// Shutdown streams
		if m.stream != nil {
			cmds = append(cmds, m.closeStreamCmd())
		}
		if m.bidiStream != nil {
			cmds = append(cmds, m.closeBidiStreamCmd())
		}

		// Experimental integrations disabled - moved to .wip files
		// if m.mcpEnabled && m.mcpIntegration != nil {
		// 	go func() {
		// 		if err := m.mcpIntegration.Shutdown(m.rootCtx); err != nil {
		// 			log.Printf("Error shutting down MCP integration: %v", err)
		// 		}
		// 	}()
		// }
		//
		// if m.voiceEnabled && m.voiceStreamer != nil {
		// 	go func() {
		// 		if err := m.voiceStreamer.Shutdown(m.rootCtx); err != nil {
		// 			log.Printf("Error shutting down voice streaming: %v", err)
		// 		}
		// 	}()
		// }
		//
		// if m.videoEnabled && m.videoStreamer != nil {
		// 	go func() {
		// 		if err := m.videoStreamer.Shutdown(m.rootCtx); err != nil {
		// 			log.Printf("Error shutting down video streaming: %v", err)
		// 		}
		// 	}()
		// }

		// Return quit command immediately for better UX
		cmds = append(cmds, tea.Quit)
		return m, tea.Batch(cmds...)

	case "ctrl+s": // Toggle settings panel
		m.showSettingsPanel = !m.showSettingsPanel
		if m.showSettingsPanel {
			m.focusedComponent = "settings"
			m.settingsPanel.Focus()
			m.textarea.Blur()
		} else {
			m.focusedComponent = "input"
			m.textarea.Focus()
		}
		return m, tea.Batch(cmds...) // Return early

	case "tab":
		// Handle tab navigation between tool calls in approval mode
		if m.showToolApproval && len(m.pendingToolCalls) > 1 && m.approvalIndex < len(m.pendingToolCalls)-1 {
			// Move to the next tool call
			m.approvalIndex++

			// UI will update automatically
			return m, tea.Batch(cmds...)
		}

		// Handle tab navigation between components
		if m.showSettingsPanel {
			if m.focusedComponent == "input" {
				m.focusedComponent = "settings"
				m.settingsPanel.Focus()
				m.textarea.Blur()
			} else if m.focusedComponent == "settings" {
				m.focusedComponent = "input"
				m.textarea.Focus()
				m.settingsPanel.Blur()
			}
		}
		return m, tea.Batch(cmds...) // Return early

	case "ctrl+m": // Toggle multimodal streaming
		if m.enableMultimodal && m.multimodalManager != nil {
			if m.multimodalManager.IsStreaming() {
				if err := m.multimodalManager.StopStreaming(); err != nil {
					m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Error stopping multimodal streaming: %v", err)))
				}
			} else {
				if err := m.multimodalManager.StartStreaming(); err != nil {
					m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Error starting multimodal streaming: %v", err)))
				}
			}
		} else {
			// Legacy mic simulation toggle
			m.micActive = !m.micActive
			log.Printf("Mic input simulation toggled: %v", m.micActive)
			if m.micActive {
				m.videoInputMode = VideoInputNone
			}
		}
		return m, tea.Batch(cmds...) // Return early

	case "ctrl+shift+a": // Toggle audio input only
		if m.enableMultimodal && m.multimodalManager != nil {
			audioManager := m.multimodalManager.GetAudioInputManager()
			if audioManager != nil {
				if err := audioManager.ToggleRecording(); err != nil {
					m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Error toggling audio input: %v", err)))
				}
			}
		}
		return m, tea.Batch(cmds...) // Return early

	case "ctrl+shift+i": // Toggle image capture only
		if m.enableMultimodal && m.multimodalManager != nil {
			imageManager := m.multimodalManager.GetImageCaptureManager()
			if imageManager != nil {
				if err := imageManager.ToggleScreenCapture(); err != nil {
					m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Error toggling screen capture: %v", err)))
				}
			}
		}
		return m, tea.Batch(cmds...) // Return early

	case "ctrl+shift+s": // Capture screen now
		if m.enableMultimodal && m.multimodalManager != nil {
			if err := m.multimodalManager.CaptureScreenNow(); err != nil {
				m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Error capturing screen: %v", err)))
			} else {
				m.messages = append(m.messages, formatMessage("System", "📷 Screen captured and sent to AI"))
			}
		}
		return m, tea.Batch(cmds...) // Return early

	case "ctrl+a": // Toggle tool approval
		if m.enableTools {
			// Toggle the approval requirement
			m.requireApproval = !m.requireApproval

			// Show a message about the change
			if m.requireApproval {
				m.messages = append(m.messages, formatMessage("System", "Tool approval is now ENABLED. All tool calls will require explicit approval."))
			} else {
				m.messages = append(m.messages, formatMessage("System", "Tool approval is now DISABLED. Tools will run automatically without approval."))

				// Clear any pre-approvals when disabling approval
				m.approvedToolTypes = make(map[string]bool)
			}

			// Update UI
			// UI will update automatically
			m.viewport.GotoBottom()
		}
		return m, tea.Batch(cmds...) // Return early

	case "ctrl+t": // Show available tools
		if m.enableTools && m.toolManager != nil {
			// Get list of available tools
			toolsList := m.toolManager.ListAvailableTools()

			// Display available tools
			m.messages = append(m.messages, formatMessage("System", toolsList))
			// UI will update automatically
			m.viewport.GotoBottom()
		} else {
			// Display a message about tools being disabled
			m.messages = append(m.messages, formatMessage("System", "Tool calling is disabled. Enable with --tools flag."))
			// UI will update automatically
			m.viewport.GotoBottom()
		}
		return m, tea.Batch(cmds...) // Return early

	case "ctrl+v": // Toggle Video state
		// Experimental video streaming disabled - moved to .wip files
		// if m.videoEnabled && m.videoStreamer != nil {
		// 	// Use actual video streaming
		// 	switch m.videoInputMode {
		// 	case VideoInputNone:
		// 		m.videoInputMode = VideoInputCamera
		// 		cmds = append(cmds, m.videoStreamer.SetVideoInputMode(VideoInputCamera))
		// 	case VideoInputCamera:
		// 		m.videoInputMode = VideoInputScreen
		// 		cmds = append(cmds, m.videoStreamer.SetVideoInputMode(VideoInputScreen))
		// 	case VideoInputScreen:
		// 		m.videoInputMode = VideoInputNone
		// 		cmds = append(cmds, m.videoStreamer.SetVideoInputMode(VideoInputNone))
		// 	}
		// } else {
		// Fallback to simulation mode
		switch m.videoInputMode {
		case VideoInputNone:
			m.videoInputMode = VideoInputCamera
		case VideoInputCamera:
			m.videoInputMode = VideoInputScreen
		case VideoInputScreen:
			m.videoInputMode = VideoInputNone
		}
		// }
		log.Printf("Video input toggled: %s", m.videoInputMode)
		if m.videoInputMode != VideoInputNone {
			m.micActive = false
		}
		return m, tea.Batch(cmds...) // Return early

	case "ctrl+p": // Play last available audio
		playCmd, triggered := m.PlayLastAudio() // Using extracted function from audio_controls.go
		if triggered {
			// startPlaybackTicker = true // Signal to start ticker if not running - handled in main Update
			cmds = append(cmds, playCmd)
		}
		// Return with potential play command and component updates
		return m, tea.Batch(cmds...)

	case "ctrl+r": // Replay last played audio
		replayCmd, triggered := m.ReplayLastAudio() // Using extracted function from audio_controls.go
		if triggered {
			// startPlaybackTicker = true // Signal to start ticker if not running - handled in main Update
			cmds = append(cmds, replayCmd)
		}
		return m, tea.Batch(cmds...)

	case "ctrl+h": // Toggle history view or save history
		if m.historyEnabled && m.historyManager != nil {
			// Save current session
			cmds = append(cmds, m.saveSessionCmd())

			// Display a message about history saving
			m.messages = append(m.messages, formatMessage("System", "Chat history saved."))
			// UI will update automatically
			m.viewport.GotoBottom()
		} else {
			// Display a message about history being disabled
			m.messages = append(m.messages, formatMessage("System", "Chat history is disabled. Enable with --history flag."))
			// UI will update automatically
			m.viewport.GotoBottom()
		}
		return m, tea.Batch(cmds...)

	case "ctrl+i": // Toggle voice input
		// Experimental voice streaming disabled - moved to .wip files
		// if m.voiceEnabled && m.voiceStreamer != nil {
		// 	if m.voiceStreamer.isStreaming {
		// 		if err := m.voiceStreamer.Stop(); err != nil {
		// 			log.Printf("Error stopping voice input: %v", err)
		// 		}
		// 		m.messages = append(m.messages, formatMessage("System", "Voice input stopped."))
		// 	} else {
		// 		if err := m.voiceStreamer.StartVoiceInput(); err != nil {
		// 			log.Printf("Error starting voice input: %v", err)
		// 			m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Failed to start voice input: %v", err)))
		// 		} else {
		// 			m.messages = append(m.messages, formatMessage("System", "Voice input started. Speak into your microphone."))
		// 		}
		// 	}
		// } else {
		m.messages = append(m.messages, formatMessage("System", "Voice input is disabled. Enable with --voice flag."))
		// }
		// UI will update automatically
		return m, tea.Batch(cmds...)

	case "ctrl+o": // Toggle voice output
		// Experimental voice streaming disabled - moved to .wip files
		// if m.voiceEnabled && m.voiceStreamer != nil {
		// 	if err := m.voiceStreamer.StartVoiceOutput(); err != nil {
		// 		log.Printf("Error starting voice output: %v", err)
		// 		m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Failed to start voice output: %v", err)))
		// 	} else {
		// 		m.messages = append(m.messages, formatMessage("System", "Voice output enabled for responses."))
		// 	}
		// } else {
		m.messages = append(m.messages, formatMessage("System", "Voice output is disabled. Enable with --voice flag."))
		// }
		// UI will update automatically
		return m, tea.Batch(cmds...)

	case "ctrl+b": // Toggle bidirectional voice
		// Experimental voice streaming disabled - moved to .wip files
		// if m.voiceEnabled && m.voiceStreamer != nil {
		// 	if m.voiceStreamer.isStreaming {
		// 		if err := m.voiceStreamer.Stop(); err != nil {
		// 			log.Printf("Error stopping voice streaming: %v", err)
		// 		}
		// 		m.messages = append(m.messages, formatMessage("System", "Bidirectional voice stopped."))
		// 	} else {
		// 		if err := m.voiceStreamer.StartBidirectional(); err != nil {
		// 			log.Printf("Error starting bidirectional voice: %v", err)
		// 			m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Failed to start bidirectional voice: %v", err)))
		// 		} else {
		// 			m.messages = append(m.messages, formatMessage("System", "Bidirectional voice started. You can speak and listen simultaneously."))
		// 		}
		// 	}
		// } else {
		m.messages = append(m.messages, formatMessage("System", "Voice streaming is disabled. Enable with --voice flag."))
		// }
		// UI will update automatically
		return m, tea.Batch(cmds...)

	case "ctrl+e": // Show MCP status
		// Experimental MCP integration disabled - moved to .wip files
		// if m.mcpEnabled && m.mcpIntegration != nil {
		// 	var statusText strings.Builder
		// 	statusText.WriteString("MCP Integration Status:\n")
		//
		// 	// Server status
		// 	if server := m.mcpIntegration.GetMCPServer(); server != nil {
		// 		statusText.WriteString("✓ MCP Server: Running\n")
		// 	} else {
		// 		statusText.WriteString("✗ MCP Server: Not running\n")
		// 	}
		//
		// 	// Client status
		// 	clients := m.mcpIntegration.GetMCPClients()
		// 	statusText.WriteString(fmt.Sprintf("✓ MCP Clients: %d connected\n", len(clients)))
		// 	for name, client := range clients {
		// 		if client.Connected {
		// 			statusText.WriteString(fmt.Sprintf("  • %s: Connected\n", name))
		// 		} else {
		// 			statusText.WriteString(fmt.Sprintf("  • %s: Disconnected\n", name))
		// 		}
		// 	}
		//
		// 	m.messages = append(m.messages, formatMessage("System", statusText.String()))
		// } else {
		m.messages = append(m.messages, formatMessage("System", "MCP integration is disabled. Enable with --mcp flag."))
		// }
		// UI will update automatically
		return m, tea.Batch(cmds...)

	case "enter": // Send message
		if txt := strings.TrimSpace(m.textarea.Value()); txt != "" {
			log.Printf("Sending message: %s", txt)
			m.messages = append(m.messages, formatMessage("You", txt)) // helpers.go

			m.textarea.Reset()
			m.textarea.Focus()
			// cmds = append(cmds, m.spinner.Tick) // Spinner is managed based on state in View/Update

			// Save message to history if enabled
			if m.historyEnabled && m.historyManager != nil {
				m.historyManager.AddMessage(formatMessage("You", txt))
				// Auto-save on new message
				cmds = append(cmds, m.saveSessionCmd())
			}

			if m.useBidi && m.bidiStream != nil {
				sendCmd = m.sendToBidiStreamCmd(txt) // stream.go
			} else {
				sendCmd = m.sendToStreamCmd(txt) // stream.go
			}
			cmds = append(cmds, sendCmd)
		}

		// } else if m.currentState != AppStateReady {
		// 	err := fmt.Errorf("cannot send message: not in Chatting state (current: %s)", m.currentState)
		// 	log.Printf("Send ignored: %v", err)
		// 	// Optionally show a less intrusive message or just log it
		// 	// m.messages = append(m.messages, formatError(err)) // helpers.go
		//  // m.viewport.SetContent(m.renderAllMessages())
		// 	// m.viewport.GotoBottom()
		// 	m.err = err // Keep error state
		// }
		return m, tea.Batch(cmds...)
	default:
		// If no specific key was handled, return the model and any accumulated commands (e.g., from textarea update)
		return m, tea.Batch(cmds...)
	}
	return m, tea.Batch(cmds...)
}

// checkPlayback determines if key combinations will trigger audio playback
func (m *Model) checkPlayback(key string) (tea.Cmd, bool) {
	switch key {
	case "ctrl+p":
		// Check if there's audio in the last Gemini message
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Sender == senderNameModel && m.messages[i].HasAudio && len(m.messages[i].AudioData) > 0 {
				return nil, true // Playback would be triggered
			}
		}
	case "ctrl+r":
		// Check if there's currently playing/last played audio
		if m.currentAudio != nil && len(m.currentAudio.Data) > 0 {
			return nil, true // Replay would be triggered
		}
	}
	return nil, false
}

// handleStreamMsg handles messages related to the generative stream.
func (m *Model) handleStreamMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case initStreamMsg: // stream.go
		// If we're retrying after an error and client is not nil, reset the client to fix connection issues
		// If we're retrying after an error and client is not nil, reset the client to fix connection issues
		if m.client != nil && m.streamRetryAttempt > 0 {
			// Perform more thorough cleanup on retry

			// Close any existing streams first to ensure proper resource cleanup
			if m.stream != nil {
				m.stream.CloseSend()
				m.stream = nil
			}
			if m.bidiStream != nil {
				m.bidiStream.CloseSend()
				m.bidiStream = nil
			}

			// Close existing client connection
			if err := m.client.Close(); err != nil {
				log.Printf("Warning: Error closing client: %v", err)
			}

			// Short delay to allow socket cleanup
			time.Sleep(100 * time.Millisecond)

			// Create a new client instance
			m.client = &api.Client{}
			if m.apiKey != "" {
				m.client.APIKey = m.apiKey
			}
			log.Printf("Client fully reset for reconnection attempt %d", m.streamRetryAttempt)
		}
		m.messages = append(m.messages, formatMessage("System", "Connecting to Gemini..."))
		// UI will update automatically
		cmds = append(cmds, m.initStreamCmd()) // Start connection attempt

	case initClientCompleteMsg: // stream.go - Client initialization complete
		log.Println("Client initialization complete, transitioning to ready state")
		m.currentState = AppStateReady // Transition state from Initializing to Ready
		m.err = nil

		// Remove "Connecting..." message if present
		if len(m.messages) > 0 && strings.Contains(m.messages[len(m.messages)-1].Content, "Connecting") {
			m.messages = m.messages[:len(m.messages)-1]
		}

		// Add success message
		m.messages = append(m.messages, formatMessage("System", "Connected. You can start chatting."))
		if m.enableTools && m.toolManager != nil {
			m.messages = append(m.messages, formatMessage("Info", fmt.Sprintf("%v tools available.", len(m.toolManager.RegisteredToolDefs))))
		}

		// Reset retry state
		m.streamRetryAttempt = 0
		m.currentStreamBackoff = initialBackoffDuration

		// Start receiving messages from the appropriate stream
		if m.bidiStream != nil {
			log.Printf("[DEBUG] Starting bidirectional stream receive loop (bidiStream: %p)", m.bidiStream)
			cmds = append(cmds, m.receiveBidiStreamCmd())
		} else if m.stream != nil {
			log.Printf("[DEBUG] Starting regular stream receive loop (stream: %p)", m.stream)
			cmds = append(cmds, m.receiveStreamCmd())
		} else {
			log.Println("[ERROR] No stream available after initialization - this should not happen!")
		}

	case streamReadyMsg: // stream.go
		m.stream = msg.stream
		m.currentState = AppStateReady // Transition state
		m.err = nil
		if len(m.messages) > 0 && strings.Contains(m.messages[len(m.messages)-1].Content, "Connecting") {
			m.messages = m.messages[:len(m.messages)-1] // Remove "Connecting..." message
		}
		m.messages = append(m.messages, formatMessage("System", "Connected. You can start chatting."))
		m.messages = append(m.messages, formatMessage("Info", fmt.Sprintf("%v tools available.", len(m.toolManager.RegisteredToolDefs))))
		// UI will update automatically
		// Reset retry state on successful connection
		m.streamRetryAttempt = 0
		m.currentStreamBackoff = initialBackoffDuration
		log.Println("Stream connection successful, retry counter reset.")
		cmds = append(cmds, m.receiveStreamCmd()) // Start receiving

	case bidiStreamReadyMsg: // stream.go
		m.bidiStream = msg.stream
		m.stream = nil
		m.currentState = AppStateReady // Transition state
		m.err = nil
		if len(m.messages) > 0 && strings.Contains(m.messages[len(m.messages)-1].Content, "Connecting") {
			m.messages = m.messages[:len(m.messages)-1] // Remove "Connecting..." message
		}
		m.messages = append(m.messages, formatMessage("System", "Connected with bidirectional stream. You can start chatting."))
		// UI will update automatically
		// Reset retry state on successful connection
		m.streamRetryAttempt = 0
		m.currentStreamBackoff = initialBackoffDuration
		log.Println("Bidirectional stream connection successful, retry counter reset.")

		// Start receiving messages
		cmds = append(cmds, m.receiveBidiStreamCmd())

	case streamResponseMsg: // stream.go (One-way stream response)
		//m.currentState = AppStateWaiting // Ensure state reflects waiting
		if msg.output.Text != "" || len(msg.output.Audio) > 0 || msg.output.FunctionCall != nil || msg.output.GroundingMetadata != nil || len(msg.output.SafetyRatings) > 0 {
			// Process grounding, safety, tokens first
			m.ProcessGenerativeLanguageResponse(msg.output)

			log.Printf("Stream response received: %s", msg.output.Text)

			// Create or append message content
			var targetMsgIdx int = -1
			createNeeded := true
			if len(m.messages) > 0 {
				lastIdx := len(m.messages) - 1
				// Append if last message was from Gemini recently and not a tool call/result/code
				if m.messages[lastIdx].Sender == "Gemini" &&
					time.Since(m.messages[lastIdx].Timestamp) < 3*time.Second &&
					!m.messages[lastIdx].IsToolCall() &&
					!m.messages[lastIdx].IsExecutableCode &&
					!m.messages[lastIdx].IsExecutableCodeResult {
					targetMsgIdx = lastIdx
					createNeeded = false
				}
			}

			if createNeeded {
				newMessage := Message{
					Sender:    "Gemini",
					Content:   msg.output.Text,
					HasAudio:  len(msg.output.Audio) > 0,
					AudioData: msg.output.Audio, // Store complete audio for now
					Timestamp: time.Now(),
					// Safety/Grounding/Tokens/FunctionCall added by ProcessGenerativeLanguageResponse
				}
				// Add function call if present (ProcessGenerativeLanguageResponse handles this now)
				// if msg.output.FunctionCall != nil {
				// 	newMessage.FunctionCall = msg.output.FunctionCall
				// }
				m.messages = append(m.messages, newMessage)
				targetMsgIdx = len(m.messages) - 1
			} else {
				// Append text if present
				if msg.output.Text != "" {
					m.messages[targetMsgIdx].Content += msg.output.Text
					m.messages[targetMsgIdx].Timestamp = time.Now()
				}
				// Append audio data if present
				if len(msg.output.Audio) > 0 {
					m.messages[targetMsgIdx].HasAudio = true
					// Append audio data if needed, or handle consolidation logic if separate
					// This simple append might not be right if consolidation is active
					m.messages[targetMsgIdx].AudioData = append(m.messages[targetMsgIdx].AudioData, msg.output.Audio...)
				}
				// Update function call if present (ProcessGenerativeLanguageResponse handles this now)
				// if msg.output.FunctionCall != nil {
				// 	m.messages[targetMsgIdx].FunctionCall = msg.output.FunctionCall
				// }
			}

			// Handle audio playback trigger if audio is present and enabled
			if len(msg.output.Audio) > 0 && m.enableAudio && m.playerCmd != "" && targetMsgIdx >= 0 {
				// Use the consolidated audio data from the message (might need adjustment)
				audioToPlay := m.messages[targetMsgIdx].AudioData
				textContent := m.messages[targetMsgIdx].Content // Use current content
				log.Printf("[UI] Triggering playback for stream message #%d", targetMsgIdx)
				// This direct playback might conflict with consolidation logic.
				// Consider using consolidateAndPlayAudio here if using bidi-style consolidation.
				cmds = append(cmds, m.playAudioCmd(audioToPlay, textContent))
				m.messages[targetMsgIdx].IsPlaying = true // Optimistic UI
			}

			// Save message to history if enabled and content exists or it's a function call
			if m.historyEnabled && m.historyManager != nil && targetMsgIdx >= 0 && m.messages[targetMsgIdx].Content != "" {
				if createNeeded {
					m.historyManager.AddMessage(m.messages[targetMsgIdx])
				} else {
					// TODO: fix this

					//m.historyManager.UpdateLastMessage(m.messages[targetMsgIdx])
				}
				cmds = append(cmds, m.saveSessionCmd()) // Auto-save periodically
			}

			// UI will update automatically
			m.viewport.GotoBottom()
		}
		if msg.output.SetupComplete != nil {
			// Handle setup completion if needed
			m.messages = append(m.messages, formatMessage("System", "Setup complete."))
		}
		// Decide next state based on whether the stream is still active
		if m.stream != nil && m.currentState != AppStateQuitting {
			// If we received the final chunk, transition back to Chatting
			// if msg.output.IsFinalChunk {
			// 	m.currentState = AppStateChatting
			// } else {
			// 	m.currentState = AppStateWaiting // Stay waiting
			// }
			cmds = append(cmds, m.receiveStreamCmd()) // Continue receiving
		} else {
			// Stream closed or quitting
			// if m.currentState != AppStateQuitting {
			// 	log.Println("Stream closed or quitting, transitioning to AppStateQuitting.")

			// 	m.currentState = AppStateQuitting
			// }
		}

	case bidiStreamResponseMsg: // stream.go (Bidirectional stream response)
		m.ProcessGenerativeLanguageResponse(msg.output)
		if !msg.output.TurnComplete {
			m.currentState = AppStateResponding
		} else {
			m.currentState = AppStateReady // Transition state
		}

		// Handle TurnComplete for final response
		if msg.output.TurnComplete && msg.output.Text != "" {
			log.Printf("Turn complete received with text, ensuring message is created")
			// Make sure there's a message for this text in the UI
			newMessage := formatMessage("Gemini", msg.output.Text)
			newMessage.Timestamp = time.Now()

			// Check if we need to append to existing message or create new
			needToCreate := true
			if len(m.messages) > 0 {
				lastIdx := len(m.messages) - 1
				if m.messages[lastIdx].Sender == senderNameModel &&
					!m.messages[lastIdx].IsToolCall() &&
					!m.messages[lastIdx].IsExecutableCode &&
					!m.messages[lastIdx].IsExecutableCodeResult {
					// Found existing message to update
					needToCreate = false
					log.Printf("Updating existing message #%d", lastIdx)
					// Update content if it's different
					if m.messages[lastIdx].Content != msg.output.Text {
						m.messages[lastIdx].Content = msg.output.Text
						// UI will update automatically
						m.viewport.GotoBottom()
					}
				}
			}

			// Create new message if needed
			if needToCreate {
				log.Printf("Creating new message for text: %s", msg.output.Text)
				m.messages = append(m.messages, newMessage)
				// UI will update automatically
				m.viewport.GotoBottom()

				// Save to history if enabled
				if m.historyEnabled && m.historyManager != nil {
					m.historyManager.AddMessage(newMessage)
					cmds = append(cmds, m.saveSessionCmd())
				}
			}
		}

		// Check for tool calls in the response if tool support is enabled
		if m.enableTools { /* && m.currentState != AppStateProcessingTool && m.currentState != AppStateWaitingToolApproval */
			// --- Tool Call Handling ---
			// Check if the *current* response chunk contains a function call
			if msg.output.FunctionCall != nil {
				// Create a *new* message specifically for this tool call request
				// Note: This message won't contain regular text or audio from this chunk
				newMessage := Message{
					Sender:    senderNameModel,
					Timestamp: time.Now(),
				}

				jsonArgs, err := msg.output.FunctionCall.Args.MarshalJSON()
				if err != nil {
					log.Printf("Error marshaling function call arguments: %v", err)
					newMessage.Content = fmt.Sprintf("Error: %v", err)
				}
				newMessage.ToolCall = &ToolCall{
					ID:        msg.output.FunctionCall.Id, // Use .Id from the protobuf definition
					Name:      msg.output.FunctionCall.Name,
					Arguments: json.RawMessage(jsonArgs),
				}
				m.messages = append(m.messages, newMessage) // Add the dedicated tool call message

				// Now, extract the tool call details for processing
				// Assuming ExtractToolCalls can handle a single FunctionCall if needed,
				// or we adapt the logic here. Let's assume it returns a slice.
				toolCalls := ExtractToolCalls(&msg.output) // This should ideally extract from newMessage.ToolCall or msg.output.FunctionCall

				if len(toolCalls) > 0 {
					// Use centralized process tool calls, which handles both approval and execution
					// This will automatically check for auto-approved tool types
					m.currentState = AppStateWaiting // Transition state

					// Process the tool calls using our unified tool processing logic
					results, err := m.processToolCalls(toolCalls)
					if err != nil {
						log.Printf("Error auto-processing tool calls: %v", err)
						m.messages = append(m.messages, formatError(fmt.Errorf("tool call error: %w", err)))
						m.currentState = AppStateReady // Revert state on error
					} else if len(results) > 0 {
						// Add formatted messages for the results
						for _, res := range results {
							m.messages = append(m.messages, formatToolResultMessage(res.Id, res.Name, res.Response, ToolCallStatusCompleted))
						}
						// Send tool results back to model (this should be a command)
						cmds = append(cmds, m.sendToolResultsCmd(results))
						// State will transition back after results are sent/processed by the model
						// Keep AppStateProcessingTool until model responds or sending fails
					} else {
						// No results to send, transition back
						log.Println("Tool calls executed successfully but returned no results.")
						// Optionally add messages indicating no results were returned
						// for _, tc := range toolCalls {
						// 	m.messages = append(m.messages, helpers.formatToolResultMessage(tc.ID, tc.Name, json.RawMessage(`{"info": "No results returned"}`)))
						// }
						m.currentState = AppStateReady
					}
					m.pendingToolCalls = nil // Clear pending calls after auto-processing
				}
			}
		}

		// Handle executable code if present (before text/audio processing for this chunk)
		if msg.output.ExecutableCode != nil {
			log.Printf("Processing executable code for language: %s", msg.output.ExecutableCode.GetLanguage())
			execCodeMessage := formatExecutableCodeMessage(msg.output.ExecutableCode)
			m.messages = append(m.messages, execCodeMessage)
			// Potentially transition state if execution is required?
			// m.currentState = AppStateExecutingCode ?
			// UI will update automatically
			m.viewport.GotoBottom()
			log.Printf("Added executable code message for language: %s", msg.output.ExecutableCode.GetLanguage())
		}

		// Handle executable code result if present
		if msg.output.CodeExecutionResult != nil {
			execResultMessage := formatExecutableCodeResultMessage(msg.output.CodeExecutionResult)
			m.messages = append(m.messages, execResultMessage)
			// Transition state back if needed
			// m.currentState = AppStateChatting ?
			// UI will update automatically
			m.viewport.GotoBottom()
		}

		// Handle executable code if present
		if msg.output.ExecutableCode != nil {
			log.Printf("Processing executable code for language: %s", msg.output.ExecutableCode.GetLanguage())

			// Create and append an executable code message
			execCodeMessage := formatExecutableCodeMessage(msg.output.ExecutableCode)
			m.messages = append(m.messages, execCodeMessage)
			// UI will update automatically
			m.viewport.GotoBottom()
			log.Printf("Added executable code message for language: %s", msg.output.ExecutableCode.GetLanguage())
		}

		// Handle executable code result if present
		if msg.output.CodeExecutionResult != nil {
			// Create and append an executable code result message
			execResultMessage := formatExecutableCodeResultMessage(msg.output.CodeExecutionResult)
			m.messages = append(m.messages, execResultMessage)
			// UI will update automatically
			m.viewport.GotoBottom()
		}

		if msg.output.Text != "" || len(msg.output.Audio) > 0 {
			// Process regular text and audio, appending or creating messages
			// Always process text from the model
			// Only log details in verbose mode
			// Skip tool call formatting text if it contains tool call markers
			// This check might be redundant if tool calls are handled separately above
			if m.enableTools && strings.Contains(msg.output.Text, "[TOOL_CALL]") {
				// Tool call text shouldn't be displayed directly
			} else {
				var targetMsgIdx int = -1
				createNeeded := true
				if len(m.messages) > 0 {
					lastIdx := len(m.messages) - 1
					// Append if last message was from Gemini recently and not a tool call/result/code/func
					if m.messages[lastIdx].Sender == "Gemini" &&
						time.Since(m.messages[lastIdx].Timestamp) < 3*time.Second &&
						!m.messages[lastIdx].IsToolCall() &&
						!m.messages[lastIdx].IsExecutableCode &&
						!m.messages[lastIdx].IsExecutableCodeResult {
						targetMsgIdx = lastIdx
						createNeeded = false
					}
				}

				if createNeeded {
					newMessage := formatMessage("Gemini", msg.output.Text)
					newMessage.Timestamp = time.Now()
					// Safety/Grounding/Tokens added by ProcessGenerativeLanguageResponse
					m.messages = append(m.messages, newMessage)

					targetMsgIdx = len(m.messages) - 1

					// Save new message to history if enabled
					if m.historyEnabled && m.historyManager != nil && msg.output.Text != "" {
						m.historyManager.AddMessage(newMessage)
						cmds = append(cmds, m.saveSessionCmd()) // Auto-save periodically
					}

					if helpers.IsAudioTraceEnabled() {
						log.Printf("[AUDIO_PIPE] Created new message #%d for incoming bidi stream data", targetMsgIdx)
					}
				} else if !createNeeded && msg.output.Text != "" {
					// For streaming, append text. For complete messages (TurnComplete), replace
					if msg.output.TurnComplete {
						// Replace the entire text for complete messages to avoid duplication
						m.messages[targetMsgIdx].Content = msg.output.Text
					} else {
						// Append for streaming updates
						m.messages[targetMsgIdx].Content += msg.output.Text
					}
					m.messages[targetMsgIdx].Timestamp = time.Now()

					if helpers.IsAudioTraceEnabled() {
						log.Printf("[AUDIO_PIPE] Appending to existing message #%d for incoming bidi stream data", targetMsgIdx)
					}

					// Handle audio consolidation and playback trigger
					if len(msg.output.Audio) > 0 && m.enableAudio && m.playerCmd != "" && targetMsgIdx >= 0 {
						consolidateCmd := m.consolidateAndPlayAudio(msg.output.Audio, m.messages[targetMsgIdx].Content, targetMsgIdx) // audio_manager.go
						cmds = append(cmds, consolidateCmd)
						// Don't set IsPlaying here; let audioPlaybackStartedMsg handle it
					}

					// Mark HasAudio = true if there's audio data, regardless of whether audio is enabled
					if len(msg.output.Audio) > 0 && targetMsgIdx >= 0 {
						m.messages[targetMsgIdx].HasAudio = true
						// Store consolidated audio data even if playback is disabled?
						// The consolidation logic might need adjustment here.
						// For now, assume consolidateAndPlayAudio handles storage if needed.
						// if !m.enableAudio {
						// Store the audio data even if playback is disabled
						// m.messages[targetMsgIdx].AudioData = ??? // Needs consolidated data
						// }
					}
				}

				// UI will update automatically
				m.viewport.GotoBottom()
			}
		}

		// Decide next state based on whether the stream is still active
		if m.currentState != AppStateQuitting {
			// Only evaluate stream condition if in a state that uses the stream
			if m.currentState == AppStateWaiting {
				// Check if the message is a bidiStreamResponseMsg
				if msg.output.TurnComplete {
					m.currentState = AppStateReady // Transition back to Ready state

					// if bidiMsg, ok := msg.(bidiStreamResponseMsg); ok {
					// 	// If we received the final chunk, transition back to Ready,
					// 	// unless we are waiting for tool approval/processing.
					// 	if bidiMsg.output.TurnComplete {
					// 		log.Println("Turn complete, transitioning back to Ready state.")
					// 		// Only switch to Ready if we're not processing tools
					// 		if !m.processingTool && len(m.pendingToolCalls) == 0 {
					// 			m.currentState = AppStateReady
					// 		}
					// 	}
					// }
				}
			}

			// Continue receiving from stream if it exists
			if m.bidiStream != nil {
				cmds = append(cmds, m.receiveBidiStreamCmd())
			}
		} else {
			// Already in quitting state - logging only
			log.Println("Stream handling skipped (application is quitting)")
		}

	case sentMsg: // stream.go
		// Message sent, transition back to chatting/receiving if we were sending
		// if m.currentState == AppStateSending {
		// 	// If using bidi, we might immediately start receiving the response
		// 	if m.useBidi {
		// 		m.currentState = AppStateWaiting
		// 		// Start receiving the response immediately after sending
		// 		cmds = append(cmds, m.receiveBidiStreamCmd())
		// 	} else {
		// 		m.currentState = AppStateReady // Or Waiting if stream auto-responds?
		// 	}
		// }
		cmds = append(cmds, m.receiveBidiStreamCmd())

	case sendErrorMsg: // stream.go
		// Error sending, transition back to chatting or to error state?
		m.currentState = AppStateReady // Allow user to retry or type something else
		m.err = fmt.Errorf("send error: %w", msg.err)
		m.messages = append(m.messages, formatError(m.err))
		// UI will update automatically

	case streamErrorMsg: // stream.go
		// Error receiving or connecting
		// Categorize the error for better handling
		var errorCategory string
		var shouldRetry bool = true

		// Check for specific error patterns
		errString := msg.err.Error()
		switch {
		case strings.Contains(errString, "bad file descriptor"):
			errorCategory = "socket descriptor issue"
			// This is a socket-level issue that needs thorough cleanup
			runtime.GC() // Try to clean up unused file descriptors
		case strings.Contains(errString, "connection refused"):
			errorCategory = "connection refused"
		case strings.Contains(errString, "connection reset"):
			errorCategory = "connection reset"
		case strings.Contains(errString, "context canceled"):
			errorCategory = "context canceled"
			// Don't retry canceled contexts
			shouldRetry = false
		case strings.Contains(errString, "deadline exceeded"):
			errorCategory = "timeout"
		case strings.Contains(errString, "certificate"):
			errorCategory = "TLS/certificate issue"
		case strings.Contains(errString, "rate limit"):
			errorCategory = "rate limited"
		case strings.Contains(errString, "model only supports text output") ||
			strings.Contains(errString, "Multi-modal output is not supported"):
			errorCategory = "model compatibility error"
			// Don't retry when model doesn't support requested features (like audio)
			shouldRetry = false
		case strings.Contains(errString, "InvalidArgument"):
			errorCategory = "invalid argument"
			// Don't retry API argument errors - they won't resolve with retries
			shouldRetry = false
		case strings.Contains(errString, "NotFound"):
			errorCategory = "model not found"
			// Don't retry model not found errors - they won't resolve with retries
			shouldRetry = false
		case strings.Contains(errString, "RESOURCE_EXHAUSTED") || strings.Contains(errString, "ResourceExhausted") || strings.Contains(errString, "quota"):
			errorCategory = "resource exhausted"
			// Don't retry resource exhaustion errors immediately
			shouldRetry = false
			// Add longer backoff for rate limiting
			m.currentStreamBackoff = time.Duration(float64(m.currentStreamBackoff) * 2)
		default:
			errorCategory = "unknown error"
		}

		// Clean up resources
		m.stream = nil // Ensure streams are nil
		m.bidiStream = nil
		if m.streamCtxCancel != nil { // Cancel existing context if any
			m.streamCtxCancel()
			m.streamCtxCancel = nil
		}

		log.Printf("Stream error detected: %s - %v", errorCategory, msg.err)

		// Only increment retry counter if we're going to retry
		if shouldRetry {
			m.streamRetryAttempt++
		}

		if !shouldRetry || m.streamRetryAttempt > maxStreamRetries {
			// Max retries reached or non-retryable error - Transition to Error state
			var errorDetails string
			if shouldRetry {
				errorDetails = fmt.Sprintf("Max retries (%d) reached. Giving up.", maxStreamRetries)
			} else {
				if errorCategory == "model compatibility error" {
					errorDetails = "This model doesn't support the requested features (e.g., audio output)."
				} else if errorCategory == "invalid argument" {
					errorDetails = "The API rejected the connection due to invalid arguments."
				} else if errorCategory == "resource exhausted" {
					errorDetails = "The API has exhausted its resources. Try again later."
				} else {
					errorDetails = "Error is not retryable."
				}
			}
			log.Printf("Stream error: %s - %v. %s", errorCategory, msg.err, errorDetails)

			m.currentState = AppStateError
			if !shouldRetry {
				m.err = fmt.Errorf("stream error (%s): %w", errorCategory, msg.err)
			} else {
				m.err = fmt.Errorf("stream failed after %d retries (%s): %w", maxStreamRetries, errorCategory, msg.err)
			}

			// Create a detailed error message for the user
			errorMessage := formatMessage("System", fmt.Sprintf("Error: %s\nDetails: %v\n\n%s\n\nTry changing models or disabling audio if using a model that doesn't support it.",
				errorCategory, msg.err, errorDetails))

			m.messages = append(m.messages, errorMessage)
			// UI will update automatically
			m.viewport.GotoBottom()

			// Exit with non-zero code if this is the initial connection attempt
			if len(m.messages) <= 2 { // Only system messages or startup messages
				log.Printf("Failed to establish initial connection. Exiting with error code 1")
				fmt.Fprintf(os.Stderr, "Error: Failed to establish initial connection: %v\n", m.err)
				// Signal to quit with error code
				m.exitCode = 1
				cmds = append(cmds, func() tea.Msg {
					return tea.Quit()
				})
			}

			// Reset retry state for potential future manual attempts
			m.streamRetryAttempt = 0
			m.currentStreamBackoff = initialBackoffDuration
		} else {
			// Attempting retry - Transition back to Initializing state
			m.currentState = AppStateInitializing

			// Ensure our backoff has some jitter to avoid thundering herd issues
			jitter := time.Duration(float64(m.currentStreamBackoff) * jitterFactor * (rand.Float64()*2 - 1)) // +/- jitterFactor
			delay := m.currentStreamBackoff + jitter
			if delay < 0 {
				delay = 100 * time.Millisecond // Ensure delay is not negative
			}

			log.Printf("Stream error (%s): Attempt %d/%d failed. Retrying in %v. Error: %v",
				errorCategory, m.streamRetryAttempt, maxStreamRetries, delay, msg.err)

			m.messages = append(m.messages, formatMessage("System",
				fmt.Sprintf("Connection error (%s). Retrying in %v (attempt %d/%d)...\nDetails: %v",
					errorCategory, delay.Round(time.Second), m.streamRetryAttempt, maxStreamRetries, msg.err)))

			// UI will update automatically
			m.viewport.GotoBottom()

			// Update backoff for the *next* attempt
			m.currentStreamBackoff = time.Duration(float64(m.currentStreamBackoff) * backoffFactor)
			if m.currentStreamBackoff > maxBackoffDuration {
				m.currentStreamBackoff = maxBackoffDuration
			}

			// Schedule the retry with the calculated delay
			cmds = append(cmds, tea.Tick(delay, func(t time.Time) tea.Msg {
				return initStreamMsg{} // Trigger reconnection attempt
			}))
		}

	case streamClosedMsg: // stream.go
		log.Println("Stream closed cleanly or unexpectedly.")
		// Stream closed cleanly or unexpectedly (but not an error handled by streamErrorMsg)
		if m.currentState != AppStateQuitting && m.currentState != AppStateError {
			// Automatically reconnect instead of showing user message
			// This provides a seamless experience for users
			if m.currentState != AppStateInitializing {
				log.Println("Stream closed - automatically reconnecting...")
				m.currentState = AppStateInitializing

				// Clean up old stream references
				m.stream = nil
				m.bidiStream = nil

				// Initiate automatic reconnection
				cmds = append(cmds, m.initStreamCmd())
			} else {
				// When already in retry process or initializing, just log it
				log.Println("Stream closed while initializing - continuing flow")
			}
		} else {
			// Clean up when quitting or in error state
			m.stream = nil
			m.bidiStream = nil
		}

		// Avoid canceling context during reconnection flow
		if m.streamCtxCancel != nil && m.streamRetryAttempt == 0 && m.currentState == AppStateQuitting {
			m.streamCtxCancel()
			m.streamCtxCancel = nil
		}
	}
	return m, tea.Batch(cmds...)
}

// monitorConnection monitors the gRPC/WebSocket connection health and logs debugging information
func (m *Model) monitorConnection() {
	log.Printf("[DEBUG] Starting connection health monitoring")

	ticker := time.NewTicker(DebuggingLogInterval)
	defer ticker.Stop()

	startTime := time.Now()
	lastLogTime := startTime

	for {
		select {
		case <-ticker.C:
			// Check if context is still valid
			if m.streamCtx == nil {
				log.Printf("[DEBUG] Stream context is nil, stopping connection monitor")
				return
			}

			if m.streamCtx.Err() != nil {
				log.Printf("[DEBUG] Stream context error: %v, stopping connection monitor", m.streamCtx.Err())
				return
			}

			// Log connection health
			now := time.Now()
			uptime := now.Sub(startTime)
			timeSinceLastLog := now.Sub(lastLogTime)

			log.Printf("[DEBUG] Connection health check - Uptime: %v, Since last log: %v",
				uptime.Truncate(time.Second), timeSinceLastLog.Truncate(time.Second))

			// Log connection state details
			if m.bidiStream != nil {
				log.Printf("[DEBUG] Bidirectional stream is active")
			} else if m.stream != nil {
				log.Printf("[DEBUG] Unidirectional stream is active")
			} else {
				log.Printf("[DEBUG] No active stream connection")
			}

			// Log current application state
			log.Printf("[DEBUG] App state: %v, Retry attempt: %d", m.currentState, m.streamRetryAttempt)

			lastLogTime = now

		case <-m.streamCtx.Done():
			log.Printf("[DEBUG] Stream context cancelled, stopping connection monitor")
			return
		}
	}
}

// autoSendCmd returns a command that sends a test message after the configured delay
func (m *Model) autoSendCmd() tea.Cmd {
	return tea.Tick(m.autoSendDelay, func(t time.Time) tea.Msg {
		return autoSendMsg{}
	})
}

// Experimental integrations disabled - moved to .wip files
// GetMCPIntegration returns the MCP integration instance
// func (m *Model) GetMCPIntegration() *MCPIntegration {
// 	return m.mcpIntegration
// }

// GetVoiceStreamer returns the voice streamer instance
// func (m *Model) GetVoiceStreamer() *VoiceStreamer {
// 	return m.voiceStreamer
// }

// GetVideoStreamer returns the video streamer instance
// func (m *Model) GetVideoStreamer() *VideoStreamer {
// 	return m.videoStreamer
// }

// NewModel creates a new aistudio model with functional options for configuration.
func NewModel(opts ...Option) (*Model, error) {
	model := New(opts...)
	if model == nil {
		return nil, fmt.Errorf("failed to create model")
	}
	return model, nil
}
