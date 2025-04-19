package aistudio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"cloud.google.com/go/ai/generativelanguage/apiv1alpha/generativelanguagepb"
	"github.com/charmbracelet/lipgloss"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

func (m *Model) formatMessageText(msg Message, messageIndex int) string {
	// Skip empty messages unless they have special formatting
	if msg.Content == "" &&
		!msg.HasAudio &&
		!msg.IsToolCall &&
		!msg.IsToolResponse &&
		msg.FunctionCall == nil &&
		!msg.IsExecutableCode &&
		!msg.IsExecutableCodeResult {
		return "" // Skip empty messages that don't have special formatting
	}

	var finalMsg strings.Builder

	// Add message header
	finalMsg.WriteString(m.formatMessageHeader(msg))

	switch {
	case msg.HasAudio:
		m.formatAudioMessage(&finalMsg, msg, messageIndex)
	case msg.IsToolCall:
		finalMsg.WriteString(msg.Content) // Pre-formatted content
	case msg.IsToolResponse:
		finalMsg.WriteString(msg.Content) // Pre-formatted content
	case msg.FunctionCall != nil:
		m.formatFunctionCallMessage(&finalMsg, msg)
	case msg.IsExecutableCode:
		// m.formatExecutableCodeMessage(&finalMsg, msg)
	case msg.IsExecutableCodeResult:
		// m.formatExecutableCodeResultMessage(&finalMsg, msg)
	default:
		m.formatDefaultMessage(&finalMsg, msg)
	}

	return finalMsg.String()
}

func (m *Model) formatMessageHeader(msg Message) string {
	var senderStyle lipgloss.Style
	switch msg.Sender {
	case senderNameUser:
		senderStyle = senderUserStyle
	case senderNameModel:
		senderStyle = senderModelStyle
	default:
		senderStyle = senderSystemStyle
	}
	return senderStyle.Render(string(msg.Sender) + ": ")
}

func (m *Model) formatAudioMessage(finalMsg *strings.Builder, msg Message, messageIndex int) {
	audioLine := m.formatAudioLine(msg, messageIndex)

	if msg.Sender == senderNameModel {
		// Inline audio with content for Gemini messages
		if msg.Content != "" {
			finalMsg.WriteString(msg.Content)
			finalMsg.WriteString(" ")
		}
		finalMsg.WriteString(strings.TrimSpace(audioLine))
	} else {
		// Separate audio and content for other senders
		if msg.Content != "" {
			finalMsg.WriteString(msg.Content)
		}
		finalMsg.WriteString(audioLine)
	}
}

func (m *Model) formatAudioLine(msg Message, messageIndex int) string {
	audioStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true) // Magenta for audio
	audioLine := audioStyle.Render(fmt.Sprintf("🔊 Audio: "))
	return audioLine
}

func (m *Model) formatFunctionCallMessage(finalMsg *strings.Builder, msg Message) {
	if msg.Content != "" {
		finalMsg.WriteString(msg.Content)
		finalMsg.WriteString("\n")
	}
	callStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	finalMsg.WriteString(callStyle.Render(fmt.Sprintf("📞 Function Call: %s", msg.FunctionCall.Name)))
	finalMsg.WriteString("\n")

	if msg.FunctionCall.Args != nil {
		argsBytes, err := json.MarshalIndent(msg.FunctionCall.Args, "", "  ")
		if err != nil {
			argsBytes = []byte(fmt.Sprintf("%v", msg.FunctionCall.Args))
		}
		finalMsg.WriteString("```json\n")
		finalMsg.WriteString(string(argsBytes))
		finalMsg.WriteString("\n```\n")
	}
}

func (m *Model) formatExecutableCodeMessage(finalMsg *strings.Builder, msg Message) {
	codeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	if msg.Content != "" {
		finalMsg.WriteString(msg.Content)
		finalMsg.WriteString("\n")
	}
	if msg.ExecutableCode != nil {
		finalMsg.WriteString(codeStyle.Render("📝 Code Execution:"))
		finalMsg.WriteString("\n")
		finalMsg.WriteString(fmt.Sprintf("```%s\n", msg.ExecutableCode.Language))
		finalMsg.WriteString(msg.ExecutableCode.Code)
		finalMsg.WriteString("\n```\n")
	}
}

func (m *Model) formatExecutableCodeResultMessage(finalMsg *strings.Builder, msg Message) {
	outputStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	if msg.Content != "" {
		finalMsg.WriteString(msg.Content)
		finalMsg.WriteString("\n")
	}
	if msg.ExecutableCodeResult != nil {
		finalMsg.WriteString(outputStyle.Render("✅ Code Result:"))
		finalMsg.WriteString("\n```\n")
		finalMsg.WriteString(fmt.Sprintf("%v", msg.ExecutableCodeResult))
		finalMsg.WriteString("\n```\n")
	}
}

func (m *Model) formatDefaultMessage(finalMsg *strings.Builder, msg Message) {
	if msg.Content != "" {
		finalMsg.WriteString(msg.Content)
	}
	// TODO:implement // m.formatSafetyRatings(finalMsg, msg)
	// TODO:implement // m.formatGroundingMetadata(finalMsg, msg)
	m.formatTokenCounts(finalMsg, msg)
}

func (m *Model) formatTokenCounts(finalMsg *strings.Builder, msg Message) {
	if msg.TokenCounts != nil {
		tokenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
		finalMsg.WriteString(tokenStyle.Render(fmt.Sprintf("Token Counts: %d (Input) / %d (Output)", msg.TokenCounts.PromptTokenCount, msg.TokenCounts.ResponseTokenCount)))
		finalMsg.WriteString("\n")
	}
}

// renderAllMessages formats all messages as a single string for display
// Groups tool calls with their results and adds borders around messages
func (m *Model) renderAllMessages() string {
	var formattedMessages []string
	var messageBuffer strings.Builder
	var pendingToolCall *Message

	// Get width for message borders
	availWidth := m.width - 4 // Account for borders and padding

	// Different border styles based on sender
	systemMessageStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")). // Gray for system messages
		Padding(0, 1).
		Width(availWidth)

	toolCallStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")). // Magenta for tool calls
		Padding(0, 1).
		Width(availWidth)

	// Helper to choose the right style for a message
	getBorderStyle := func(msg Message) lipgloss.Style {
		if msg.IsToolCall || msg.IsToolResponse {
			return toolCallStyle
		}
		return systemMessageStyle
	}
	// lipBorderForeground(getBorderStyle(msg))
	// Padding(0, 1)

	for i, msg := range m.messages {
		messageBorderStyle := getBorderStyle(msg)
		// Handle tool call pairing
		if pendingToolCall != nil && msg.IsToolResponse &&
			msg.ToolResponse != nil && pendingToolCall.ToolCall != nil &&
			msg.ToolResponse.Id == pendingToolCall.ToolCall.ID {
			// Add this tool response to the buffered tool call
			messageBuffer.WriteString(m.formatMessageText(msg, i))

			// Border and add the complete tool call + result
			if messageBuffer.Len() > 0 {
				formattedMessages = append(formattedMessages,
					messageBorderStyle.Render(messageBuffer.String()))
				messageBuffer.Reset()
			}
			pendingToolCall = nil
			continue
		}

		// Handle beginning of a new tool call
		if msg.IsToolCall && pendingToolCall == nil {
			// If there's anything in the buffer, add it first
			if messageBuffer.Len() > 0 {
				formattedMessages = append(formattedMessages,
					messageBorderStyle.Render(messageBuffer.String()))
				messageBuffer.Reset()
			}

			// Start a new buffer with this tool call
			messageBuffer.WriteString(m.formatMessageText(msg, i))
			pendingToolCall = &msg
			continue
		}

		// For non-tool messages or unpaired tool calls/responses
		if pendingToolCall != nil {
			// Add the pending tool call as it wasn't paired
			formattedMessages = append(formattedMessages,
				messageBorderStyle.Render(messageBuffer.String()))
			messageBuffer.Reset()
			pendingToolCall = nil
		}

		// Format and add the current message
		formatted := m.formatMessageText(msg, i)
		if formatted != "" {
			formattedMessages = append(formattedMessages,
				messageBorderStyle.Render(formatted))
		}
	}
	defaultBorderStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")). // Gray for system messages
		Padding(0, 1).
		Width(availWidth)

	// Add any remaining buffered content
	if messageBuffer.Len() > 0 {
		formattedMessages = append(formattedMessages,
			defaultBorderStyle.Render(messageBuffer.String()))
	}

	// Join with spacing between bordered messages
	return strings.Join(formattedMessages, "\n")
}

// renderToolApprovalModal renders a modal for approving a tool call
func (m *Model) renderToolApprovalModal() string {
	if !m.showToolApproval || len(m.pendingToolCalls) == 0 || m.approvalIndex >= len(m.pendingToolCalls) {
		return ""
	}

	// Get the current tool call being approved
	toolCall := m.pendingToolCalls[m.approvalIndex]

	// Create styles for the modal
	boxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(1, 3).
		Width(m.width - 10).
		Align(lipgloss.Center)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("63")).
		MarginBottom(1)

	toolNameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)

	buttonStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("63")).
		Padding(0, 2).
		MarginRight(2)

	buttonDangerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("160")).
		Padding(0, 2).
		MarginRight(2)

	// Format arguments for display
	var argsBuf bytes.Buffer
	if err := json.Indent(&argsBuf, toolCall.Arguments, "", "  "); err != nil {
		argsBuf.Write(toolCall.Arguments)
	}

	// Build the modal content
	var builder strings.Builder
	builder.WriteString(titleStyle.Render("Tool Call Approval Required"))
	builder.WriteString("\n\n")
	builder.WriteString(fmt.Sprintf("Tool: %s", toolNameStyle.Render(toolCall.Name)))
	builder.WriteString("\n\n")
	builder.WriteString("Arguments:\n")
	builder.WriteString("```json\n")
	builder.WriteString(argsBuf.String())
	builder.WriteString("\n```\n\n")

	// Add progress indicator if there are multiple pending tool calls
	if len(m.pendingToolCalls) > 1 {
		builder.WriteString(fmt.Sprintf("Tool call %d of %d\n\n", m.approvalIndex+1, len(m.pendingToolCalls)))
	}

	// Add action buttons
	builder.WriteString(buttonStyle.Render("Y: Approve"))
	builder.WriteString(" ")
	builder.WriteString(buttonDangerStyle.Render("N: Deny"))
	if len(m.pendingToolCalls) > 1 && m.approvalIndex < len(m.pendingToolCalls)-1 {
		builder.WriteString(" ")
		builder.WriteString(buttonStyle.Render("Tab: Next"))
	}

	return boxStyle.Render(builder.String())
}

// convertSafetyRating converts API safety rating to our display format
func convertSafetyRating(apiRating *generativelanguagepb.SafetyRating) *SafetyRating {
	if apiRating == nil {
		return nil
	}

	// Convert category to a simplified string representation
	category := fmt.Sprintf("%s", apiRating.Category)
	// Remove the prefix if present
	category = strings.TrimPrefix(category, "HARM_CATEGORY_")

	// Convert probability to string representation
	probability := fmt.Sprintf("%s", apiRating.Probability)

	// Check if this rating is blocking content - high and medium are generally considered blocking
	blocked := apiRating.Probability == generativelanguagepb.SafetyRating_HIGH ||
		apiRating.Probability == generativelanguagepb.SafetyRating_MEDIUM

	return &SafetyRating{
		Category:    category,
		Probability: probability,
		Score:       0, // API doesn't provide probability score directly
		Blocked:     blocked,
	}
}

// convertGroundingMetadata converts API grounding metadata to our display format
func (m *Model) convertGroundingMetadata(apiMetadata *generativelanguagepb.GroundingMetadata) *GroundingMetadata {
	if apiMetadata == nil {
		return nil
	}

	result := &GroundingMetadata{
		Chunks:              make([]*GroundingChunk, 0),
		Supports:            make([]*GroundingSupport, 0),
		WebSearchQueries:    make([]string, 0),
		HasSearchEntryPoint: false,
	}

	// Convert grounding chunks
	for i, chunk := range apiMetadata.GetGroundingChunks() {
		displayChunk := &GroundingChunk{
			ID:       fmt.Sprintf("chunk-%d", i),
			Selected: false,
		}

		// Handle web chunk
		if web := chunk.GetWeb(); web != nil {
			displayChunk.IsWeb = true
			displayChunk.URI = web.GetUri()
			displayChunk.Title = web.GetTitle()
		}

		result.Chunks = append(result.Chunks, displayChunk)
	}

	// Convert web search queries
	result.WebSearchQueries = apiMetadata.GetWebSearchQueries()

	// Handle search entry point
	if entryPoint := apiMetadata.GetSearchEntryPoint(); entryPoint != nil {
		result.HasSearchEntryPoint = true
		result.SearchEntryPoint = &SearchEntryPoint{
			RenderedContent: entryPoint.GetRenderedContent(),
			HasSDKBlob:      len(entryPoint.GetSdkBlob()) > 0,
		}
	}

	// Handle support information
	for _, support := range apiMetadata.GetGroundingSupports() {
		// Convert []int32 to []int for ChunkIndices
		int32Indices := support.GetGroundingChunkIndices()
		intIndices := make([]int, len(int32Indices))
		for i, idx := range int32Indices {
			intIndices[i] = int(idx)
		}

		displaySupport := &GroundingSupport{
			Text:         "", // Will be extracted from segment information
			ChunkIndices: intIndices,
			Confidence:   support.GetConfidenceScores(),
		}

		// Extract text from segment if available
		if seg := support.GetSegment(); seg != nil {
			displaySupport.Text = seg.GetText()
		}

		// Associate chunks with this support
		displaySupport.ChunksSelected = make([]*GroundingChunk, 0)
		for _, idx := range displaySupport.ChunkIndices {
			if idx >= 0 && int(idx) < len(result.Chunks) {
				// Mark the chunk as selected
				result.Chunks[idx].Selected = true
				displaySupport.ChunksSelected = append(displaySupport.ChunksSelected, result.Chunks[idx])
			}
		}

		result.Supports = append(result.Supports, displaySupport)
	}

	return result
}

// convertFunctionCall converts API function call to our display format
func convertFunctionCall(apiCall *generativelanguagepb.FunctionCall) *FormattedFunctionCall {
	if apiCall == nil {
		return nil
	}

	// Parse arguments to structured data
	var argsMap interface{}
	argsStr := ""

	if apiCall.Args != nil {
		// Try to marshal the args to a formatted JSON string
		if argsBytes, err := json.MarshalIndent(apiCall.Args, "", "  "); err == nil {
			argsStr = string(argsBytes)
			// Also unmarshal to a map for structured access
			json.Unmarshal(argsBytes, &argsMap)
		} else {
			// Fallback for display
			argsStr = fmt.Sprintf("%v", apiCall.Args)
		}
	}

	return &FormattedFunctionCall{
		Name:         apiCall.Name,
		Arguments:    argsStr,
		ArgumentsMap: argsMap,
		Raw:          apiCall,
	}
}

// headerView renders the header for the UI
func (m Model) headerView() string {
	var header strings.Builder

	// Add logo if enabled
	if m.showLogo {
		logoLines := strings.Split("aistudio", "\n") // constants.go
		for _, line := range logoLines {
			if line != "" {
				padding := (m.width - lipgloss.Width(line)) / 2 // Use lipgloss.Width for accurate calculation
				if padding < 0 {
					padding = 0
				}
				paddedLine := strings.Repeat(" ", padding) + line
				header.WriteString(logoStyle.Render(lipgloss.NewStyle().MaxWidth(m.width).Render(paddedLine)) + "\n")
			}
		}
	}

	// Add the title
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	modelInfo := m.modelName
	if m.enableAudio {
		voice := m.voiceName
		if voice == "" {
			voice = "Default"
		}
		modelInfo += fmt.Sprintf(" (Audio: %s)", voice)
	}

	// Add mode indicator
	if m.useBidi {
		modelInfo += " [BidiStream]"
	} else {
		modelInfo += " [Stream]"
	}

	// Add history and tool indicators
	if m.historyEnabled {
		modelInfo += " [History]"
	}
	if m.enableTools {
		modelInfo += " [Tools]"
	}

	// Render title centered within available width
	header.WriteString(titleStyle.Width(m.width).Align(lipgloss.Center).Render("Gemini Live Chat - " + modelInfo))
	header.WriteString("\n") // Add newline after title

	return header.String()
}

// logMessagesView renders the log messages box
func (m Model) logMessagesView() string {
	if !m.showLogMessages || len(m.logMessages) == 0 {
		return ""
	}

	// Create a bordered box for log messages
	logBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")). // Gray border
		Padding(0, 1).                         // Padding inside the border
		Width(m.width - 2)                     // Account for border width

	// Header for the log box
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("8")). // Gray text
		Render("Recent Log Messages")

	// Format each log message
	var logContent strings.Builder
	logContent.WriteString(header + "\n")

	// Calculate available width for log messages inside padding
	innerWidth := m.width - 4 // 2 for border, 2 for padding

	for i, logMsg := range m.logMessages {
		// Format the log message with a prefix
		prefix := fmt.Sprintf("[%d] ", i+1)
		maxMsgWidth := innerWidth - lipgloss.Width(prefix)
		if maxMsgWidth < 1 {
			maxMsgWidth = 1
		}
		// Render message, potentially truncating
		renderedMsg := logMessageStyle.MaxWidth(maxMsgWidth).Render(logMsg)
		logContent.WriteString(prefix + renderedMsg + "\n")
	}

	// Render the box around the content
	return logBoxStyle.Render(logContent.String())
}

// audioStatusView is now just a stub function that always returns an empty string
// Audio status will be shown only in the status line using the spinner
func (m Model) audioStatusView() string {
	// Always return empty string - we no longer show the audio status box
	return ""
}

// footerView renders the simplified footer for the UI
func (m Model) footerView() string {
	var footer strings.Builder

	// Define base styles
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	toolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // Red for errors
	inputModeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("213"))

	// Build the status line
	var statusLine strings.Builder

	// Add input mode indicator if active
	if m.micActive {
		statusLine.WriteString(inputModeStyle.Render("[Mic ON] "))
	} else if m.videoInputMode != VideoInputNone {
		statusLine.WriteString(inputModeStyle.Render(fmt.Sprintf("[%s ON] ", m.videoInputMode)))
	}

	// Show current state with a nice spinner when waiting
	switch m.currentState {
	case AppStateWaiting:
		if m.isAudioProcessing && m.enableAudio {
			statusLine.WriteString(spinnerStyle.Render(m.spinner.View()) + " " + statusStyle.Render("Playing audio..."))
		} else if m.processingTool || len(m.pendingToolCalls) > 0 {
			statusLine.WriteString(spinnerStyle.Render(m.spinner.View()) + " " + toolStyle.Render("Processing tool..."))
		} else {
			statusLine.WriteString(spinnerStyle.Render(m.spinner.View()) + " " + statusStyle.Render("Waiting for response..."))
		}
	case AppStateInitializing:
		statusLine.WriteString(spinnerStyle.Render(m.spinner.View()) + " " + statusStyle.Render("Initializing..."))
	case AppStateReady:
		if m.isAudioProcessing && m.enableAudio {
			statusLine.WriteString(spinnerStyle.Render(m.spinner.View()) + " " + statusStyle.Render("Playing audio..."))
		} else {
			statusLine.WriteString(statusStyle.Render("Ready"))
		}
	case AppStateError:
		if m.err != nil {
			errStr := fmt.Sprintf("Error: %v", m.err)
			statusLine.WriteString(errorStyle.Render(errStr))
		} else {
			statusLine.WriteString(errorStyle.Render("Error"))
		}
	default:
		statusLine.WriteString(statusStyle.Render(string(m.currentState)))
	}

	// For help text
	var helpParts []string
	helpParts = append(helpParts, "Enter: Send")
	if m.micActive || m.enableAudio {
		helpParts = append(helpParts, "Ctrl+M: Mic")
	}
	if m.historyEnabled {
		helpParts = append(helpParts, "Ctrl+H: Save History")
	}
	if m.enableTools {
		helpParts = append(helpParts, "Ctrl+T: Tools")
	}
	if m.enableAudio {
		helpParts = append(helpParts, "Ctrl+P: Play/Pause")
		helpParts = append(helpParts, "Ctrl+R: Replay")
	}
	helpParts = append(helpParts, "Tab: Navigate")
	helpParts = append(helpParts, "Ctrl+C: Quit")

	help := statusStyle.Render(strings.Join(helpParts, " | "))

	// Show model and tool info at the top of footer
	var infoLine string
	if m.enableTools && m.toolManager != nil {
		toolCount := len(m.toolManager.RegisteredToolDefs)
		infoLine = statusStyle.Render(fmt.Sprintf("Model: %s | Tools: %d available", m.modelName, toolCount))
	} else {
		infoLine = statusStyle.Render(fmt.Sprintf("Model: %s", m.modelName))
	}

	// Combine into a nice footer
	footer.WriteString(infoLine)
	footer.WriteString("\n")
	footer.WriteString(statusLine.String())
	footer.WriteString("  ")
	footer.WriteString(help)
	footer.WriteString("\n")
	footer.WriteString(m.textarea.View())

	return footer.String()
}

// formatMessage creates a Message from sender and content
func formatMessage(sender senderName, content string) Message {
	return Message{
		Sender:    sender,
		Content:   content,
		HasAudio:  false, // Default, can be updated later
		Timestamp: time.Now(),
	}
}

// formatError creates an error Message
func formatError(err error) Message {
	return Message{
		Sender:    "System",
		Content:   fmt.Sprintf("Error: %v", err),
		HasAudio:  false,
		Timestamp: time.Now(),
	}
}

// formatErrorString formats an error as a string for the UI
func formatErrorString(err error) string {
	return errorStyle.Render(fmt.Sprintf("Error: %v", err))
}

// detectAudioPlayer attempts to find a suitable audio player command
func detectAudioPlayer() string {
	var cmd string
	var playerPath string
	var err error

	// Try ffplay (FFmpeg) first - handles stdin well
	if playerPath, err = exec.LookPath("ffplay"); err == nil {
		// Note: `-i -` reads from stdin. Assumes raw PCM with format flags.
		// WAV header *might* also work with ffplay but raw is cleaner if flags are right.
		cmd = fmt.Sprintf("%s -autoexit -nodisp -loglevel error -f %s -ar %d -ac 1 -i -", playerPath, audioFormat, audioSampleRate)
		log.Printf("Auto-detected audio player: %s (using ffplay)", cmd)
		return cmd
	}

	// Try Linux-specific players
	if runtime.GOOS == "linux" {
		if playerPath, err = exec.LookPath("aplay"); err == nil {
			// aplay needs specific format flags for raw PCM from stdin
			cmd = fmt.Sprintf("%s -q -c 1 -r %d -f %s -", playerPath, audioSampleRate, "S16_LE") // S16_LE matches s16le
			log.Printf("Auto-detected audio player: %s (using aplay)", cmd)
			return cmd
		}
		if playerPath, err = exec.LookPath("paplay"); err == nil {
			// PulseAudio player, also needs format flags for raw PCM
			cmd = fmt.Sprintf("%s --raw --channels=1 --rate=%d --format=%s", playerPath, audioSampleRate, audioFormat)
			log.Printf("Auto-detected audio player: %s (using paplay)", cmd)
			return cmd
		}
	}

	// Try macOS player (afplay) - requires temp files
	if runtime.GOOS == "darwin" {
		if playerPath, err = exec.LookPath("afplay"); err == nil {
			log.Println("Detected 'afplay'. Will use temp files for playback.")
			// Return just "afplay" as the command name. Playback logic handles the rest.
			return "afplay"
		} else {
			log.Println("Info: 'ffplay' not found. For best audio on macOS, install FFmpeg (`brew install ffmpeg`).")
		}
	}

	// Try ffmpeg as a player (less common, might depend on output device setup)
	if playerPath, err = exec.LookPath("ffmpeg"); err == nil {
		audioOutput := "alsa" // Default for Linux
		if runtime.GOOS == "darwin" {
			audioOutput = "coreaudio"
		} else if runtime.GOOS == "windows" {
			audioOutput = "dsound" // Example for Windows DirectSound
		}
		// Reads raw PCM from stdin, outputs to default audio device
		cmd = fmt.Sprintf("%s -f %s -ar %d -ac 1 -i - -f %s -", playerPath, audioFormat, audioSampleRate, audioOutput)
		log.Printf("Auto-detected audio player: %s (using ffmpeg)", cmd)
		return cmd
	}

	// Fallback if nothing found
	if runtime.GOOS == "windows" {
		log.Println("Warning: Audio playback auto-detect failed for Windows. Install FFmpeg or use WithAudioPlayerCommand.")
	} else {
		log.Println("Warning: Could not auto-detect a suitable audio player. Please install ffplay, aplay, paplay, or use WithAudioPlayerCommand.")
	}
	return "" // Return empty string if no player found
}

// logInterceptor implements io.Writer to capture log output for display in UI
type logInterceptor struct {
	model    *Model
	original io.Writer // The original log output
}

func (li *logInterceptor) Write(p []byte) (n int, err error) {
	message := string(p)

	// Add message to the model's log messages (if enabled in model)
	if li.model != nil && li.model.maxLogMessages > 0 {
		// Trim whitespace for cleaner display
		trimmedMessage := strings.TrimSpace(message)
		if trimmedMessage != "" { // Avoid adding empty lines
			// Add to the model's log messages
			li.model.logMessages = append(li.model.logMessages, trimmedMessage)
			// Trim to max length
			if len(li.model.logMessages) > li.model.maxLogMessages {
				li.model.logMessages = li.model.logMessages[len(li.model.logMessages)-li.model.maxLogMessages:]
			}
			// Send message to update UI if needed (optional)
			// tea.Batch(logMessageMsg{message: trimmedMessage})? Needs careful handling.
		}
	}

	// Write to the original log output (e.g., file)
	if li.original != nil {
		// Write original bytes to preserve formatting in log file
		return li.original.Write(p)
	}

	return len(p), nil
}

// formatDuration converts total seconds into MM:SS format.
func formatDuration(totalSeconds float64) string {
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	// Round to nearest second for display consistency
	ts := int(math.Round(totalSeconds))
	minutes := ts / 60
	seconds := ts % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// formatToolCallMessage creates a Message for a tool call with enhanced formatting
func formatToolCallMessage(toolCall ToolCall, status ToolCallStatus) Message {
	var content strings.Builder
	toolCallStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true) // Magenta style for tool calls
	idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))                  // Subtle gray for IDs
	spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208"))             // Orange for spinner
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75"))               // Blue for status

	content.WriteString(toolCallStyle.Render(fmt.Sprintf("🔧 Tool Call: %s", toolCall.Name)))
	content.WriteString(" ")
	content.WriteString(idStyle.Render(fmt.Sprintf("(ID: %s)", toolCall.ID)))

	// Add a spinner or status indicator based on the status
	switch status {
	case ToolCallStatusRunning:
		content.WriteString(" ")

		// Use a more animated spinner character based on the current time
		// This will create an animation effect when rendering is updated
		spinnerChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinnerIndex := int(time.Now().UnixNano()/100000000) % len(spinnerChars)
		spinnerChar := spinnerChars[spinnerIndex]

		content.WriteString(spinnerStyle.Render(fmt.Sprintf("%s Running...", spinnerChar)))
	case ToolCallStatusPending:
		content.WriteString(" ")
		content.WriteString(spinnerStyle.Render("⏳ Pending..."))
	case ToolCallStatusCompleted:
		content.WriteString(" ")
		content.WriteString(statusStyle.Render("✓ Completed"))
	case ToolCallStatusRejected:
		content.WriteString(" ")
		content.WriteString(statusStyle.Render("✗ Rejected"))
	}
	content.WriteString("\n")

	// Format arguments
	if len(toolCall.Arguments) > 0 {
		var args bytes.Buffer
		if err := json.Indent(&args, toolCall.Arguments, "", "  "); err != nil {
			// Fallback if indent fails
			content.WriteString(fmt.Sprintf("Arguments: %s", string(toolCall.Arguments)))
		} else {
			content.WriteString("```json\n")
			content.WriteString(args.String())
			content.WriteString("\n```\n")
		}
	} else {
		content.WriteString("Arguments: (none)\n")
	}

	return Message{
		Sender:     "System",
		Content:    content.String(), // Use the pre-formatted content
		IsToolCall: true,
		ToolCall:   &toolCall,
		Timestamp:  time.Now(),
	}
}

func formatToolResultMessage(toolCallID, toolName string, result *structpb.Struct, status ToolCallStatus) Message {
	var content strings.Builder
	toolResultStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
	idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	content.WriteString(toolResultStyle.Render(fmt.Sprintf("✅ Tool Result: %s", toolName)))
	content.WriteString(" ")
	content.WriteString(idStyle.Render(fmt.Sprintf("(ID: %s)", toolCallID)))
	content.WriteString("\n")

	if result != nil && len(result.Fields) > 0 {
		jsonBytes, err := protojson.Marshal(result)
		if err != nil {
			content.WriteString(fmt.Sprintf("Error marshaling result: %v", err))
		} else {
			var prettyJSON bytes.Buffer
			if err := json.Indent(&prettyJSON, jsonBytes, "", "  "); err != nil {
				content.WriteString(string(jsonBytes))
			} else {
				content.WriteString("```json\n")
				content.WriteString(prettyJSON.String())
				content.WriteString("\n```\n")
			}
		}
	} else {
		content.WriteString("Result: (empty)\n")
	}

	return Message{
		Sender:         "System",
		Content:        content.String(),
		IsToolResponse: true,
		ToolResponse: &ToolResponse{
			Id:       toolCallID,
			Name:     toolName,
			Response: result,
		},
		ToolCall:  &ToolCall{ID: toolCallID, Name: toolName},
		Timestamp: time.Now(),
	}
}
