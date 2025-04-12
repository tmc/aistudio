package aistudio

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

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

	cleanedContent := strings.TrimSpace(msg.Content)

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
	} else {
		// For messages without audio
		if cleanedContent != "" {
			finalMsg.WriteString(cleanedContent)
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

	// Input mode indicator
	var inputMode string
	if m.micActive {
		inputMode = inputModeStyle.Render("[Mic ON]")
	} else if m.videoInputMode != VideoInputNone {
		inputMode = inputModeStyle.Render(fmt.Sprintf("[%s ON]", m.videoInputMode))
	}

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
	help := statusStyle.Render("Ctrl+M: Mic | Ctrl+S: Settings | Tab: Navigate | Ctrl+C: Quit")

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
	footer.WriteString(lipgloss.JoinVertical(lipgloss.Left, "", textAreaView, statusLine))

	return footer.String()
}
