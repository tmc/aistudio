package settings

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model represents the settings panel state
type Model struct {
	Width           int
	Height          int
	Focused         bool
	CurrentModel    string
	CurrentVoice    string
	AudioEnabled    bool
	ShowLogo        bool
	ShowLogMessage  bool
	ShowAudioStatus bool
}

// New creates a new settings model
func New() Model {
	return Model{
		CurrentModel:    "gemini-pro",
		CurrentVoice:    "Default",
		AudioEnabled:    true,
		ShowLogo:        true,
		ShowLogMessage:  false,
		ShowAudioStatus: true,
	}
}

// Init initializes the settings model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles updating the settings model
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width / 3
		m.Height = msg.Height
	case tea.KeyMsg:
		if !m.Focused {
			return m, nil
		}

		if msg.String() == "esc" {
			m.Focused = false
		}
	}

	return m, nil
}

// View renders the settings panel
func (m Model) View() string {
	if !m.Focused {
		return ""
	}

	style := lipgloss.NewStyle().
		Width(m.Width).
		Height(m.Height).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	content := fmt.Sprintf("Settings\n\nModel: %s\nVoice: %s\nAudio Enabled: %t\nShow Logo: %t\nShow Log Messages: %t\nShow Audio Status: %t\n\nPress ESC to close",
		m.CurrentModel, m.CurrentVoice, m.AudioEnabled, m.ShowLogo, m.ShowLogMessage, m.ShowAudioStatus)

	return style.Render(content)
}

// Focus sets focus on the settings panel
func (m *Model) Focus() {
	m.Focused = true
}

// Blur removes focus from the settings panel
func (m *Model) Blur() {
	m.Focused = false
}

// IsFocused returns whether the settings panel is focused
func (m Model) IsFocused() bool {
	return m.Focused
}