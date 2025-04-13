package aistudio

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tmc/aistudio/api"
	"github.com/tmc/aistudio/audioplayer"
	"github.com/tmc/aistudio/internal/helpers"
	"github.com/tmc/aistudio/settings"
)

// New creates a new Model instance with default settings and applies options.
func New(opts ...Option) *Model {
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
	}

	for _, opt := range opts {
		if err := opt(m); err != nil {
			log.Printf("Warning: Error applying option: %v", err)
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

	return m, nil
}

// initCmd returns the command sequence to start the application logic.
func (m *Model) initCmd() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
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

// Init is the initial command called by Bubble Tea.
func (m Model) Init() tea.Cmd {
	// Ensure the textarea has focus from the very beginning
	// This is critical for the input prompt to be visible
	m.textarea.Focus()

	cmds := []tea.Cmd{
		m.spinner.Tick,
		m.initCmd(),               // Starts stream connection attempt
		m.listenForUIUpdatesCmd(), // Starts listening for background messages
	}

	// Start the audio processor goroutine if audio is enabled
	if m.enableAudio && m.audioChannel != nil {
		m.startAudioProcessor() // Defined in audio_player.go
	}

	return tea.Batch(cmds...)
}

// Update handles incoming messages and updates the model state.
// It acts as the main dispatcher.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		taCmd               tea.Cmd
		vpCmd               tea.Cmd
		spCmd               tea.Cmd
		cmds                []tea.Cmd
		sendCmd             tea.Cmd
		startPlaybackTicker bool // Flag to start the ticker
	)

	// Update standard components
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Key messages processed below
	default:
		m.textarea, taCmd = m.textarea.Update(msg)
		m.viewport, vpCmd = m.viewport.Update(msg)
	}
	m.spinner, spCmd = m.spinner.Update(msg)
	cmds = append(cmds, taCmd, vpCmd, spCmd)

	// --- Process specific message types ---
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
		} else {
			// Otherwise handle key in main UI
			m.textarea, taCmd = m.textarea.Update(msg) // Update textarea here
			cmds = append(cmds, taCmd)
		}

		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			if m.stream != nil {
				cmds = append(cmds, m.closeStreamCmd())
			}
			if m.bidiStream != nil {
				cmds = append(cmds, m.closeBidiStreamCmd())
			}
			return m, tea.Quit

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
				startPlaybackTicker = true // Signal to start ticker if not running
				cmds = append(cmds, playCmd)
			}
			// Return with potential play command and component updates
			return m, tea.Batch(cmds...)

		case "ctrl+r": // Replay last played audio
			replayCmd, triggered := m.ReplayLastAudio() // Using extracted function from audio_controls.go
			if triggered {
				startPlaybackTicker = true // Signal to start ticker if not running
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
			if txt := strings.TrimSpace(m.textarea.Value()); txt != "" && m.streamReady {
				m.messages = append(m.messages, formatMessage("You", txt)) // helpers.go
				m.viewport.SetContent(m.formatAllMessages())               // ui.go
				m.viewport.GotoBottom()
				m.textarea.Reset()
				m.textarea.Focus()
				m.sending = true
				cmds = append(cmds, m.spinner.Tick)

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

			} else if !m.streamReady {
				err := fmt.Errorf("cannot send message: stream not ready")
				m.messages = append(m.messages, formatError(err)) // helpers.go
				m.viewport.SetContent(m.formatAllMessages())
				m.viewport.GotoBottom()
				m.err = err // Keep error state
			}
			return m, tea.Batch(cmds...)
		}

	// --- Stream Messages ---
	case initStreamMsg: // stream.go
		m.messages = append(m.messages, formatMessage("System", "Connecting to Gemini..."))
		m.viewport.SetContent(m.formatAllMessages())
		m.viewport.GotoBottom()
		cmds = append(cmds, m.initStreamCmd()) // Start connection

	case streamReadyMsg: // stream.go
		m.stream = msg.stream
		m.bidiStream = nil
		m.streamReady = true
		m.receiving = true
		m.err = nil
		if len(m.messages) > 0 && strings.Contains(m.messages[len(m.messages)-1].Content, "Connecting") {
			m.messages = m.messages[:len(m.messages)-1]
		}
		m.messages = append(m.messages, formatMessage("System", "Connected. You can start chatting."))
		m.viewport.SetContent(m.formatAllMessages())
		m.viewport.GotoBottom()
		cmds = append(cmds, m.receiveStreamCmd()) // Start receiving

	case bidiStreamReadyMsg: // stream.go
		m.bidiStream = msg.stream
		m.stream = nil
		m.streamReady = true
		m.receiving = true
		m.err = nil
		if len(m.messages) > 0 && strings.Contains(m.messages[len(m.messages)-1].Content, "Connecting") {
			m.messages = m.messages[:len(m.messages)-1]
		}
		m.messages = append(m.messages, formatMessage("System", "Connected with bidirectional stream. You can start chatting."))
		m.viewport.SetContent(m.formatAllMessages())
		m.viewport.GotoBottom()
		cmds = append(cmds, m.receiveBidiStreamCmd()) // Start receiving

	case streamResponseMsg: // stream.go (One-way stream response)
		m.receiving = true
		if msg.output.Text != "" || len(msg.output.Audio) > 0 {
			newMessage := Message{
				Sender:    "Gemini",
				Content:   msg.output.Text,
				HasAudio:  len(msg.output.Audio) > 0,
				AudioData: msg.output.Audio, // Store complete audio
				Timestamp: time.Now(),
			}
			m.messages = append(m.messages, newMessage)
			idx := len(m.messages) - 1

			// Save message to history if enabled
			if m.historyEnabled && m.historyManager != nil && msg.output.Text != "" {
				m.historyManager.AddMessage(newMessage)
				// Auto-save periodically
				cmds = append(cmds, m.saveSessionCmd())
			}

			if newMessage.HasAudio && m.enableAudio && m.playerCmd != "" {
				log.Printf("[UI] Triggering playback for stream message #%d", idx)
				cmds = append(cmds, m.playAudioCmd(newMessage.AudioData, newMessage.Content))
				m.messages[idx].IsPlaying = true // Optimistic UI
				startPlaybackTicker = true
			}
			m.viewport.SetContent(m.formatAllMessages())
			m.viewport.GotoBottom()
		}
		if m.stream != nil && !m.quitting {
			cmds = append(cmds, m.receiveStreamCmd()) // Continue receiving
		} else {
			m.receiving = false
		}

	case bidiStreamResponseMsg: // stream.go (Bidirectional stream response)
		m.receiving = true

		// Check for tool calls in the response if tool support is enabled
		if m.enableTools && !m.processingTool {
			toolCalls := ExtractToolCalls(&msg.output)
			if len(toolCalls) > 0 {
				// Process the tool calls
				m.processingTool = true
				results, err := m.processToolCalls(toolCalls)
				if err != nil {
					// Handle error
					log.Printf("Error processing tool calls: %v", err)
					m.messages = append(m.messages, formatError(fmt.Errorf("tool call error: %w", err)))
				} else if len(results) > 0 {
					// Send tool results back to model
					cmds = append(cmds, m.sendToolResultsCmd(results))
				}
				m.processingTool = false
			}
		}

		if msg.output.Text != "" || len(msg.output.Audio) > 0 {
			// Skip tool call formatting text if it contains tool call markers
			if m.enableTools && strings.Contains(msg.output.Text, "[TOOL_CALL]") {
				// Tool call text shouldn't be displayed directly
				// We'll let the tool handling code create more helpful messages
			} else {
				var targetMsgIdx int = -1
				createNeeded := true
				if len(m.messages) > 0 {
					lastIdx := len(m.messages) - 1
					if m.messages[lastIdx].Sender == "Gemini" && time.Since(m.messages[lastIdx].Timestamp) < 3*time.Second {
						targetMsgIdx = lastIdx
						createNeeded = false
					}
				}

				if createNeeded {
					newMessage := formatMessage("Gemini", msg.output.Text)
					newMessage.Timestamp = time.Now()
					m.messages = append(m.messages, newMessage)
					targetMsgIdx = len(m.messages) - 1

					// Save message to history if enabled
					if m.historyEnabled && m.historyManager != nil && msg.output.Text != "" {
						m.historyManager.AddMessage(newMessage)
						// Auto-save periodically
						cmds = append(cmds, m.saveSessionCmd())
					}

					if helpers.IsAudioTraceEnabled() {
						log.Printf("[AUDIO_PIPE] Created new message #%d for incoming bidi stream data", targetMsgIdx)
					}
				} else {
					if msg.output.Text != "" {
						m.messages[targetMsgIdx].Content += msg.output.Text
						m.messages[targetMsgIdx].Timestamp = time.Now()

						// Update message in history
						if m.historyEnabled && m.historyManager != nil {
							if m.historyManager.CurrentSession != nil && targetMsgIdx < len(m.historyManager.CurrentSession.Messages) {
								m.historyManager.CurrentSession.Messages[targetMsgIdx] = m.messages[targetMsgIdx]
							}
						}
					}
					if helpers.IsAudioTraceEnabled() {
						log.Printf("[AUDIO_PIPE] Appending to existing message #%d for incoming bidi stream data", targetMsgIdx)
					}
				}

				if len(msg.output.Audio) > 0 && m.enableAudio && m.playerCmd != "" && targetMsgIdx >= 0 {
					consolidateCmd := m.consolidateAndPlayAudio(msg.output.Audio, m.messages[targetMsgIdx].Content, targetMsgIdx) // audio_manager.go
					cmds = append(cmds, consolidateCmd)
					// Don't set IsPlaying here; let audioPlaybackStartedMsg handle it
				}

				m.viewport.SetContent(m.formatAllMessages())
				m.viewport.GotoBottom()
			}
		}

		if m.bidiStream != nil && !m.quitting {
			cmds = append(cmds, m.receiveBidiStreamCmd()) // Continue receiving
		} else {
			m.receiving = false
		}

	case sentMsg: // stream.go
		m.sending = false

	case sendErrorMsg: // stream.go
		m.sending = false
		m.err = fmt.Errorf("send error: %w", msg.err)
		m.messages = append(m.messages, formatError(m.err))
		m.viewport.SetContent(m.formatAllMessages())
		m.viewport.GotoBottom()

	case streamErrorMsg: // stream.go
		m.receiving = false
		m.streamReady = false
		m.err = fmt.Errorf("stream error: %w", msg.err)
		m.messages = append(m.messages, formatError(m.err))
		m.viewport.SetContent(m.formatAllMessages())
		m.viewport.GotoBottom()
		m.stream = nil
		m.bidiStream = nil
		if m.streamCtxCancel != nil {
			m.streamCtxCancel()
			m.streamCtxCancel = nil
		}

	case streamClosedMsg: // stream.go
		m.receiving = false
		m.streamReady = false
		if !m.quitting {
			m.messages = append(m.messages, formatMessage("System", "Stream closed."))
			m.viewport.SetContent(m.formatAllMessages())
			m.viewport.GotoBottom()
		}
		m.stream = nil
		m.bidiStream = nil
		if m.streamCtxCancel != nil {
			m.streamCtxCancel()
			m.streamCtxCancel = nil
		}

	// --- Tool Call Messages ---
	case toolCallSentMsg:
		// Tool call results sent successfully, nothing to do

	case toolCallResultMsg:
		// Results from tool calls, might be displayed or logged
		log.Printf("Received %d tool call results", len(msg.results))

	// --- History Messages ---
	case historyLoadedMsg:
		if msg.session != nil {
			m.loadMessagesFromSession(msg.session)
			m.messages = append(m.messages, formatMessage("System", fmt.Sprintf("Loaded chat session: %s", msg.session.Title)))
			m.viewport.SetContent(m.formatAllMessages())
			m.viewport.GotoBottom()
		}

	case historyLoadFailedMsg:
		m.messages = append(m.messages, formatError(fmt.Errorf("failed to load history: %w", msg.err)))
		m.viewport.SetContent(m.formatAllMessages())
		m.viewport.GotoBottom()

	case historySavedMsg:
		// History saved successfully, could show a notification
		log.Println("Chat history saved successfully")

	case historySaveFailedMsg:
		m.messages = append(m.messages, formatError(fmt.Errorf("failed to save history: %w", msg.err)))
		m.viewport.SetContent(m.formatAllMessages())
		m.viewport.GotoBottom()

	// --- Audio Messages ---
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
		m.viewport.SetContent(m.formatAllMessages()) // Update UI

		// Make sure textarea maintains focus when audio starts playing
		if m.focusedComponent == "input" {
			m.textarea.Focus()
		}

		startPlaybackTicker = true // Signal to start ticker

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
		m.viewport.SetContent(m.formatAllMessages()) // Update UI

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
		m.viewport.SetContent(m.formatAllMessages())
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
			m.viewport.SetContent(m.formatAllMessages()) // Refresh view for progress bar

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

	// --- Other System Messages ---
	case spinner.TickMsg:
		// Handled by spinner update earlier

	case tea.WindowSizeMsg:
		// Prevent zero dimensions
		m.width = max(msg.Width, 20)
		m.height = max(msg.Height, 10)

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

		// Size viewport proportionally
		m.viewport.Width = m.width
		m.viewport.Height = vpHeight

		// Always ensure textarea is visible with proper width
		m.textarea.SetWidth(m.width)
		m.textarea.SetHeight(1) // One line is enough

		// Update content
		m.viewport.SetContent(m.formatAllMessages()) // Refresh content
		m.viewport.GotoBottom()

	default:
		// Ignore unknown messages
	}

	// Start ticker command if flagged and not already running
	if startPlaybackTicker && !m.tickerRunning {
		m.tickerRunning = true
		log.Println("[UI] Starting playback ticker.")
		cmds = append(cmds, playbackTickCmd())
	}

	// Always ensure textarea keeps focus, even during audio playback
	// This is critical for ensuring the input prompt remains visible
	m.textarea.Focus()

	// Important: Always add the listener command back to the batch
	cmds = append(cmds, m.listenForUIUpdatesCmd())

	return m, tea.Batch(cmds...)
}

// View renders the UI.
func (m Model) View() string {
	if m.quitting {
		return "Closing stream and quitting...\n"
	}

	// Set default dimensions if needed
	if m.width == 0 || m.height == 0 {
		// Set default dimensions for better initial rendering
		m.width = 80
		m.height = 24
	}

	// Always ensure textarea is visible with proper width - this is critical!
	m.textarea.SetWidth(m.width)
	m.textarea.SetHeight(1) // One line is enough

	// Ensure the textarea has focus when in input mode
	// This is necessary for the input prompt to be visible
	if m.focusedComponent == "input" {
		m.textarea.Focus()
	}

	// Give viewport some space
	headerHeight := 2 // Minimal header height
	footerHeight := 2 // Minimal footer (input + status)
	vpHeight := m.height - headerHeight - footerHeight
	m.viewport.Width = m.width
	m.viewport.Height = max(10, vpHeight) // At least 5 lines for messages

	// Apply rounded borders to viewport
	m.viewport.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	// Main content view (without settings panel)
	var mainContent strings.Builder
	mainContent.WriteString(m.headerView()) // In ui.go
	mainContent.WriteString(m.viewport.View())
	mainContent.WriteString(m.footerView()) // In ui.go

	// If settings panel is visible, join horizontally
	if m.showSettingsPanel && m.settingsPanel != nil {
		// Calculate content width for settings panel (1/4 of screen instead of 1/3)
		settingsPanelWidth := m.width / 4 // Reduce settings panel width to leave more for main content

		// Calculate main content width
		mainContentWidth := m.width - settingsPanelWidth - 1 // -1 for padding space

		// Resize textarea for the main content area
		m.textarea.SetWidth(mainContentWidth)

		// Get settings panel view
		settingsView := m.settingsPanel.View()

		// Make sure we're using a safe width for the main content
		settingsPanelWidth = min(settingsPanelWidth, m.width/4) // Keep limit consistent with calculation above

		// Apply styling to settings panel only
		settingsStyled := lipgloss.NewStyle().
			Width(settingsPanelWidth).
			Height(m.height).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			Render(settingsView)

		// Prepare a new builder for the combined view
		var combined strings.Builder

		// Split the main content by lines
		mainLines := strings.Split(mainContent.String(), "\n")
		settingsLines := strings.Split(settingsStyled, "\n")

		// Use the longer of the two for iteration
		maxLines := max(len(mainLines), len(settingsLines))

		// Pad the settings lines array if needed to avoid index out of bounds
		for len(settingsLines) < maxLines {
			settingsLines = append(settingsLines, "")
		}

		// Pad the main lines array if needed to avoid index out of bounds
		for len(mainLines) < maxLines {
			mainLines = append(mainLines, "")
		}

		// Combine them side by side
		for i := 0; i < maxLines; i++ {
			// Get settings line
			settingsLine := settingsLines[i]

			// Get main content line
			mainLine := mainLines[i]

			// Padding between panels
			padding := " "

			// Ensure settings line is full width to maintain alignment
			settingsLine = lipgloss.NewStyle().Width(settingsPanelWidth).Render(settingsLine)

			// Combine the lines
			combined.WriteString(settingsLine)
			combined.WriteString(padding)
			combined.WriteString(mainLine)
			combined.WriteString("\n")
		}

		return combined.String()
	}

	// Return just the main content if settings panel is not visible
	return mainContent.String()
}

// Cleanup properly releases all resources
func (m *Model) Cleanup() {
	log.Println("Cleaning up resources")

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
	if m.client != nil && m.client.GenAI != nil {
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
