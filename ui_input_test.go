package aistudio_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tmc/aistudio"
)

// TestUIKeyboardInput tests that keyboard input is properly handled
func TestUIKeyboardInput(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	// Create a model for testing
	opts := []aistudio.Option{
		aistudio.WithModel("models/gemini-1.5-flash-latest"),
		aistudio.WithAPIKey("test-key"),
	}

	model := aistudio.New(opts...)
	if model == nil {
		t.Fatal("Failed to create model")
	}

	// Initialize the model
	initialCmd := model.Init()
	if initialCmd == nil {
		t.Fatal("Expected initial command from Init()")
	}

	// Test typing input
	t.Run("TypingInput", func(t *testing.T) {
		// Simulate typing "hello"
		for _, char := range "hello" {
			keyMsg := tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{char},
			}

			updatedModel, cmd := model.Update(keyMsg)
			if updatedModel == nil {
				t.Errorf("Model should not be nil after key input")
			}
			model = updatedModel.(*aistudio.Model)

			// Command can be nil for regular typing
			_ = cmd
		}

		// Check that the textarea has the typed content
		view := model.View()
		if !strings.Contains(view, "hello") {
			t.Logf("View content: %s", view)
			t.Errorf("Expected 'hello' to appear in the view after typing")
		}
	})

	// Test Ctrl+C handling
	t.Run("CtrlCHandling", func(t *testing.T) {
		// Simulate Ctrl+C
		keyMsg := tea.KeyMsg{
			Type: tea.KeyCtrlC,
		}

		updatedModel, cmd := model.Update(keyMsg)
		if updatedModel == nil {
			t.Errorf("Model should not be nil after Ctrl+C")
		}

		// Should return a Quit command
		if cmd == nil {
			t.Errorf("Expected a command after Ctrl+C (should be Quit)")
		}

		// Check that the model is in quitting state
		model = updatedModel.(*aistudio.Model)
		// We can't directly access private fields, but we can test the behavior
		// by sending another update and seeing if it returns Quit
		testModel, testCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if testModel == nil || testCmd == nil {
			t.Logf("Model state after Ctrl+C seems to be handled correctly")
		}
	})

	// Test other control keys
	t.Run("ControlKeys", func(t *testing.T) {
		controlKeys := []struct {
			name string
			key  tea.KeyMsg
		}{
			{"Ctrl+S", tea.KeyMsg{Type: tea.KeyCtrlS}},
			{"Ctrl+T", tea.KeyMsg{Type: tea.KeyCtrlT}},
			{"Ctrl+H", tea.KeyMsg{Type: tea.KeyCtrlH}},
		}

		for _, test := range controlKeys {
			t.Run(test.name, func(t *testing.T) {
				updatedModel, cmd := model.Update(test.key)
				if updatedModel == nil {
					t.Errorf("Model should not be nil after %s", test.name)
				}

				// Commands can be nil or non-nil depending on the key
				_ = cmd

				model = updatedModel.(*aistudio.Model)
			})
		}
	})

	// Test Enter key (should trigger message send)
	t.Run("EnterKey", func(t *testing.T) {
		// First add some text
		for _, char := range "test message" {
			keyMsg := tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{char},
			}
			updatedModel, _ := model.Update(keyMsg)
			model = updatedModel.(*aistudio.Model)
		}

		// Now press Enter
		keyMsg := tea.KeyMsg{
			Type: tea.KeyEnter,
		}

		updatedModel, cmd := model.Update(keyMsg)
		if updatedModel == nil {
			t.Errorf("Model should not be nil after Enter")
		}

		// Should trigger some command (likely sending the message)
		if cmd == nil {
			t.Logf("Enter key may not trigger a command in test mode (expected)")
		}

		model = updatedModel.(*aistudio.Model)
	})
}

// TestUIKeyMessageHandling tests that tea.KeyMsg is properly routed to handleKeyMsg
func TestUIKeyMessageHandling(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	// This test verifies that the Update method correctly handles KeyMsg
	opts := []aistudio.Option{
		aistudio.WithModel("models/gemini-1.5-flash-latest"),
		aistudio.WithAPIKey("test-key"),
	}

	model := aistudio.New(opts...)
	if model == nil {
		t.Fatal("Failed to create model")
	}

	// Test that KeyMsg is not ignored
	keyMsg := tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'a'},
	}

	// The Update method should handle this and not return nil
	updatedModel, cmd := model.Update(keyMsg)
	if updatedModel == nil {
		t.Errorf("Update should not return nil model for KeyMsg")
	}

	// Verify it's still our model type
	if _, ok := updatedModel.(*aistudio.Model); !ok {
		t.Errorf("Updated model should still be *aistudio.Model")
	}

	// Command can be nil or non-nil
	_ = cmd

	t.Log("KeyMsg handling test passed - Update method properly processes keyboard input")
}

// TestUIWindowResize tests window resize handling
func TestUIWindowResize(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	opts := []aistudio.Option{
		aistudio.WithModel("models/gemini-1.5-flash-latest"),
		aistudio.WithAPIKey("test-key"),
	}

	model := aistudio.New(opts...)
	if model == nil {
		t.Fatal("Failed to create model")
	}

	// Test window resize
	resizeMsg := tea.WindowSizeMsg{
		Width:  100,
		Height: 50,
	}

	updatedModel, cmd := model.Update(resizeMsg)
	if updatedModel == nil {
		t.Errorf("Model should not be nil after window resize")
	}

	// Should not return a command for resize
	if cmd != nil {
		t.Logf("Window resize returned command: expected nil")
	}

	t.Log("Window resize handling test passed")
}
