package audioplayer

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Constants for the audio player UI
const (
	progressBarWidth = 30
)

// State represents the state of the audio player
type State int

const (
	// Ready means the audio is ready to play
	Ready State = iota
	// Playing means the audio is currently playing
	Playing
	// Paused means the audio is paused
	Paused
	// Ended means the audio has finished playing
	Ended
)

// KeyMap defines the keybindings for the audio player
type KeyMap struct {
	Play  key.Binding
	Pause key.Binding
	Stop  key.Binding
}

// DefaultKeyMap returns a set of default keybindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Play: key.NewBinding(
			key.WithKeys("p", "P"),
			key.WithHelp("p", "play"),
		),
		Pause: key.NewBinding(
			key.WithKeys("space"),
			key.WithHelp("space", "pause"),
		),
		Stop: key.NewBinding(
			key.WithKeys("s", "S"),
			key.WithHelp("s", "stop"),
		),
	}
}

// PlayMsg is a message that tells the audio player to start playing
type PlayMsg struct{}

// PauseMsg is a message that tells the audio player to pause
type PauseMsg struct{}

// StopMsg is a message that tells the audio player to stop
type StopMsg struct{}

// TickMsg is a message that updates the audio player's progress
type TickMsg time.Time

// AudioEndedMsg is a message that indicates the audio has finished playing
type AudioEndedMsg struct {
	MessageIndex int
}

// Model represents the audio player state
type Model struct {
	KeyMap       KeyMap
	State        State
	ElapsedTime  float64
	TotalTime    float64
	Width        int
	MessageIndex int
	Text         string
	StartTime    time.Time
	AudioData    []byte
	Focused      bool
}

// New creates a new audio player model
func New() Model {
	return Model{
		KeyMap: DefaultKeyMap(),
		State:  Ready,
		Width:  progressBarWidth,
	}
}

// Init initializes the audio player model
func (m Model) Init() tea.Cmd {
	return nil
}

// tickCmd returns a command that will send a tick message after a delay
func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// Update handles updating the audio player model
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg.(type) {
	case PlayMsg:
		if m.State != Playing {
			m.State = Playing
			m.StartTime = time.Now()
			return m, tickCmd()
		}
	
	case PauseMsg:
		if m.State == Playing {
			m.State = Paused
			// Save the elapsed time when pausing
			m.ElapsedTime = time.Since(m.StartTime).Seconds()
			return m, nil
		} else if m.State == Paused {
			m.State = Playing
			// Adjust the start time to account for the pause
			m.StartTime = time.Now().Add(-time.Duration(m.ElapsedTime * float64(time.Second)))
			return m, tickCmd()
		}
	
	case StopMsg:
		m.State = Ended
		m.ElapsedTime = m.TotalTime
		return m, func() tea.Msg {
			return AudioEndedMsg{MessageIndex: m.MessageIndex}
		}
	}

	// Handle TickMsg separately as we need to use the value
	if tickMsg, ok := msg.(TickMsg); ok {
		if m.State == Playing {
			if m.TotalTime > 0 && !m.StartTime.IsZero() {
				m.ElapsedTime = time.Since(m.StartTime).Seconds()
				
				// Check if the audio has finished
				if m.ElapsedTime >= m.TotalTime {
					m.ElapsedTime = m.TotalTime
					m.State = Ended
					return m, func() tea.Msg {
						return AudioEndedMsg{MessageIndex: m.MessageIndex}
					}
				}
			}
			
			// We use tickMsg just to make the compiler happy
			_ = tickMsg
			return m, tickCmd()
		}
	}

	return m, nil
}

// View renders the audio player UI
func (m Model) View() string {
	var audioLine strings.Builder

	// Determine the icon based on the state
	audioIcon := "❓"
	switch m.State {
	case Playing:
		audioIcon = "🔊" // Green playing icon
	case Paused:
		audioIcon = "⏸️" // Pause icon
	case Ended:
		audioIcon = "✓" // Gray check for played
	case Ready:
		audioIcon = "🔈" // Magenta speaker
	}

	// Format timestamps
	elapsedDurationStr := formatDuration(m.ElapsedTime)
	totalDurationStr := formatDuration(m.TotalTime)
	timestampStr := fmt.Sprintf("%s / %s", elapsedDurationStr, totalDurationStr)

	// Calculate progress bar
	progressBar := strings.Repeat("╌", m.Width) // Default empty bar
	
	if m.State == Playing || m.State == Paused || m.State == Ended {
		progress := 0.0
		if m.TotalTime > 0 {
			progress = m.ElapsedTime / m.TotalTime
		}
		progress = math.Min(1.0, math.Max(0.0, progress)) // Clamp progress [0, 1]
		filledWidth := int(progress * float64(m.Width))
		emptyWidth := m.Width - filledWidth
		progressBar = strings.Repeat("━", filledWidth) + strings.Repeat("╌", emptyWidth)
	}

	// Help text based on state
	helpText := ""
	switch m.State {
	case Ready:
		helpText = "[P]lay"
	case Playing:
		helpText = "[Space]pause"
	case Paused:
		helpText = "[Space]resume"
	case Ended:
		helpText = "[R]eplay"
	}

	// Assemble the audio line
	audioLine.WriteString(audioIcon)
	audioLine.WriteString(" ")
	audioLine.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(timestampStr))
	audioLine.WriteString(" ")
	audioLine.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render(progressBar))
	
	if helpText != "" {
		audioLine.WriteString(" ")
		audioLine.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Render(helpText))
	}

	return audioLine.String()
}

// Configure sets up the audio player for a specific audio chunk
func (m *Model) Configure(audioData []byte, text string, messageIndex int) {
	m.AudioData = audioData
	m.Text = text
	m.MessageIndex = messageIndex
	m.State = Ready
	m.ElapsedTime = 0
	m.TotalTime = float64(len(audioData)) / 48000.0 // Assuming 48kHz sample rate
}

// Play starts playback or resets if already ended
func (m *Model) Play() tea.Cmd {
	if m.State == Ended {
		m.State = Ready
		m.ElapsedTime = 0
	}
	
	if m.State == Ready {
		m.State = Playing
		m.StartTime = time.Now()
		return tickCmd()
	}
	
	return nil
}

// Pause toggles between play and pause
func (m *Model) Pause() tea.Cmd {
	if m.State == Playing {
		m.State = Paused
		m.ElapsedTime = time.Since(m.StartTime).Seconds()
		return nil
	} else if m.State == Paused {
		m.State = Playing
		m.StartTime = time.Now().Add(-time.Duration(m.ElapsedTime * float64(time.Second)))
		return tickCmd()
	}
	return nil
}

// Stop ends playback
func (m *Model) Stop() tea.Cmd {
	if m.State == Playing || m.State == Paused {
		m.State = Ended
		m.ElapsedTime = m.TotalTime
		return func() tea.Msg {
			return AudioEndedMsg{MessageIndex: m.MessageIndex}
		}
	}
	return nil
}

// Focus sets focus on the audio player
func (m *Model) Focus() {
	m.Focused = true
}

// Blur removes focus from the audio player
func (m *Model) Blur() {
	m.Focused = false
}

// IsFocused returns whether the audio player is focused
func (m Model) IsFocused() bool {
	return m.Focused
}

// IsPlaying returns whether the audio is playing
func (m Model) IsPlaying() bool {
	return m.State == Playing
}

// IsPaused returns whether the audio is paused
func (m Model) IsPaused() bool {
	return m.State == Paused
}

// IsEnded returns whether the audio has ended
func (m Model) IsEnded() bool {
	return m.State == Ended
}

// IsReady returns whether the audio is ready to play
func (m Model) IsReady() bool {
	return m.State == Ready
}

// formatDuration formats a duration in seconds as MM:SS
func formatDuration(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	
	minutes := int(seconds) / 60
	remainingSeconds := int(seconds) % 60
	return fmt.Sprintf("%02d:%02d", minutes, remainingSeconds)
}