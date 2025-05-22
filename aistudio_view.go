package aistudio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// formatToolApprovalArguments formats JSON arguments for display in the tool approval modal
func formatToolApprovalArguments(arguments json.RawMessage) string {
	var argsBuf bytes.Buffer
	if err := json.Indent(&argsBuf, arguments, "", "  "); err != nil {
		return string(arguments) // Fallback to raw JSON
	}
	return argsBuf.String()
}

// renderToolApprovalModalContent generates the text content for the tool approval modal.
func (m *Model) renderToolApprovalModalContent() string {
	if !m.showToolApproval || len(m.pendingToolCalls) == 0 || m.approvalIndex >= len(m.pendingToolCalls) {
		return ""
	}

	// Get the current tool call being approved
	toolCall := m.pendingToolCalls[m.approvalIndex]

	// Format arguments for display
	formattedArgs := formatToolApprovalArguments(toolCall.Arguments)

	// Build the modal content string
	var builder strings.Builder
	builder.WriteString(viewTitleStyle.Render("Tool Call Approval"))
	builder.WriteString("\n\n")

	// Tool name in the dialog style
	builder.WriteString(fmt.Sprintf("%s\n\n", viewToolNameStyle.Render(toolCall.Name)))

	// Arguments formatted in code block style
	builder.WriteString(formattedArgs)
	builder.WriteString("\n")
	builder.WriteString(dialogOptionUnselected.Render(fmt.Sprintf("Execute %s tool", toolCall.Name)))
	builder.WriteString("\n\n")

	// Add the options section
	builder.WriteString("Do you want to proceed?\n")

	// Option 1: Approve
	builder.WriteString(dialogOptionSelected.Render("❯ "))
	builder.WriteString(dialogOptionNum.Render("1."))
	builder.WriteString(dialogOptionSelected.Render("Yes") + "\n")

	// Option 2: Approve and don't ask again
	builder.WriteString("  ")
	builder.WriteString(dialogOptionUnselected.Render("2."))
	builder.WriteString(" Yes, and don't ask again for ")
	builder.WriteString(dialogActionHighlight.Render(toolCall.Name))
	builder.WriteString(" tools\n")

	// Option 3: Deny
	builder.WriteString("  ")
	builder.WriteString(dialogOptionUnselected.Render("3. "))
	builder.WriteString("No")
	builder.WriteString(fmt.Sprintf(" (%s)", dialogHintStyle.Render("esc")))
	builder.WriteString("\n")

	// Add progress indicator if there are multiple pending tool calls
	if len(m.pendingToolCalls) > 1 {
		builder.WriteString(fmt.Sprintf("\nTool call %d of %d", m.approvalIndex+1, len(m.pendingToolCalls)))
		// Add tab navigation if there are more calls
		if m.approvalIndex < len(m.pendingToolCalls)-1 {
			builder.WriteString(fmt.Sprintf(" (press %s for next)", dialogHintStyle.Render("tab")))
		}
	}

	return builder.String()
}

// renderViewToolApprovalModal renders a modal for approving tool calls
func (m *Model) renderViewToolApprovalModal() string {
	content := m.renderToolApprovalModalContent()
	if content == "" {
		return ""
	}

	// Apply box styling to the content
	return viewModalStyle.
		Width(m.width - 10).
		Align(lipgloss.Center).
		Render(content)
}

// renderInputArea builds the input area for user input
func (m *Model) renderInputArea() string {
	// Set the width for the textarea
	m.textarea.SetWidth(m.width)

	// Render the input area
	return m.textarea.View()
}

// renderStatusLine builds the status line for the footer
func (m *Model) renderStatusLine() string {
	var statusLine strings.Builder

	// Add input mode indicator if active
	if m.micActive {
		statusLine.WriteString(inputModeStyle.Render("[Mic ON] "))
	} else if m.videoInputMode != VideoInputNone {
		statusLine.WriteString(inputModeStyle.Render(fmt.Sprintf("[%s ON] ", m.videoInputMode)))
	}

	// Show current state with spinner when appropriate
	switch m.currentState {
	case AppStateWaiting:
		if m.isAudioProcessing && m.enableAudio {
			statusLine.WriteString(viewSpinnerStyle.Render(m.spinner.View()) + " " + statusStyle.Render("Playing audio..."))
		} else if m.processingTool {
			statusLine.WriteString(viewSpinnerStyle.Render(m.spinner.View()) + " " + viewToolStyle.Render("Processing tool..."))
		} else {
			statusLine.WriteString(viewSpinnerStyle.Render(m.spinner.View()) + " " + statusStyle.Render("Waiting for response..."))
		}
	case AppStateInitializing:
		statusLine.WriteString(viewSpinnerStyle.Render(m.spinner.View()) + " " + statusStyle.Render("Initializing..."))
	case AppStateReady:
		statusLine.WriteString(statusStyle.Render("Ready"))
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

	return statusLine.String()
}

// renderHelpText returns the help text for keyboard shortcuts
func (m *Model) renderHelpText() string {
	// Build help text for available keyboard shortcuts
	helpParts := []string{"Enter: Send", "Tab: Navigate", "↑/↓: Scroll", "PgUp/PgDn: Page", "Ctrl+C: Quit"}

	if m.historyEnabled {
		helpParts = append(helpParts, "Ctrl+H: Save History")
	}
	if m.enableTools {
		helpParts = append(helpParts, "Ctrl+T: Tools")
		// Show appropriate tool approval status and toggle hint
		if m.requireApproval {
			helpParts = append(helpParts, "Y/N: Tool Approval")
			helpParts = append(helpParts, "Ctrl+A: Disable Approval")
		} else {
			helpParts = append(helpParts, "Ctrl+A: Enable Approval")
		}
	}
	if m.enableAudio {
		helpParts = append(helpParts, "Ctrl+P: Play/Pause")
		helpParts = append(helpParts, "Ctrl+R: Replay")
	}

	return statusStyle.Render(strings.Join(helpParts, " | "))
}

// View renders the UI by composing calls to helper render functions.
// Modified to only show the input area and status line, not the full TUI
func (m *Model) View() string {
	// Handle special application states first
	if m.currentState == AppStateQuitting {
		return "Closing stream and quitting...\n"
	}

	if m.currentState == AppStateError && m.err != nil {
		return fmt.Sprintf("Error: %v", m.err)
	}

	parts := []string{}
	parts = append(parts, viewTitleStyle.Render("AI Studio"))

	// Add model name explicitly for tests
	parts = append(parts, fmt.Sprintf("Model: %s", m.modelName))

	parts = append(parts, m.renderAllMessages())
	// Add the input area
	parts = append(parts, m.renderInputArea())
	// Add the status line
	parts = append(parts, m.renderStatusLine())
	// Add the help text
	parts = append(parts, m.renderHelpText())
	// Add the tool approval modal if needed
	if m.showToolApproval && len(m.pendingToolCalls) > 0 {
		parts = append(parts, m.renderViewToolApprovalModal())
	}

	return strings.Join(parts, "\n")
}
