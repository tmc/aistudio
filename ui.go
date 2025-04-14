package aistudio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"cloud.google.com/go/ai/generativelanguage/apiv1alpha/generativelanguagepb"
	"github.com/charmbracelet/lipgloss"
)

// formatMessageText formats a message as a string for display, including audio UI
func (m *Model) formatMessageText(msg Message, messageIndex int) string { // Added m *Model, messageIndex int
	var senderStyle lipgloss.Style
	switch msg.Sender {
	case "You":
		senderStyle = senderYouStyle
	case "Gemini":
		senderStyle = senderGeminiStyle
	default:
		senderStyle = senderSystemStyle
	}

	// Keep sender header even if content is empty
	header := senderStyle.Render(msg.Sender + ":")

	cleanedContent := msg.Content

	var audioLine strings.Builder
	if msg.HasAudio {
		// Check if this message corresponds to the active audio player
		isActiveAudio := m.activeAudioPlayer != nil && m.activeAudioPlayer.MessageIndex == messageIndex

		if isActiveAudio {
			// Use the audio player component to render the audio line
			audioLine.WriteString(m.activeAudioPlayer.View())
			audioLine.WriteString("\n")
		} else {
			// Render a static representation for non-active audio players
			totalSeconds := 0.0
			if len(msg.AudioData) > 0 {
				// Calculate total duration from stored *complete* audio data length
				totalSeconds = float64(len(msg.AudioData)) / 48000.0
			} else if msg.IsPlaying && m.currentAudio != nil && m.currentAudio.Duration > 0 && m.currentAudio.MessageIndex == messageIndex {
				// Fallback ONLY if message data is missing but currently playing chunk matches index
				totalSeconds = m.currentAudio.Duration
				log.Printf("[UI Warning] Using currentAudio duration for Msg #%d as AudioData is empty.", messageIndex)
			}
			totalDurationStr := formatDuration(totalSeconds) // helpers.go

			audioIcon := "❓"
			timestampStr := fmt.Sprintf("??:?? / %s", totalDurationStr)
			progressBar := strings.Repeat("╌", progressBarWidth) // Default empty bar
			helpText := ""                                       // Hint for Play/Replay

			if msg.IsPlaying {
				audioIcon = audioPlayIcon // Green playing icon from constants.go

				// --- Get Progress ---
				elapsedSeconds := 0.0
				// Check if the currently playing audio chunk corresponds to THIS message index
				if m.currentAudio != nil && m.currentAudio.IsProcessing && m.currentAudio.MessageIndex == messageIndex && !m.currentAudio.StartTime.IsZero() {
					elapsedSeconds = time.Since(m.currentAudio.StartTime).Seconds()
					// Ensure elapsed doesn't exceed total due to timing issues
					elapsedSeconds = math.Min(elapsedSeconds, totalSeconds)
				}
				elapsedSeconds = math.Max(0, elapsedSeconds) // Ensure non-negative
				elapsedDurationStr := formatDuration(elapsedSeconds)
				timestampStr = fmt.Sprintf("%s / %s", elapsedDurationStr, totalDurationStr)

				// --- Calculate Progress Bar ---
				progress := 0.0
				if totalSeconds > 0 {
					progress = elapsedSeconds / totalSeconds
				}
				progress = math.Min(1.0, math.Max(0.0, progress)) // Clamp progress [0, 1]
				filledWidth := int(progress * float64(progressBarWidth))
				emptyWidth := progressBarWidth - filledWidth
				progressBar = strings.Repeat("━", filledWidth) + strings.Repeat("╌", emptyWidth)

			} else if msg.IsPlayed {
				audioIcon = audioPlayedIcon // Gray check for played
				timestampStr = fmt.Sprintf("%s / %s", totalDurationStr, totalDurationStr)
				progressBar = strings.Repeat("━", progressBarWidth) // Full bar
				helpText = "[R]eplay"
			} else {
				// Available but not played
				audioIcon = audioReadyIcon // Magenta speaker
				timestampStr = fmt.Sprintf("00:00 / %s", totalDurationStr)
				// progressBar remains empty ("╌"...)
				helpText = "[P]lay"
			}

			// Assemble the audio line
			audioLine.WriteString(audioIcon)
			audioLine.WriteString(" ")
			audioLine.WriteString(audioTimeStyle.Render(timestampStr))
			audioLine.WriteString(" ")
			audioLine.WriteString(audioProgStyle.Render(progressBar))
			if helpText != "" {
				audioLine.WriteString(" ")
				audioLine.WriteString(audioHelpStyle.Render(helpText))
			}
			audioLine.WriteString("\n")
		}
	}

	// Assemble the final message
	var finalMsg strings.Builder

	// Add message header
	finalMsg.WriteString(header)
	finalMsg.WriteString("\n")

	// Special handling to make audio playback in-line with Gemini messages
	if msg.HasAudio && msg.Sender == "Gemini" {
		// Get just the audio UI line without any wrapping newlines
		audioString := strings.TrimSpace(audioLine.String())

		// For Gemini messages with audio, place the content and audio inline
		if cleanedContent != "" {
			// Add the message text
			finalMsg.WriteString(cleanedContent)

			// Add the audio controls on the same line as the content
			if audioString != "" {
				// Add a space between content and audio controls
				finalMsg.WriteString(" ")
				finalMsg.WriteString(audioString)
			}

			finalMsg.WriteString("\n") // Just one newline after the combined content
		} else if audioString != "" {
			// If there's no content, just show the audio controls
			finalMsg.WriteString(audioString)
			finalMsg.WriteString("\n")
		}
	} else if msg.HasAudio {
		// For other senders with audio, keep them separate
		audioString := strings.TrimSpace(audioLine.String())

		if cleanedContent != "" {
			finalMsg.WriteString(cleanedContent)
			finalMsg.WriteString("\n")
		}

		if audioString != "" {
			finalMsg.WriteString(audioString)
			finalMsg.WriteString("\n")
		}
	} else if msg.IsToolCall || msg.FunctionCall != nil {
		// For tool calls or function calls, show the content and info about the tool
		if cleanedContent != "" {
			finalMsg.WriteString(cleanedContent)
			finalMsg.WriteString("\n")
		}

		// Handle regular tool calls
		if msg.IsToolCall && msg.ToolCall != nil {
			toolCallStr := fmt.Sprintf("Tool: %s", msg.ToolCall.Name)
			toolCallStr = toolCallStyle.Render(toolCallStr)
			finalMsg.WriteString(toolCallStr)
			finalMsg.WriteString("\n")

			// Show tool arguments if available
			if len(msg.ToolCall.Arguments) > 0 {
				var args bytes.Buffer
				if err := json.Indent(&args, msg.ToolCall.Arguments, "", "  "); err != nil {
					args.Write(msg.ToolCall.Arguments) // Fallback if formatting fails
				}

				finalMsg.WriteString("Arguments:\n")
				finalMsg.WriteString("```json\n")
				finalMsg.WriteString(args.String())
				finalMsg.WriteString("\n```\n")
			}
		}

		// Handle BidiGenerateContent function calls
		if msg.FunctionCall != nil {
			callStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39")) // Teal for function calls

			funcName := msg.FunctionCall.Name
			finalMsg.WriteString(callStyle.Render(fmt.Sprintf("📞 Function Call: %s", funcName)))
			finalMsg.WriteString("\n")

			// Format and display arguments
			if msg.FunctionCall.Args != nil {
				argsBytes, err := json.MarshalIndent(msg.FunctionCall.Args, "", "  ")
				if err != nil {
					argsBytes = []byte(fmt.Sprintf("%v", msg.FunctionCall.Args))
				}

				finalMsg.WriteString("Arguments:\n")
				finalMsg.WriteString("```json\n")
				finalMsg.WriteString(string(argsBytes))
				finalMsg.WriteString("\n```\n")
			}
		}
	} else if msg.IsExecutableCode {
		// For executable code, show content and the code block
		codeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // Blue for code

		if cleanedContent != "" {
			finalMsg.WriteString(cleanedContent)
			finalMsg.WriteString("\n")
		}
		if msg.ExecutableCode != nil {
			languageInfo := fmt.Sprintf("```%s", msg.ExecutableCode.Language)
			finalMsg.WriteString(codeStyle.Render(languageInfo))
			finalMsg.WriteString("\n")
			finalMsg.WriteString(msg.ExecutableCode.Code)
			finalMsg.WriteString("\n")
			finalMsg.WriteString(codeStyle.Render("```"))
			finalMsg.WriteString("\n")
		}
	} else if msg.IsExecutableCodeResult {
		// For executable code results, show content and result
		if cleanedContent != "" {
			finalMsg.WriteString(cleanedContent)
			finalMsg.WriteString("\n")
		}
		if msg.ExecutableCodeResult != nil {
			// Since we don't know the exact field access pattern, use a more general approach
			outputStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green for success
			finalMsg.WriteString(outputStyle.Render("Code Execution Result:"))
			finalMsg.WriteString("\n")
			finalMsg.WriteString("```\n")
			// Just show the output as-is, which should contain either the result or the error
			finalMsg.WriteString(fmt.Sprintf("%v", msg.ExecutableCodeResult))
			finalMsg.WriteString("\n```\n")
		}
	} else {
		// For messages without audio
		if cleanedContent != "" {
			finalMsg.WriteString(cleanedContent)
			finalMsg.WriteString("\n")
		}

		// Display safety ratings if present
		if len(msg.SafetyRatings) > 0 {
			safetyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // Orange for safety info

			finalMsg.WriteString(safetyStyle.Render("Safety Ratings:"))
			finalMsg.WriteString("\n")

			for _, rating := range msg.SafetyRatings {
				var ratingStyle lipgloss.Style

				// Color-code based on probability
				switch rating.Probability {
				case "HIGH":
					ratingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // Red for high
				case "MEDIUM":
					ratingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // Orange for medium
				case "LOW":
					ratingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Yellow for low
				default:
					ratingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green for negligible/unspecified
				}

				ratingStr := fmt.Sprintf("• %s: %s", rating.Category, rating.Probability)
				if rating.Score > 0 {
					ratingStr += fmt.Sprintf(" (%.3f)", rating.Score)
				}
				if rating.Blocked {
					ratingStr += " [BLOCKED]"
				}

				finalMsg.WriteString(ratingStyle.Render(ratingStr))
				finalMsg.WriteString("\n")
			}
		}

		// Display grounding metadata if present
		if msg.HasGroundingMetadata && msg.GroundingMetadata != nil {
			gmData := msg.GroundingMetadata

			// Display grounding chunks if available
			if len(gmData.Chunks) > 0 {
				chunkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // Light blue for chunks
				finalMsg.WriteString(chunkStyle.Render("Grounding Sources:"))
				finalMsg.WriteString("\n")

				for i, chunk := range gmData.Chunks {
					chunkStyle := lipgloss.NewStyle()
					if chunk.Selected {
						chunkStyle = chunkStyle.Bold(true)
					}

					chunkInfo := fmt.Sprintf("%d. %s", i+1, chunk.Title)
					if chunk.URI != "" {
						chunkInfo += fmt.Sprintf(" (%s)", chunk.URI)
					}

					finalMsg.WriteString(chunkStyle.Render(chunkInfo))
					finalMsg.WriteString("\n")
				}
			}

			// Display web search queries if available
			if len(gmData.WebSearchQueries) > 0 {
				queryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39")) // Teal for queries
				finalMsg.WriteString(queryStyle.Render("Suggested Search Queries:"))
				finalMsg.WriteString("\n")

				for i, query := range gmData.WebSearchQueries {
					finalMsg.WriteString(fmt.Sprintf("%d. %s", i+1, query))
					finalMsg.WriteString("\n")
				}
			}
		}

		// Display token counts if enabled and available
		if m.displayTokenCounts && msg.HasTokenInfo {
			tokenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Faint(true) // Gray, faint for token counts
			tokenInfo := fmt.Sprintf("Tokens: %d prompt + %d response = %d total",
				msg.PromptTokenCount, msg.ResponseTokenCount, msg.TotalTokenCount)
			finalMsg.WriteString(tokenStyle.Render(tokenInfo))
			finalMsg.WriteString("\n")
		}
	}

	// Add an extra newline for spacing between messages
	finalMsg.WriteString("\n")

	return finalMsg.String()
}

// formatAllMessages formats all messages as a single string for display
// It now requires the model to pass context to formatMessageText
func (m *Model) formatAllMessages() string {
	var formattedMessages []string
	for i, msg := range m.messages {
		// Pass the model 'm' and the message index 'i'
		formatted := m.formatMessageText(msg, i)
		if formatted != "" {
			formattedMessages = append(formattedMessages, formatted)
		}
	}
	// Use a single newline to join, as formatMessageText now adds trailing newlines
	return strings.Join(formattedMessages, "")
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

// footerView renders the footer for the UI
func (m Model) footerView() string {
	var footer strings.Builder

	// Add optional boxes first
	logBox := m.logMessagesView()
	audioBox := m.audioStatusView()

	if logBox != "" {
		footer.WriteString(logBox)
		footer.WriteRune('\n')
	}
	if audioBox != "" {
		footer.WriteString(audioBox)
		footer.WriteRune('\n')
	}

	// --- Input Area and Status Line ---

	// Input mode indicator (Mic or Video)
	var inputMode string
	if m.micActive {
		inputMode = inputModeStyle.Render("[Mic ON]") // Display Mic status if active
	} else if m.videoInputMode != VideoInputNone {
		inputMode = inputModeStyle.Render(fmt.Sprintf("[%s ON]", m.videoInputMode)) // Display Video status if active
	}
	// If neither is active, inputMode remains an empty string

	// Status indicator
	var status string
	if m.err != nil {
		errStr := fmt.Sprintf("Error: %v", m.err)
		// Truncate error if too long for status line
		maxErrWidth := m.width / 3 // Limit error display width
		if lipgloss.Width(errStr) > maxErrWidth {
			// Basic truncation, might cut mid-word
			errStr = errStr[:maxErrWidth-3] + "..."
		}
		status = errorStyle.Render(errStr)
	} else if !m.streamReady && !m.quitting {
		status = m.spinner.View() + " Connecting..."
	} else if m.sending {
		status = m.spinner.View() + " Sending..."
	} else if m.receiving && m.streamReady {
		// Only show receiving if not playing audio (playing is more specific)
		if !m.isAudioProcessing {
			status = statusStyle.Render("Connected...")
		} else {
			status = m.spinner.View() + " Playing audio..." // Show playing status here too
		}
	} else if m.isAudioProcessing && m.enableAudio {
		status = m.spinner.View() + " Playing audio..."
	} else if m.streamReady {
		status = statusStyle.Render("Ready.")
	} else {
		status = statusStyle.Render("Disconnected. Ctrl+C exit.")
	}

	// Help text
	var helpParts []string
	helpParts = append(helpParts, "Ctrl+M: Mic")
	if m.historyEnabled {
		helpParts = append(helpParts, "Ctrl+H: Save History")
	}
	if m.enableTools {
		helpParts = append(helpParts, "Ctrl+T: Tools")
	}
	helpParts = append(helpParts, "Ctrl+S: Settings")
	helpParts = append(helpParts, "Tab: Navigate")
	helpParts = append(helpParts, "Ctrl+C: Quit")
	help := statusStyle.Render(strings.Join(helpParts, " | "))

	// Layout status line elements
	statusWidth := lipgloss.Width(status)
	inputModeWidth := lipgloss.Width(inputMode)
	helpWidth := lipgloss.Width(help)

	// Calculate space for the spacer between status/input and help
	// Ensure total doesn't exceed available width
	availableWidth := m.width - statusWidth - inputModeWidth - helpWidth
	spacerWidth := availableWidth - 2 // Subtract 2 for minimal spacing around spacer

	if spacerWidth < 1 {
		spacerWidth = 1 // Ensure at least one space
	}
	spacer := strings.Repeat(" ", spacerWidth)

	// Construct status line, ensuring it doesn't overflow
	statusLine := lipgloss.NewStyle().Width(m.width).Render(
		lipgloss.JoinHorizontal(lipgloss.Bottom, status, " ", inputMode, spacer, help),
	)

	// Text area
	textAreaView := m.textarea.View()

	// Combine input area and status line
	footerElements := []string{"", textAreaView, statusLine} // Start with empty line, text area, status line

	// Prepend tools info only if tools are enabled
	if m.enableTools && m.toolManager != nil {
		toolsInfoView := fmt.Sprintf("Tools: %d available", len(m.toolManager.RegisteredToolDefs))
		toolsInfoStyled := statusStyle.Render(toolsInfoView) // Use status style for consistency
		// Insert tools info after the initial empty line for spacing
		footerElements = append([]string{footerElements[0], toolsInfoStyled}, footerElements[1:]...)
	}

	footer.WriteString(lipgloss.JoinVertical(lipgloss.Left, footerElements...))

	return footer.String()
}
