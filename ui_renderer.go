package aistudio

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// UIRenderer encapsulates UI rendering logic
type UIRenderer struct {
	model *Model
}

// NewUIRenderer creates a new UI renderer
func NewUIRenderer(model *Model) *UIRenderer {
	return &UIRenderer{
		model: model,
	}
}

// RenderHeader renders the application header
func (r *UIRenderer) RenderHeader() string {
	var builder strings.Builder

	// Add program title with logo if enabled
	if r.model.showLogo {
		// Simple styled app title
		builder.WriteString("AIStudio CLI")
		builder.WriteString("\n")
	}

	// Add model name and/or additional header info if needed
	if r.model.modelName != "" {
		modelInfo := fmt.Sprintf("Model: %s", r.model.modelName)
		builder.WriteString(modelInfo)
		builder.WriteString("\n")
	}

	return builder.String()
}

// RenderFooter renders the application footer
func (r *UIRenderer) RenderFooter() string {
	var builder strings.Builder

	// Add info line (model name, etc.)
	infoLine := r.RenderInfoLine()
	builder.WriteString(infoLine)
	builder.WriteString("\n")

	// Add status line with spinner if processing
	statusLine := r.model.renderStatusLine()
	builder.WriteString(statusLine)

	// Add help text (keyboard shortcuts)
	helpText := r.model.renderHelpText()
	builder.WriteString("  ")
	builder.WriteString(helpText)

	return builder.String()
}

// RenderInfoLine renders the info line with model and tools information
func (r *UIRenderer) RenderInfoLine() string {
	var info string
	if r.model.enableTools && r.model.toolManager != nil {
		toolCount := len(r.model.toolManager.RegisteredToolDefs)
		info = fmt.Sprintf("Model: %s | Tools: %d available", r.model.modelName, toolCount)
	} else {
		info = fmt.Sprintf("Model: %s", r.model.modelName)
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("69")).
		Render(info)
}

// RenderLogMessages renders the log messages section
func (r *UIRenderer) RenderLogMessages() string {
	if !r.model.showLogMessages || len(r.model.logMessages) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Recent Log Messages:\n")

	// Show most recent messages first, up to maxLogMessages
	start := len(r.model.logMessages) - r.model.maxLogMessages
	if start < 0 {
		start = 0
	}

	for i := start; i < len(r.model.logMessages); i++ {
		builder.WriteString(r.model.logMessages[i])
		builder.WriteString("\n")
	}

	return builder.String()
}
