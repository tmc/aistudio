package settings

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNew(t *testing.T) {
	model := New()

	// Check default values
	if model.CurrentModel != "gemini-pro" {
		t.Errorf("New() CurrentModel = %s, want gemini-pro", model.CurrentModel)
	}

	if model.CurrentVoice != "Default" {
		t.Errorf("New() CurrentVoice = %s, want Default", model.CurrentVoice)
	}

	if !model.AudioEnabled {
		t.Error("New() AudioEnabled = false, want true")
	}

	if !model.ShowLogo {
		t.Error("New() ShowLogo = false, want true")
	}

	if model.ShowLogMessage {
		t.Error("New() ShowLogMessage = true, want false")
	}

	if !model.ShowAudioStatus {
		t.Error("New() ShowAudioStatus = false, want true")
	}

	if model.Focused {
		t.Error("New() Focused = true, want false")
	}
}

func TestInit(t *testing.T) {
	model := New()
	cmd := model.Init()

	if cmd != nil {
		t.Error("Init() returned non-nil command")
	}
}

func TestFocusAndBlur(t *testing.T) {
	model := New()

	// Test Focus
	model.Focus()
	if !model.Focused {
		t.Error("Focus() should set Focused to true")
	}
	if !model.IsFocused() {
		t.Error("IsFocused() should return true after Focus()")
	}

	// Test Blur
	model.Blur()
	if model.Focused {
		t.Error("Blur() should set Focused to false")
	}
	if model.IsFocused() {
		t.Error("IsFocused() should return false after Blur()")
	}
}

func TestView(t *testing.T) {
	model := New()

	// Test view when not focused
	view := model.View()
	if view != "" {
		t.Errorf("View() when not focused should return empty string, got: %s", view)
	}

	// Test view when focused
	model.Focus()
	view = model.View()
	if view == "" {
		t.Error("View() when focused should not return empty string")
	}

	// Check that view contains important settings
	if !strings.Contains(view, "Settings") {
		t.Error("View() should contain 'Settings' title")
	}

	if !strings.Contains(view, "Model: gemini-pro") {
		t.Error("View() should contain model information")
	}

	if !strings.Contains(view, "Voice: Default") {
		t.Error("View() should contain voice information")
	}

	if !strings.Contains(view, "Audio Enabled: true") {
		t.Error("View() should contain audio enabled status")
	}

	if !strings.Contains(view, "Press ESC to close") {
		t.Error("View() should contain exit instructions")
	}
}

func TestUpdate(t *testing.T) {
	model := New()

	// Test window size message
	newModel, cmd := model.Update(tea.WindowSizeMsg{Width: 120, Height: 80})
	if newModel.Width != 40 { // Width should be 1/3 of window width
		t.Errorf("Update(WindowSizeMsg) Width = %d, want 40", newModel.Width)
	}
	if newModel.Height != 80 {
		t.Errorf("Update(WindowSizeMsg) Height = %d, want 80", newModel.Height)
	}
	if cmd != nil {
		t.Error("Update(WindowSizeMsg) returned non-nil command")
	}

	// Test key message when not focused
	model.Focused = false
	newModel, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if newModel.Focused {
		t.Error("Update(KeyMsg) when not focused should not change focus state")
	}
	if cmd != nil {
		t.Error("Update(KeyMsg) when not focused returned non-nil command")
	}

	// Test key message when focused
	model.Focus()
	newModel, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if newModel.Focused {
		t.Error("Update(KeyMsg{Esc}) when focused should set Focused to false")
	}
	if cmd != nil {
		t.Error("Update(KeyMsg{Esc}) returned non-nil command")
	}

	// Test other key message when focused
	model.Focus()
	newModel, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !newModel.Focused {
		t.Error("Update(KeyMsg{Enter}) when focused should not change focus state")
	}
	if cmd != nil {
		t.Error("Update(KeyMsg{Enter}) returned non-nil command")
	}
}
