package aistudio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/ai/generativelanguage/apiv1alpha/generativelanguagepb"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tmc/aistudio/api"
	"github.com/tmc/aistudio/audioplayer"
	"github.com/tmc/aistudio/internal/helpers"
	"github.com/tmc/aistudio/settings"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	initialBackoffDuration = 1 * time.Second
	maxBackoffDuration     = 30 * time.Second
	backoffFactor          = 1.8
	jitterFactor           = 0.2 // +/- 20%
	maxStreamRetries       = 5
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
	ta.SetWidth(50)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)

	vp := viewport.New(50, 5)
	vp.SetContent("Initializing...")

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
		useBidi:                true,                       // Default to using BidiGenerateContent
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
		historyEnabled: false,
		enableTools:    false,

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

	// --- Continue with the original Init behavior ---
	// Start with ready state
	//m.currentState = AppStateReady

	// Add welcome message command
	cmds = append(cmds, tea.Printf("\n\033[1;33m%s:\033[0m %s\n", "System",
		fmt.Sprintf("Welcome to aistudio! Model: %s", m.modelName)))

	cmds = append(cmds, m.setupInitialUICmd())
	cmds = append(cmds, m.initStreamCmd())

	return tea.Batch(cmds...)
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
	config := api.ClientConfig{
		ModelName:    m.modelName,
		EnableAudio:  false, // Disable audio in stdin mode
		SystemPrompt: m.systemPrompt,
		// Add other config options as needed
		Temperature:     m.temperature,
		TopP:            m.topP,
		TopK:            m.topK,
		MaxOutputTokens: m.maxOutputTokens,
	}

	// Add tool definitions if enabled
	if m.enableTools && m.toolManager != nil {
		config.ToolDefinitions = m.toolManager.GetAvailableTools()
	}

	// Initialize bidirectional stream
	var err error
	m.bidiStream, err = m.client.InitBidiStream(m.rootCtx, config)
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
					results, err := m.executeToolCalls(toolCalls)
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

		// Output response
		fmt.Println(responseText.String())
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin read error: %w", err)
	}

	return nil
}

// initCmd returns the command sequence to start the application logic.
func (m *Model) initCmd() tea.Cmd {
	// Transition state before returning the command
	//m.currentState = AppStateInitializing
	return tea.Batch(
		// m.spinner.Tick, // Spinner tick is handled globally now based on state
		tea.Sequence(
			waitForFocusCmd(),                         // Defined in stream.go
			func() tea.Msg { return initStreamMsg{} }, // Defined in stream.go
		),
	)
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
	// else { // No need for else, handled below if not settings panel
	// Otherwise handle key in main UI
	// Note: Textarea update happens in the main Update function before this handler
	// m.textarea, taCmd = m.textarea.Update(msg) // Update textarea here
	// cmds = append(cmds, taCmd)
	// }

	switch msg.String() {
	// Add handlers for Y/N keys for tool approval
	case "y", "Y": // Approve tool call
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

			// Update UI
			m.viewport.SetContent(m.formatAllMessages())
			m.viewport.GotoBottom()
			return m, tea.Batch(cmds...)
		}

	case "n", "N": // Deny tool call
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
			m.viewport.SetContent(m.formatAllMessages())
			m.viewport.GotoBottom()
			// Update UI
			m.viewport.SetContent(m.formatAllMessages())
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
		if m.stream != nil {
			cmds = append(cmds, m.closeStreamCmd())
		}
		if m.bidiStream != nil {
			cmds = append(cmds, m.closeBidiStreamCmd())
		}
		// Let the main Update loop handle tea.Quit
		// return m, tea.Quit
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

	case "ctrl+m": // Toggle Mic state
		m.micActive = !m.micActive
		log.Printf("Mic input simulation toggled: %v", m.micActive)
		if m.micActive {
			m.videoInputMode = VideoInputNone
		}
		return m, tea.Batch(cmds...) // Return early

	case "ctrl+t": // Show available tools
		if m.enableTools && m.toolManager != nil {
			// Get list of available tools
			toolsList := m.toolManager.ListAvailableTools()

			// Display available tools
			m.messages = append(m.messages, formatMessage("System", toolsList))
			m.viewport.SetContent(m.formatAllMessages())
			m.viewport.GotoBottom()
		} else {
			// Display a message about tools being disabled
			m.messages = append(m.messages, formatMessage("System", "Tool calling is disabled. Enable with --tools flag."))
			m.viewport.SetContent(m.formatAllMessages())
			m.viewport.GotoBottom()
		}
		return m, tea.Batch(cmds...) // Return early

	case "ctrl+v": // Toggle Video state
		switch m.videoInputMode {
		case VideoInputNone:
			m.videoInputMode = VideoInputCamera
		case VideoInputCamera:
			m.videoInputMode = VideoInputScreen
		case VideoInputScreen:
			m.videoInputMode = VideoInputNone
		}
		log.Printf("Video input simulation toggled: %s", m.videoInputMode)
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
			m.viewport.SetContent(m.formatAllMessages())
			m.viewport.GotoBottom()
		} else {
			// Display a message about history being disabled
			m.messages = append(m.messages, formatMessage("System", "Chat history is disabled. Enable with --history flag."))
			m.viewport.SetContent(m.formatAllMessages())
			m.viewport.GotoBottom()
		}
		return m, tea.Batch(cmds...)

	case "enter": // Send message
		if txt := strings.TrimSpace(m.textarea.Value()); txt != "" {
			log.Printf("Sending message: %s", txt)
			m.messages = append(m.messages, formatMessage("You", txt)) // helpers.go

			// Print to stdout through Bubble Tea to avoid rendering clashes
			cmds = append(cmds, tea.Printf("\n\033[1m%s:\033[0m %s\n", "You", txt))
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
		//  // m.viewport.SetContent(m.formatAllMessages())
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
		if m.client != nil && m.streamRetryAttempt > 0 {
			// Close existing client connection
			if err := m.client.Close(); err != nil {
				log.Printf("Warning: Error closing client: %v", err)
			}
			// Create a new client instance
			m.client = &api.Client{}
			if m.apiKey != "" {
				m.client.APIKey = m.apiKey
			}
			log.Println("Client reset for reconnection attempt")
		}

		m.messages = append(m.messages, formatMessage("System", "Connecting to Gemini..."))
		// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
		m.viewport.GotoBottom()
		cmds = append(cmds, m.initStreamCmd()) // Start connection attempt

	case streamReadyMsg: // stream.go
		m.stream = msg.stream
		m.currentState = AppStateReady // Transition state
		m.err = nil
		if len(m.messages) > 0 && strings.Contains(m.messages[len(m.messages)-1].Content, "Connecting") {
			m.messages = m.messages[:len(m.messages)-1] // Remove "Connecting..." message
		}
		m.messages = append(m.messages, formatMessage("System", "Connected. You can start chatting."))
		m.messages = append(m.messages, formatMessage("Info", fmt.Sprintf("%v tools available.", len(m.toolManager.RegisteredToolDefs))))
		// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
		m.viewport.GotoBottom()
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
		// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
		m.viewport.GotoBottom()
		// Reset retry state on successful connection
		m.streamRetryAttempt = 0
		m.currentStreamBackoff = initialBackoffDuration
		log.Println("Bidirectional stream connection successful, retry counter reset.")
		cmds = append(cmds, m.receiveBidiStreamCmd()) // Start receiving

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
					!m.messages[lastIdx].IsToolCall &&
					!m.messages[lastIdx].IsExecutableCode &&
					!m.messages[lastIdx].IsExecutableCodeResult &&
					m.messages[lastIdx].FunctionCall == nil { // Also check for function call
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
			if m.historyEnabled && m.historyManager != nil && targetMsgIdx >= 0 && (m.messages[targetMsgIdx].Content != "" || m.messages[targetMsgIdx].FunctionCall != nil) {
				if createNeeded {
					m.historyManager.AddMessage(m.messages[targetMsgIdx])
				} else {
					// TODO: fix this

					//m.historyManager.UpdateLastMessage(m.messages[targetMsgIdx])
				}
				cmds = append(cmds, m.saveSessionCmd()) // Auto-save periodically
			}

			// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
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
				if m.messages[lastIdx].Sender == "Gemini" &&
					!m.messages[lastIdx].IsToolCall &&
					!m.messages[lastIdx].IsExecutableCode &&
					!m.messages[lastIdx].IsExecutableCodeResult &&
					m.messages[lastIdx].FunctionCall == nil {
					// Found existing message to update
					needToCreate = false
					log.Printf("Updating existing message #%d", lastIdx)
					// Update content if it's different
					if m.messages[lastIdx].Content != msg.output.Text {
						m.messages[lastIdx].Content = msg.output.Text
						// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
						m.viewport.GotoBottom()
					}
				}
			}

			// Create new message if needed
			if needToCreate {
				log.Printf("Creating new message for text: %s", msg.output.Text)
				m.messages = append(m.messages, newMessage)
				// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
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
					Sender:     senderNameModel,
					IsToolCall: true,
					Timestamp:  time.Now(),
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

				// Add tool calls to pending list for approval/execution
				m.pendingToolCalls = append(m.pendingToolCalls, toolCalls...)

				if m.requireApproval {
					// Transition state to wait for user approval
					m.currentState = AppStateWaiting
					m.showToolApproval = true
					m.approvalIndex = 0 // Start with the first pending call
					log.Printf("Waiting for user approval for %d tool call(s)", len(m.pendingToolCalls))
					// Update UI to show modal
					// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI // Refresh to potentially show modal
					m.viewport.GotoBottom()
				} else {
					// Auto-approve and process
					m.currentState = AppStateWaiting // Transition state
					log.Printf("Auto-processing %d tool call(s)", len(toolCalls))

					// Add formatted messages for executing the tool calls
					for _, tc := range toolCalls {
						m.messages = append(m.messages, formatToolCallMessage(tc, "Executing...")) // Use helper
					}
					// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI // Update UI to show "Executing"
					m.viewport.GotoBottom()

					// Process the tool calls (this might need to become a command)
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
			// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
			m.viewport.GotoBottom()
			log.Printf("Added executable code message for language: %s", msg.output.ExecutableCode.GetLanguage())
		}

		// Handle executable code result if present
		if msg.output.CodeExecutionResult != nil {
			execResultMessage := formatExecutableCodeResultMessage(msg.output.CodeExecutionResult)
			m.messages = append(m.messages, execResultMessage)
			// Transition state back if needed
			// m.currentState = AppStateChatting ?
			// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
			m.viewport.GotoBottom()
		}

		// Handle executable code if present
		if msg.output.ExecutableCode != nil {
			log.Printf("Processing executable code for language: %s", msg.output.ExecutableCode.GetLanguage())

			// Create and append an executable code message
			execCodeMessage := formatExecutableCodeMessage(msg.output.ExecutableCode)
			m.messages = append(m.messages, execCodeMessage)
			// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
			m.viewport.GotoBottom()
			log.Printf("Added executable code message for language: %s", msg.output.ExecutableCode.GetLanguage())
		}

		// Handle executable code result if present
		if msg.output.CodeExecutionResult != nil {
			// Create and append an executable code result message
			execResultMessage := formatExecutableCodeResultMessage(msg.output.CodeExecutionResult)
			m.messages = append(m.messages, execResultMessage)
			// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
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
						!m.messages[lastIdx].IsToolCall &&
						!m.messages[lastIdx].IsExecutableCode &&
						!m.messages[lastIdx].IsExecutableCodeResult &&
						m.messages[lastIdx].FunctionCall == nil {
						targetMsgIdx = lastIdx
						createNeeded = false
					}
				}

				if createNeeded {
					newMessage := formatMessage("Gemini", msg.output.Text)
					newMessage.Timestamp = time.Now()
					// Safety/Grounding/Tokens added by ProcessGenerativeLanguageResponse
					m.messages = append(m.messages, newMessage)

					// Print Gemini response to stdout with rich formatting using tea.Printf
					// to avoid rendering clashes with Bubble Tea
					cmds = append(cmds, tea.Printf("\n\033[1;35m%s:\033[0m %s", "Gemini", msg.output.Text))

					// If there's audio data, print a player status indicator
					if len(msg.output.Audio) > 0 {
						duration := float64(len(msg.output.Audio)) / 48000.0
						cmds = append(cmds, tea.Printf("\n    \033[36m🔊 Audio: %.1f seconds\033[0m", duration))
					}
					// Add newline at the end when full message received
					if msg.output.TurnComplete {
						cmds = append(cmds, tea.Printf("\n"))
					}
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
					// Append text to existing message
					m.messages[targetMsgIdx].Content += msg.output.Text
					m.messages[targetMsgIdx].Timestamp = time.Now()

					// Print the appended text to stdout using tea.Printf
					// 			cmds = append(cmds, tea.Printf("%s", msg.output.Text))

					// 			// If this is the final chunk, end with a newline
					// 			if msg.output.TurnComplete {
					// 				cmds = append(cmds, tea.Printf("\n"))
					// 			}

					// 		// Update message in history
					// 		// TODO: fix this
					// 		// if m.historyEnabled && m.historyManager != nil {
					// 		// 	m.historyManager.UpdateLastMessage(m.messages[targetMsgIdx])
					// 		// }
					// 	}
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

				// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
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
		// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
		m.viewport.GotoBottom()

	case streamErrorMsg: // stream.go
		// Error receiving or connecting
		m.stream = nil // Ensure streams are nil
		m.bidiStream = nil
		if m.streamCtxCancel != nil { // Cancel existing context if any
			m.streamCtxCancel()
			m.streamCtxCancel = nil
		}

		m.streamRetryAttempt++

		if m.streamRetryAttempt > maxStreamRetries {
			// Max retries reached - Transition to Error state
			log.Printf("Stream error: Max retries (%d) reached. Giving up. Error: %v", maxStreamRetries, msg.err)
			m.currentState = AppStateError
			m.err = fmt.Errorf("stream failed after %d retries: %w", maxStreamRetries, msg.err)
			m.messages = append(m.messages, formatError(m.err))
			// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
			m.viewport.GotoBottom()
			// Reset retry state for potential future manual attempts? Or require restart?
			// m.streamRetryAttempt = 0
			// m.currentStreamBackoff = initialBackoffDuration
		} else {
			// Attempting retry - Transition back to Initializing state
			m.currentState = AppStateInitializing
			jitter := time.Duration(float64(m.currentStreamBackoff) * jitterFactor * (rand.Float64()*2 - 1)) // +/- jitterFactor
			delay := m.currentStreamBackoff + jitter
			if delay < 0 {
				delay = 100 * time.Millisecond // Ensure delay is not negative
			}

			log.Printf("Stream error: Attempt %d/%d failed. Retrying in %v. Error: %v", m.streamRetryAttempt, maxStreamRetries, delay, msg.err)
			m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Connection error. Retrying in %v (attempt %d/%d)...", delay.Round(time.Second), m.streamRetryAttempt, maxStreamRetries)))
			// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
			m.viewport.GotoBottom()

			// Update backoff for the *next* attempt
			m.currentStreamBackoff = time.Duration(float64(m.currentStreamBackoff) * backoffFactor)
			if m.currentStreamBackoff > maxBackoffDuration {
				m.currentStreamBackoff = maxBackoffDuration
			}

			// Schedule the retry
			cmds = append(cmds, tea.Tick(delay, func(t time.Time) tea.Msg {
				return initStreamMsg{} // Trigger reconnection attempt
			}))
		}

	case streamClosedMsg: // stream.go
		log.Println("Stream closed cleanly or unexpectedly.")
		// Stream closed cleanly or unexpectedly (but not an error handled by streamErrorMsg)
		if m.currentState != AppStateQuitting && m.currentState != AppStateError {
			// Only show stream closed message if we're not just initializing
			if m.currentState != AppStateInitializing && m.streamRetryAttempt == 0 {
				// Add error message
				m.messages = append(m.messages, formatMessage("System", "Stream closed. Type a message to reconnect."))

				// Print message to stdout through Bubble Tea to avoid rendering clashes
				cmds = append(cmds, tea.Printf("\n\033[1;33m%s:\033[0m %s\n", "System",
					"Stream closed. Type a message to reconnect."))

				// Set to ready state to allow reconnection
				m.currentState = AppStateReady
				m.viewport.GotoBottom()
			} else {
				// When already in retry process or initializing, just log it
				log.Println("Stream closed while retrying connection or initializing - continuing flow")
			}
		}
		m.stream = nil
		m.bidiStream = nil
		// Avoid canceling context during reconnection flow
		if m.streamCtxCancel != nil && m.streamRetryAttempt == 0 {
			m.streamCtxCancel()
			m.streamCtxCancel = nil
		}
	}
	return m, tea.Batch(cmds...)
}

// handleAudioMsg handles messages related to audio playback and processing.
func (m *Model) handleAudioMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	// startPlaybackTicker := false // Handled in main Update

	switch msg := msg.(type) {
	case audioPlaybackStartedMsg: // types.go
		log.Printf("[UI] Received audioPlaybackStartedMsg: Msg #%d, Size=%d", msg.chunk.MessageIndex, len(msg.chunk.Data))
		m.isAudioProcessing = true // Mark that playback is active
		idx := msg.chunk.MessageIndex
		if idx >= 0 && idx < len(m.messages) {
			m.messages[idx].IsPlaying = true
			m.messages[idx].IsPlayed = false
		} else {
			log.Printf("[UI Warning] audioPlaybackStartedMsg: Invalid message index %d", idx)
		}
		// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI // Update UI

		// Make sure textarea maintains focus when audio starts playing
		if m.focusedComponent == "input" {
			m.textarea.Focus()
		}
		// startPlaybackTicker = true // Signal to start ticker - Handled in main Update

	case audioPlaybackCompletedMsg: // types.go
		log.Printf("[UI] Received audioPlaybackCompletedMsg: Msg #%d, Size=%d", msg.chunk.MessageIndex, len(msg.chunk.Data))
		m.isAudioProcessing = false // Mark playback as inactive AFTER processing this message
		idx := msg.chunk.MessageIndex
		if idx >= 0 && idx < len(m.messages) {
			if m.messages[idx].IsPlaying { // Check if it was the one playing
				m.messages[idx].IsPlaying = false
				m.messages[idx].IsPlayed = true
			} else {
				log.Printf("[UI Warning] audioPlaybackCompletedMsg: Message #%d was not marked as playing.", idx)
				// Mark as played anyway if not already
				if !m.messages[idx].IsPlayed {
					m.messages[idx].IsPlayed = true
				}
			}
		} else {
			log.Printf("[UI Warning] audioPlaybackCompletedMsg: Invalid message index %d", idx)
		}
		// Remove completed chunk from UI queue
		for i, qChunk := range m.audioQueue {
			// Simple comparison; might need more robust check if multiple identical chunks possible
			if bytes.Equal(qChunk.Data, msg.chunk.Data) && qChunk.MessageIndex == msg.chunk.MessageIndex {
				m.audioQueue = append(m.audioQueue[:i], m.audioQueue[i+1:]...)
				break
			}
		}
		// Clear current audio reference *after* processing completion
		if m.currentAudio != nil && bytes.Equal(m.currentAudio.Data, msg.chunk.Data) {
			m.currentAudio = nil
		}
		// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI // Update UI

		// Ensure textarea has focus after audio playback completes
		// This is critical for keeping the input prompt visible
		if m.focusedComponent == "input" {
			m.textarea.Focus()
		}

	case audioQueueUpdatedMsg: // types.go
		// Optional: Could trigger a footer refresh if needed
		// log.Println("[UI] Audio queue updated.")

	case audioPlaybackErrorMsg: // types.go
		audioErr := fmt.Errorf("audio playback error: %w", msg.err)
		m.messages = append(m.messages, formatError(audioErr))
		// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
		m.viewport.GotoBottom()
		m.err = nil                 // Clear main error status line, show in history
		m.isAudioProcessing = false // Ensure processing stops on error

	case flushAudioBufferMsg: // types.go
		if len(m.consolidatedAudioData) > 0 {
			log.Printf("[UI] Handling audio buffer flush trigger (%d bytes)", len(m.consolidatedAudioData))
			cmds = append(cmds, m.flushAudioBuffer()) // audio_manager.go
		}

	case playbackTickMsg: // types.go
		// Only update UI and continue ticking if audio is actually playing
		if m.isAudioProcessing {
			// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI // Refresh view for progress bar

			// Ensure textarea stays focused during playback ticks
			if m.focusedComponent == "input" {
				m.textarea.Focus()
			}

			cmds = append(cmds, playbackTickCmd()) // Continue ticking
		} else {
			// Audio stopped, ensure ticker flag is off
			m.tickerRunning = false
			log.Println("[UI] Playback stopped, ticker halting.")
			// Do NOT return playbackTickCmd()

			// Make sure we regain focus when playback stops
			if m.focusedComponent == "input" {
				m.textarea.Focus()
			}
		}
	}
	return m, tea.Batch(cmds...)
}

// handleToolMsg handles messages related to tool calls and results.
func (m *Model) handleToolMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case toolCallSentMsg:
		// Tool call results sent successfully, nothing to do
		log.Println("Tool call results sent successfully.")

	case toolCallResultMsg:
		// Results from tool calls, might be displayed or logged
		log.Printf("Received %d tool call results", len(msg.results))
		// Potentially add a system message here if needed
		// m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Processed %d tool results.", len(msg.results))))
		// // m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
		// m.viewport.GotoBottom()
	}
	return m, tea.Batch(cmds...)
}

// handleHistoryMsg handles messages related to chat history loading and saving.
func (m *Model) handleHistoryMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case historyLoadedMsg:
		if msg.session != nil {
			m.loadMessagesFromSession(msg.session)
			m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Loaded chat session: %s", msg.session.Title)))
			// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
			m.viewport.GotoBottom()
		}

	case historyLoadFailedMsg:
		m.messages = append(m.messages, formatError(fmt.Errorf("failed to load history: %w", msg.err)))
		// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
		m.viewport.GotoBottom()

	case historySavedMsg:
		// History saved successfully, could show a notification
		log.Println("Chat history saved successfully")

	case historySaveFailedMsg:
		m.messages = append(m.messages, formatError(fmt.Errorf("failed to save history: %w", msg.err)))
		// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
		m.viewport.GotoBottom()
	}
	return m, tea.Batch(cmds...)
}

// handleSystemMsg handles general system messages like ticks and window size changes.
func (m *Model) handleSystemMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case spinner.TickMsg:
		// Handled by spinner update in the main Update loop

	case tea.WindowSizeMsg:
		// Prevent zero dimensions
		m.width = max(msg.Width, 20)
		m.height = max(msg.Height, 10)

		// FIXME: We don't need all this viewport calculation anymore since we're not rendering
		// the full TUI with viewport. This can be simplified in the future. We keep it working
		// for now to avoid breaking anything.

		headerHeight := lipgloss.Height(m.headerView()) // ui.go
		tempM := m
		tempM.showLogMessages = false
		tempM.showAudioStatus = false
		footerHeight := lipgloss.Height(tempM.footerView()) // ui.go
		if m.showLogMessages && len(m.logMessages) > 0 {
			footerHeight += lipgloss.Height(m.logMessagesView()) + 1 // ui.go
		}
		if m.showAudioStatus && (m.isAudioProcessing || len(m.audioQueue) > 0 || m.currentAudio != nil) {
			footerHeight += lipgloss.Height(m.audioStatusView()) + 1 // ui.go
		}

		vpHeight := m.height - headerHeight - footerHeight
		if vpHeight < 1 {
			vpHeight = 1
		}

		// Size viewport proportionally (not rendered but kept for backward compatibility)
		m.viewport.Width = m.width
		m.viewport.Height = vpHeight

		// Always ensure textarea is visible with proper width
		m.textarea.SetWidth(m.width)
		m.textarea.SetHeight(1) // One line is enough

		// Update viewport content (not rendered but kept for backward compatibility)
		// m.viewport.SetContent(m.formatAllMessages()) // Not needed with simplified UI
		m.viewport.GotoBottom()

	default:
		// Ignore unknown messages handled elsewhere or not relevant here
	}
	return m, tea.Batch(cmds...)
}

// Update handles incoming messages and updates the model state.
// It acts as the main dispatcher, delegating to helper functions.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		taCmd               tea.Cmd
		vpCmd               tea.Cmd
		spCmd               tea.Cmd
		cmds                []tea.Cmd
		helperCmd           tea.Cmd // Command returned by helper function
		startPlaybackTicker bool    // Flag to start the ticker, potentially set by helpers
	)

	// --- Pre-Update Checks and Component Updates ---

	// --- Pre-Update Checks and Component Updates ---

	// Check if the root context is done (timeout or cancelled)
	if m.rootCtx != nil && m.rootCtx.Err() != nil {
		log.Printf("Root context closed: %v", m.rootCtx.Err())
		if m.currentState != AppStateQuitting {
			m.currentState = AppStateQuitting // Ensure state reflects quitting
		}
		// Let the quit check below handle tea.Quit
	}

	// Update standard components (textarea, viewport, spinner) before handling specific messages
	// Note: KeyMsg handling might update textarea again, which is fine.
	m.textarea, taCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	// Update spinner only if in a state that requires it
	//if m.currentState == AppStateConnecting || m.currentState == AppStateSending || m.currentState == AppStateReceiving || m.currentState == AppStateProcessingTool {
	var spinnerText string
	// switch m.currentState {
	// case AppStateConnecting:
	// 	spinnerText = " Connecting..."
	// case AppStateSending:
	// 	spinnerText = " Sending..."
	// case AppStateReceiving:
	// 	spinnerText = " Receiving..."
	// case AppStateProcessingTool:
	// 	spinnerText = " Processing tool..."

	// }
	_ = spinnerText // TODO: Use this in the spinner update
	// TODO: wrap spinner to wrap with text
	// Update spinner text if needed (spinner doesn't support dynamic text directly)
	// We'll handle the text in the View function based on state.
	m.spinner, spCmd = m.spinner.Update(msg)
	cmds = append(cmds, spCmd)
	cmds = append(cmds, taCmd, vpCmd) // Add textarea and viewport commands

	// --- Delegate to Message Handlers ---
	// Only process messages if not already quitting
	if m.currentState != AppStateQuitting {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			// Check for Ctrl+P/Ctrl+R which might set startPlaybackTicker
			if msg.String() == "ctrl+p" || msg.String() == "ctrl+r" {
				// Peek ahead to see if playback was triggered
				_, triggered := m.checkPlayback(msg.String())
				if triggered {
					startPlaybackTicker = true
				}
			}
			_, helperCmd = m.handleKeyMsg(msg)

		// --- Stream Messages ---
		case initStreamMsg, streamReadyMsg, bidiStreamReadyMsg, streamResponseMsg, bidiStreamResponseMsg, sentMsg, sendErrorMsg, streamErrorMsg, streamClosedMsg:
			// Check streamResponseMsg for audio which might set startPlaybackTicker
			if respMsg, ok := msg.(streamResponseMsg); ok {
				// Check if audio data is present by checking the length of the Audio slice
				if len(respMsg.output.Audio) > 0 && m.enableAudio && m.playerCmd != "" {
					startPlaybackTicker = true
				}
			}
			_, helperCmd = m.handleStreamMsg(msg)

		// --- Tool Call Messages ---
		case toolCallSentMsg, toolCallResultMsg:
			_, helperCmd = m.handleToolMsg(msg)

		// --- History Messages ---
		case historyLoadedMsg, historyLoadFailedMsg, historySavedMsg, historySaveFailedMsg:
			_, helperCmd = m.handleHistoryMsg(msg)

		// --- Audio Messages ---
		case audioPlaybackStartedMsg, audioPlaybackCompletedMsg, audioQueueUpdatedMsg, audioPlaybackErrorMsg, flushAudioBufferMsg, playbackTickMsg:
			// Check audioPlaybackStartedMsg which might set startPlaybackTicker
			if _, ok := msg.(audioPlaybackStartedMsg); ok {
				startPlaybackTicker = true
			}
			_, helperCmd = m.handleAudioMsg(msg)

		// --- Other System Messages ---
		case spinner.TickMsg, tea.WindowSizeMsg:
			_, helperCmd = m.handleSystemMsg(msg)

		default:
			// Ignore unknown messages or messages handled by component updates above
		}
	}

	// Append command from the helper function
	if helperCmd != nil {
		cmds = append(cmds, helperCmd)
	}

	// --- Post-Update Logic ---

	// Start ticker command if flagged by any handler and not already running
	if startPlaybackTicker && !m.tickerRunning {
		m.tickerRunning = true
		log.Println("[UI] Starting playback ticker.")
		cmds = append(cmds, playbackTickCmd())
	}

	// Always ensure textarea keeps focus if it's the active component
	// This is critical for ensuring the input prompt remains visible
	if m.focusedComponent == "input" {
		m.textarea.Focus()
	}

	// Important: Always add the listener command back to the batch
	// to keep listening for background updates, unless quitting.
	if m.currentState != AppStateQuitting {
		cmds = append(cmds, m.listenForUIUpdatesCmd())
	}

	// Check for quit state *after* handling messages and adding commands
	if m.currentState == AppStateQuitting {
		log.Println("Quitting state detected, returning tea.Quit")
		// Perform final cleanup before quitting? Or rely on defer in main?
		// m.Cleanup() // Maybe call cleanup here?
		return m, tea.Quit
	}

	return m, tea.Batch(cmds...)
}

// Cleanup properly releases all resources
func (m *Model) Cleanup() {
	log.Println("Cleaning up resources")

	// Cancel root context if it exists
	if m.rootCtxCancel != nil {
		log.Println("Canceling root context")
		m.rootCtxCancel()
		m.rootCtxCancel = nil
	}

	// Close streams and contexts
	if m.stream != nil {
		m.stream.CloseSend()
		m.stream = nil
	}
	if m.bidiStream != nil {
		m.bidiStream.CloseSend()
		m.bidiStream = nil
	}
	if m.streamCtxCancel != nil {
		m.streamCtxCancel()
		m.streamCtxCancel = nil
	}

	// Close the audio channel and clear the queue
	if m.audioChannel != nil {
		log.Println("Closing audio processing channel")
		close(m.audioChannel) // Signal processor goroutine to exit
		m.audioChannel = nil
		m.audioQueue = nil
		m.currentAudio = nil
		m.isAudioProcessing = false
	}

	// Close the UI update channel
	if m.uiUpdateChan != nil {
		// It's generally safer *not* to close channels that multiple goroutines
		// might write to, unless you have robust synchronization ensuring no
		// writes happen after the close. Let program exit handle cleanup.
		// close(m.uiUpdateChan)
		m.uiUpdateChan = nil // Allow GC
	}

	// Clean up audio consolidation resources
	if m.bufferTimer != nil {
		log.Println("Stopping audio buffer timer")
		m.bufferTimer.Stop()
		m.bufferTimer = nil
	}
	m.consolidatedAudioData = nil
	m.bufferMessageIdx = -1
	m.bufferStartTime = time.Time{}
	m.lastFlushTime = time.Time{}
	m.currentBufferWindow = initialAudioBufferingWindow
	m.recentChunkSizes = nil
	m.recentChunkTimes = nil
	m.consecutiveSmallChunks = 0

	// Close the API client
	if m.client != nil && m.client.GenerativeClient != nil {
		m.client.Close()
	}

	// Save any pending history changes
	if m.historyEnabled && m.historyManager != nil && m.historyManager.CurrentSession != nil {
		if err := m.historyManager.SaveSession(m.historyManager.CurrentSession); err != nil {
			log.Printf("Error saving history during cleanup: %v", err)
		}
	}

	log.Println("Cleanup finished.")
}
