package aistudio

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// MessageRenderer handles the rendering of messages in the UI
type MessageRenderer struct {
	model *Model
}

// NewMessageRenderer creates a new message renderer
func NewMessageRenderer(model *Model) *MessageRenderer {
	return &MessageRenderer{
		model: model,
	}
}

// RenderMessages renders all messages for display
func (r *MessageRenderer) RenderMessages() string {
	// Get available width for message borders
	availWidth := r.model.width - 4 // Account for borders and padding

	// Group related messages together
	messageGroups := r.groupMessages()

	// Render each message group
	var formattedMessages []string

	for _, group := range messageGroups {
		if len(group) == 0 {
			continue
		}

		rendered := r.renderMessageGroup(group, availWidth)
		if rendered != "" {
			formattedMessages = append(formattedMessages, rendered)
		}
	}

	// Join with spacing between bordered messages
	return strings.Join(formattedMessages, "\n")
}

// renderMessageGroup renders a group of related messages (like a tool call and its response)
func (r *MessageRenderer) renderMessageGroup(group []Message, availWidth int) string {
	// For multi-message groups like tool calls + responses
	if len(group) > 1 {
		return r.renderMultiMessage(group, availWidth)
	}

	// Single message
	return r.renderSingleMessage(group[0], availWidth)
}

// renderMultiMessage renders a group of related messages together
func (r *MessageRenderer) renderMultiMessage(messages []Message, availWidth int) string {
	var groupContent strings.Builder

	// Find indexes of these messages in the original list
	for _, msg := range messages {
		index := r.findMessageIndex(msg)
		formatted := r.formatMessageText(msg, index)
		if formatted != "" {
			groupContent.WriteString(formatted)
		}
	}

	// Render the whole group with appropriate border
	content := groupContent.String()
	cleanContent := StripANSI(content)
	if strings.TrimSpace(cleanContent) != "" {
		var style lipgloss.Style
		if messages[0].IsToolCall() || messages[0].IsToolResponse() {
			style = toolBorderStyle.Copy().Width(availWidth)
		} else {
			style = systemMessageStyle.Copy().Width(availWidth)
		}
		return style.Render(content)
	}

	return ""
}

// renderSingleMessage renders a single message
func (r *MessageRenderer) renderSingleMessage(msg Message, availWidth int) string {
	index := r.findMessageIndex(msg)

	// Get the right border style for this message
	var messageBorderStyle lipgloss.Style
	if msg.IsToolCall() || msg.IsToolResponse() {
		messageBorderStyle = toolBorderStyle.Copy().Width(availWidth)
	} else {
		messageBorderStyle = systemMessageStyle.Copy().Width(availWidth)
	}

	// Format the message text
	formatted := r.formatMessageText(msg, index)

	// Only render if there's actual content after stripping ANSI codes and whitespace
	cleanContent := StripANSI(formatted)
	if strings.TrimSpace(cleanContent) != "" {
		return messageBorderStyle.Render(formatted)
	}

	return ""
}

// findMessageIndex finds the index of a message in the model's message list
func (r *MessageRenderer) findMessageIndex(msg Message) int {
	for i, m := range r.model.messages {
		if m.Timestamp == msg.Timestamp && m.Content == msg.Content {
			return i
		}
	}
	return -1
}

// groupMessages groups related messages (like tool calls and responses) together
func (r *MessageRenderer) groupMessages() [][]Message {
	var messageGroups [][]Message
	var currentGroup []Message
	processedToolCallIDs := make(map[string]bool)

	for i, msg := range r.model.messages {
		// Skip already processed tool calls
		if msg.IsToolCall() && msg.ToolCall != nil {
			if _, alreadyProcessed := processedToolCallIDs[msg.ToolCall.ID]; alreadyProcessed {
				continue
			}
		}

		// Start a new tool call group
		if msg.IsToolCall() {
			// Finish any existing group
			if len(currentGroup) > 0 {
				messageGroups = append(messageGroups, currentGroup)
				currentGroup = nil
			}

			// Start a new group with this tool call
			currentGroup = append(currentGroup, msg)

			// Look ahead for matching tool response
			responseMsg := r.findToolResponse(msg, i+1)
			if responseMsg != nil {
				// Add matching response to this group
				currentGroup = append(currentGroup, *responseMsg)
				// Mark this tool call as processed
				if msg.ToolCall != nil {
					processedToolCallIDs[msg.ToolCall.ID] = true
				}
			}

			// Add the group and start a new one
			messageGroups = append(messageGroups, currentGroup)
			currentGroup = nil
		} else if !msg.IsToolResponse() ||
			(msg.IsToolResponse() && msg.ToolResponse != nil && !processedToolCallIDs[msg.ToolResponse.Id]) {
			// For non-tool messages or unpaired tool responses, add as individual groups
			messageGroups = append(messageGroups, []Message{msg})
		}
	}

	// Add any remaining messages
	if len(currentGroup) > 0 {
		messageGroups = append(messageGroups, currentGroup)
	}

	return messageGroups
}

// findToolResponse looks for a matching tool response for a tool call
func (r *MessageRenderer) findToolResponse(toolCallMsg Message, startIdx int) *Message {
	if !toolCallMsg.IsToolCall() || toolCallMsg.ToolCall == nil {
		return nil
	}

	for j := startIdx; j < len(r.model.messages); j++ {
		responseMsg := r.model.messages[j]
		if responseMsg.IsToolResponse() &&
			responseMsg.ToolResponse != nil &&
			responseMsg.ToolResponse.Id == toolCallMsg.ToolCall.ID {
			return &responseMsg
		}
	}

	return nil
}

// formatMessageText formats a message's text with appropriate styling
func (r *MessageRenderer) formatMessageText(msg Message, messageIndex int) string {
	// Skip empty messages unless they have special formatting
	if msg.Content == "" &&
		!msg.HasAudio &&
		!msg.IsToolCall() &&
		!msg.IsToolResponse() &&
		!msg.IsExecutableCode &&
		!msg.IsExecutableCodeResult {
		return "" // Skip empty messages that don't have special formatting
	}

	var finalMsg strings.Builder

	// Add message header with type and index
	finalMsg.WriteString(r.formatMessageHeader(msg, messageIndex))

	switch {
	case msg.HasAudio:
		r.formatAudioMessage(&finalMsg, msg, messageIndex)
	case msg.ToolCall != nil:
		finalMsg.WriteString(msg.Content) // Pre-formatted content
	case msg.ToolResponse != nil:
		finalMsg.WriteString(msg.Content) // Pre-formatted content
	case msg.IsExecutableCode:
		r.formatExecutableCodeMessage(&finalMsg, msg)
	case msg.IsExecutableCodeResult:
		r.formatExecutableCodeResultMessage(&finalMsg, msg)
	default:
		r.formatDefaultMessage(&finalMsg, msg)
	}

	return finalMsg.String()
}

// formatMessageHeader formats the header of a message
func (r *MessageRenderer) formatMessageHeader(msg Message, messageIndex int) string {
	var senderStyle lipgloss.Style
	switch msg.Sender {
	case senderNameUser:
		senderStyle = senderUserStyle
	case senderNameModel:
		senderStyle = senderModelStyle
	default:
		senderStyle = senderSystemStyle
	}

	// Determine message type
	var msgType string
	switch {
	case msg.IsToolCall():
		msgType = "ToolCall"
	case msg.IsToolResponse():
		msgType = "ToolResponse"
	case msg.HasAudio:
		msgType = "Audio"
	case msg.IsExecutableCode:
		msgType = "Code"
	case msg.IsExecutableCodeResult:
		msgType = "CodeResult"
	default:
		msgType = "Text"
	}

	// Display a short version of the ID (first 8 characters) if available
	idDisplay := ""
	if msg.ID != "" {
		if len(msg.ID) > 8 {
			idDisplay = ":" + msg.ID[0:8]
		} else {
			idDisplay = ":" + msg.ID
		}
	}

	return senderStyle.Render(fmt.Sprintf("%s [%s:%d%s]: ",
		string(msg.Sender), msgType, messageIndex, idDisplay))
}

// formatAudioMessage formats an audio message
func (r *MessageRenderer) formatAudioMessage(finalMsg *strings.Builder, msg Message, messageIndex int) {
	audioLine := r.formatAudioLine(msg, messageIndex)

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

// formatAudioLine formats the audio line of a message
func (r *MessageRenderer) formatAudioLine(msg Message, messageIndex int) string {
	audioStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true) // Magenta for audio
	audioLine := audioStyle.Render(fmt.Sprintf("🔊 Audio: "))
	return audioLine
}

// formatExecutableCodeMessage formats an executable code message
func (r *MessageRenderer) formatExecutableCodeMessage(finalMsg *strings.Builder, msg Message) {
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

// formatExecutableCodeResultMessage formats an executable code result message
func (r *MessageRenderer) formatExecutableCodeResultMessage(finalMsg *strings.Builder, msg Message) {
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

// formatDefaultMessage formats a regular message
func (r *MessageRenderer) formatDefaultMessage(finalMsg *strings.Builder, msg Message) {
	if msg.Content != "" {
		finalMsg.WriteString(msg.Content)
	}
	r.formatTokenCounts(finalMsg, msg)
}

// formatTokenCounts formats token count information
func (r *MessageRenderer) formatTokenCounts(finalMsg *strings.Builder, msg Message) {
	if msg.TokenCounts != nil {
		tokenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
		finalMsg.WriteString(tokenStyle.Render(
			fmt.Sprintf("Token Counts: %d (Input) / %d (Output)",
				msg.TokenCounts.PromptTokenCount,
				msg.TokenCounts.ResponseTokenCount)))
		finalMsg.WriteString("\n")
	}
}
