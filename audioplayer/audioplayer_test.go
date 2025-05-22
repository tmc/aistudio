package audioplayer

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	model := New()

	if model.State != Ready {
		t.Errorf("New() should set initial state to Ready, got: %v", model.State)
	}

	if model.Width != progressBarWidth {
		t.Errorf("New() should set width to progressBarWidth, got: %v", model.Width)
	}

	// Verify KeyMap is initialized
	if model.KeyMap.Play.Keys()[0] != "p" {
		t.Errorf("New() should initialize KeyMap with play key binding")
	}
}

func TestInit(t *testing.T) {
	model := New()
	cmd := model.Init()

	if cmd != nil {
		t.Error("Init() should return nil command")
	}
}

func TestConfigure(t *testing.T) {
	model := New()
	audioData := []byte("test audio data")
	text := "test text"
	messageIndex := 42

	model.Configure(audioData, text, messageIndex)

	if model.State != Ready {
		t.Errorf("Configure() should set state to Ready, got: %v", model.State)
	}

	if string(model.AudioData) != string(audioData) {
		t.Errorf("Configure() should set AudioData, got: %v", string(model.AudioData))
	}

	if model.Text != text {
		t.Errorf("Configure() should set Text, got: %v", model.Text)
	}

	if model.MessageIndex != messageIndex {
		t.Errorf("Configure() should set MessageIndex, got: %v", model.MessageIndex)
	}

	if model.ElapsedTime != 0 {
		t.Errorf("Configure() should reset ElapsedTime, got: %v", model.ElapsedTime)
	}

	expectedDuration := float64(len(audioData)) / 48000.0
	if model.TotalTime != expectedDuration {
		t.Errorf("Configure() should set TotalTime based on audio data length, got: %v, want: %v",
			model.TotalTime, expectedDuration)
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

func TestStateCheckers(t *testing.T) {
	model := New()

	// Test Ready state
	model.State = Ready
	if !model.IsReady() {
		t.Error("IsReady() should return true when state is Ready")
	}
	if model.IsPlaying() || model.IsPaused() || model.IsEnded() {
		t.Error("Other state checkers should return false when state is Ready")
	}

	// Test Playing state
	model.State = Playing
	if !model.IsPlaying() {
		t.Error("IsPlaying() should return true when state is Playing")
	}
	if model.IsReady() || model.IsPaused() || model.IsEnded() {
		t.Error("Other state checkers should return false when state is Playing")
	}

	// Test Paused state
	model.State = Paused
	if !model.IsPaused() {
		t.Error("IsPaused() should return true when state is Paused")
	}
	if model.IsReady() || model.IsPlaying() || model.IsEnded() {
		t.Error("Other state checkers should return false when state is Paused")
	}

	// Test Ended state
	model.State = Ended
	if !model.IsEnded() {
		t.Error("IsEnded() should return true when state is Ended")
	}
	if model.IsReady() || model.IsPlaying() || model.IsPaused() {
		t.Error("Other state checkers should return false when state is Ended")
	}
}

func TestFormatDuration(t *testing.T) {
	testCases := []struct {
		seconds  float64
		expected string
	}{
		{0, "00:00"},
		{1, "00:01"},
		{59, "00:59"},
		{60, "01:00"},
		{61, "01:01"},
		{3599, "59:59"},
		{3600, "60:00"},
		{-1, "00:00"}, // Negative values should be treated as 0
	}

	for _, tc := range testCases {
		result := formatDuration(tc.seconds)
		if result != tc.expected {
			t.Errorf("formatDuration(%v) = %v, want %v", tc.seconds, result, tc.expected)
		}
	}
}

func TestPlayMethod(t *testing.T) {
	model := New()

	// Test Play from Ready state
	cmd := model.Play()
	if model.State != Playing {
		t.Errorf("Play() from Ready state should set state to Playing, got: %v", model.State)
	}
	if cmd == nil {
		t.Error("Play() from Ready state should return a tick command")
	}

	// Test Play from Ended state
	model.State = Ended
	cmd = model.Play()
	if model.State != Playing {
		t.Errorf("Play() from Ended state should reset and set state to Playing, got: %v", model.State)
	}
	if cmd == nil {
		t.Error("Play() from Ended state should return a tick command")
	}

	// Test Play from Playing state (should do nothing)
	model.State = Playing
	cmd = model.Play()
	if model.State != Playing {
		t.Errorf("Play() from Playing state should keep state as Playing, got: %v", model.State)
	}
	// No assertion for cmd as it depends on the implementation
}

func TestPauseMethod(t *testing.T) {
	model := New()

	// Set up a playing state
	model.State = Playing
	now := time.Now()
	model.StartTime = now.Add(-5 * time.Second) // Started 5 seconds ago

	// Test Pause from Playing state
	cmd := model.Pause()
	if model.State != Paused {
		t.Errorf("Pause() from Playing state should set state to Paused, got: %v", model.State)
	}
	if cmd != nil {
		t.Error("Pause() from Playing state should return nil command")
	}
	if model.ElapsedTime < 4.9 || model.ElapsedTime > 5.1 {
		t.Errorf("Pause() should set ElapsedTime to approximately 5, got: %v", model.ElapsedTime)
	}

	// Test Pause from Paused state (should resume)
	cmd = model.Pause()
	if model.State != Playing {
		t.Errorf("Pause() from Paused state should set state to Playing, got: %v", model.State)
	}
	if cmd == nil {
		t.Error("Pause() from Paused state should return a tick command")
	}

	// Test Pause from other states
	model.State = Ready
	cmd = model.Pause()
	if model.State != Ready {
		t.Errorf("Pause() from Ready state should not change state, got: %v", model.State)
	}
	if cmd != nil {
		t.Error("Pause() from Ready state should return nil command")
	}
}

func TestStopMethod(t *testing.T) {
	model := New()

	// Setup
	model.TotalTime = 10.0

	// Test Stop from Playing state
	model.State = Playing
	cmd := model.Stop()
	if model.State != Ended {
		t.Errorf("Stop() from Playing state should set state to Ended, got: %v", model.State)
	}
	if cmd == nil {
		t.Error("Stop() from Playing state should return a command")
	}
	if model.ElapsedTime != model.TotalTime {
		t.Errorf("Stop() should set ElapsedTime to TotalTime, got: %v, want: %v",
			model.ElapsedTime, model.TotalTime)
	}

	// Test Stop from Paused state
	model.State = Paused
	model.ElapsedTime = 5.0
	cmd = model.Stop()
	if model.State != Ended {
		t.Errorf("Stop() from Paused state should set state to Ended, got: %v", model.State)
	}
	if cmd == nil {
		t.Error("Stop() from Paused state should return a command")
	}
	if model.ElapsedTime != model.TotalTime {
		t.Errorf("Stop() should set ElapsedTime to TotalTime, got: %v, want: %v",
			model.ElapsedTime, model.TotalTime)
	}

	// Test Stop from other states
	model.State = Ready
	cmd = model.Stop()
	if model.State != Ready {
		t.Errorf("Stop() from Ready state should not change state, got: %v", model.State)
	}
	if cmd != nil {
		t.Error("Stop() from Ready state should return nil command")
	}
}

func TestUpdatePlayMsg(t *testing.T) {
	model := New()

	// Test update with PlayMsg when Ready
	newModel, cmd := model.Update(PlayMsg{})
	if newModel.State != Playing {
		t.Errorf("Update(PlayMsg{}) from Ready state should set state to Playing, got: %v", newModel.State)
	}
	if cmd == nil {
		t.Error("Update(PlayMsg{}) from Ready state should return a tick command")
	}

	// Test update with PlayMsg when already Playing (should do nothing)
	model.State = Playing
	newModel, cmd = model.Update(PlayMsg{})
	if newModel.State != Playing {
		t.Errorf("Update(PlayMsg{}) from Playing state should keep state as Playing, got: %v", newModel.State)
	}
}

func TestUpdatePauseMsg(t *testing.T) {
	model := New()

	// Set up a playing state
	model.State = Playing
	now := time.Now()
	model.StartTime = now.Add(-5 * time.Second) // Started 5 seconds ago

	// Test update with PauseMsg when Playing
	newModel, cmd := model.Update(PauseMsg{})
	if newModel.State != Paused {
		t.Errorf("Update(PauseMsg{}) from Playing state should set state to Paused, got: %v", newModel.State)
	}
	if cmd != nil {
		t.Error("Update(PauseMsg{}) from Playing state should return nil command")
	}

	// Test update with PauseMsg when Paused
	model.State = Paused
	model.ElapsedTime = 5.0
	newModel, cmd = model.Update(PauseMsg{})
	if newModel.State != Playing {
		t.Errorf("Update(PauseMsg{}) from Paused state should set state to Playing, got: %v", newModel.State)
	}
	if cmd == nil {
		t.Error("Update(PauseMsg{}) from Paused state should return a tick command")
	}
}

func TestUpdateStopMsg(t *testing.T) {
	model := New()

	// Setup
	model.State = Playing
	model.TotalTime = 10.0

	// Test update with StopMsg
	newModel, cmd := model.Update(StopMsg{})
	if newModel.State != Ended {
		t.Errorf("Update(StopMsg{}) should set state to Ended, got: %v", newModel.State)
	}
	if cmd == nil {
		t.Error("Update(StopMsg{}) should return a command")
	}
	if newModel.ElapsedTime != newModel.TotalTime {
		t.Errorf("Update(StopMsg{}) should set ElapsedTime to TotalTime, got: %v, want: %v",
			newModel.ElapsedTime, newModel.TotalTime)
	}
}

func TestUpdateTickMsg(t *testing.T) {
	model := New()

	// Set up a playing state
	model.State = Playing
	model.TotalTime = 10.0
	model.StartTime = time.Now().Add(-5 * time.Second) // Started 5 seconds ago

	// Test update with TickMsg when Playing but not yet completed
	newModel, cmd := model.Update(TickMsg(time.Now()))
	if newModel.State != Playing {
		t.Errorf("Update(TickMsg) with ongoing playback should keep state as Playing, got: %v", newModel.State)
	}
	if cmd == nil {
		t.Error("Update(TickMsg) with ongoing playback should return a tick command")
	}
	if newModel.ElapsedTime < 4.9 || newModel.ElapsedTime > 5.1 {
		t.Errorf("Update(TickMsg) should update ElapsedTime to approximately 5, got: %v", newModel.ElapsedTime)
	}

	// Test update with TickMsg when playback should complete
	model.TotalTime = 4.0                              // Set total time less than elapsed time
	model.StartTime = time.Now().Add(-5 * time.Second) // Started 5 seconds ago

	newModel, cmd = model.Update(TickMsg(time.Now()))
	if newModel.State != Ended {
		t.Errorf("Update(TickMsg) with completed playback should set state to Ended, got: %v", newModel.State)
	}
	if cmd == nil {
		t.Error("Update(TickMsg) with completed playback should return a command")
	}
	if newModel.ElapsedTime != newModel.TotalTime {
		t.Errorf("Update(TickMsg) with completed playback should set ElapsedTime to TotalTime, got: %v, want: %v",
			newModel.ElapsedTime, newModel.TotalTime)
	}

	// Test update with TickMsg when not Playing
	model.State = Ready
	newModel, cmd = model.Update(TickMsg(time.Now()))
	if newModel.State != Ready {
		t.Errorf("Update(TickMsg) when not Playing should not change state, got: %v", newModel.State)
	}
	if cmd != nil {
		t.Error("Update(TickMsg) when not Playing should return nil command")
	}
}

func TestView(t *testing.T) {
	model := New()

	// Test View in Ready state
	model.State = Ready
	view := model.View()
	if !contains(view, "🔈") || !contains(view, "[P]lay") {
		t.Errorf("View() in Ready state should contain ready icon and play help, got: %v", view)
	}

	// Test View in Playing state
	model.State = Playing
	model.ElapsedTime = 5.0
	model.TotalTime = 10.0
	view = model.View()
	if !contains(view, "🔊") || !contains(view, "[Space]pause") {
		t.Errorf("View() in Playing state should contain playing icon and pause help, got: %v", view)
	}
	if !contains(view, "00:05 / 00:10") {
		t.Errorf("View() should format time correctly, got: %v", view)
	}

	// Test View in Paused state
	model.State = Paused
	view = model.View()
	if !contains(view, "⏸️") || !contains(view, "[Space]resume") {
		t.Errorf("View() in Paused state should contain pause icon and resume help, got: %v", view)
	}

	// Test View in Ended state
	model.State = Ended
	view = model.View()
	if !contains(view, "✓") || !contains(view, "[R]eplay") {
		t.Errorf("View() in Ended state should contain check icon and replay help, got: %v", view)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return s != "" && substr != "" && s != substr && len(s) >= len(substr) && s[:len(substr)] == substr || len(s) >= len(substr) && s[len(s)-len(substr):] == substr || (func() bool {
		for i := 0; i < len(s); i++ {
			if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
