package aistudio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// styleDefinitions contains all the styles used in the UI.
// Centralizing styles helps with consistency and easier updates.
var (
	// Text colors for views - these override constants.go styles within this file
	viewTitleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))                                                    // Magenta
	viewToolNameStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))                                                    // Teal
	viewButtonStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("63")).Padding(0, 2).MarginRight(2)  // Black on Magenta
	viewButtonDangerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("160")).Padding(0, 2).MarginRight(2) // Black on Red
	viewSpinnerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))                                                              // Pink for spinner
	viewToolStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))                                                               // Teal for tools

	// Borders and containers
	viewportStyle      = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	settingsPanelStyle = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	viewModalStyle     = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 3)
)

// renderHeader renders the header part of the UI.
func (m *Model) renderHeader() string {
	return m.headerView() // Calls existing function in ui.go
}

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
	builder.WriteString(viewTitleStyle.Render("Tool Call Approval Required"))
	builder.WriteString("\n\n")
	builder.WriteString(fmt.Sprintf("Tool: %s", viewToolNameStyle.Render(toolCall.Name)))
	builder.WriteString("\n\n")
	builder.WriteString("Arguments:\n")
	builder.WriteString("```json\n") // Use markdown for code block
	builder.WriteString(formattedArgs)
	builder.WriteString("\n```\n\n")

	// Add progress indicator if there are multiple pending tool calls
	if len(m.pendingToolCalls) > 1 {
		builder.WriteString(fmt.Sprintf("Tool call %d of %d\n\n", m.approvalIndex+1, len(m.pendingToolCalls)))
	}

	// Add action buttons
	builder.WriteString(viewButtonStyle.Render("Y: Approve"))
	builder.WriteString(" ")
	builder.WriteString(viewButtonDangerStyle.Render("N: Deny"))
	if len(m.pendingToolCalls) > 1 && m.approvalIndex < len(m.pendingToolCalls)-1 {
		builder.WriteString(" ")
		builder.WriteString(viewButtonStyle.Render("Tab: Next")) // Assuming Tab navigates if multiple calls
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

// renderViewportContent renders the main content area (messages or modal).
func (m *Model) renderViewportContent() string {
	// Apply styles to viewport
	m.viewport.Style = viewportStyle

	if m.showToolApproval && len(m.pendingToolCalls) > 0 {
		// Render the modal instead of the normal viewport content
		return lipgloss.NewStyle().
			Width(m.viewport.Width).
			Height(m.viewport.Height).
			Render(m.renderViewToolApprovalModal())
	}

	// Show normal viewport content (messages)
	return m.viewport.View()
}

// renderFooter renders the footer part of the UI (input, status, etc.).
func (m *Model) renderFooter() string {
	return m.footerView() // Calls existing function in ui.go
}

// renderSettingsPanel renders the settings panel view.
func (m *Model) renderSettingsPanel(height int) string {
	// Calculate appropriate width
	settingsPanelWidth := m.width / 4
	settingsPanelWidth = min(settingsPanelWidth, m.width/4) // Ensure limit

	// Get the settings panel content
	settingsView := m.settingsPanel.View()

	// Apply styling to settings panel
	return settingsPanelStyle.
		Width(settingsPanelWidth).
		Height(height).
		Render(settingsView)
}

// renderCombinedLayout joins the main content view and the settings panel view horizontally.
func (m *Model) renderCombinedLayout(mainContent string, settingsStyled string, height int) string {
	// Calculate widths
	settingsPanelWidth := m.width / 4
	settingsPanelWidth = min(settingsPanelWidth, m.width/4)
	mainContentWidth := m.width - settingsPanelWidth - 1 // Account for padding

	// Style the main content with correct width
	styledMainContent := lipgloss.NewStyle().
		Width(mainContentWidth).
		Height(height).
		Render(mainContent)

	// Join components horizontally
	return lipgloss.JoinHorizontal(lipgloss.Top, settingsStyled, " ", styledMainContent)
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
	}
	if m.enableAudio {
		helpParts = append(helpParts, "Ctrl+P: Play/Pause")
		helpParts = append(helpParts, "Ctrl+R: Replay")
	}

	return statusStyle.Render(strings.Join(helpParts, " | "))
}

// renderInfoLine returns the model/tools info line
func (m *Model) renderInfoLine() string {
	if m.enableTools && m.toolManager != nil {
		toolCount := len(m.toolManager.RegisteredToolDefs)
		return statusStyle.Render(fmt.Sprintf("Model: %s | Tools: %d available", m.modelName, toolCount))
	}
	return statusStyle.Render(fmt.Sprintf("Model: %s", m.modelName))
}

// View renders the UI by composing calls to helper render functions.
// Modified to only show the input area and status line, not the full TUI
func (m *Model) View() string {
	parts := []string{}

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

	// Handle special application states
	if m.currentState == AppStateQuitting {
		return "Closing stream and quitting...\n"
	}

	if m.currentState == AppStateError && m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	// Set the width for the textarea
	m.textarea.SetWidth(m.width)

	// Render the status information
	infoLine := m.renderInfoLine()
	statusLine := m.renderStatusLine()
	helpText := m.renderHelpText()

	// Combine into footer
	var footer strings.Builder
	footer.WriteString(infoLine)
	footer.WriteString("\n")
	footer.WriteString(statusLine)
	footer.WriteString("  ")
	footer.WriteString(helpText)
	footer.WriteString("\n")
	footer.WriteString(m.textarea.View())

	return footer.String()
}
