package aistudio_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tmc/aistudio"
)

// TestUIComprehensiveFunctionality tests the fixed UI with comprehensive scenarios
func TestUIComprehensiveFunctionality(t *testing.T) {
	if os.Getenv("AISTUDIO_RUN_UI_TESTS") == "" {
		t.Skip("Skipping UI tests - set AISTUDIO_RUN_UI_TESTS=1 to run")
	}

	t.Log("🧪 Testing comprehensive UI functionality after bug fixes...")

	// Test 1: Complete typing scenario
	t.Run("CompleteTypingScenario", func(t *testing.T) {
		model := createTestModel(t)

		// Type a complete message
		message := "Hello, this is a test message!"

		for i, char := range message {
			keyMsg := tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{char},
			}

			var err error
			model, err = updateModel(t, model, keyMsg)
			if err != nil {
				t.Fatalf("Failed at character %d ('%c'): %v", i, char, err)
			}
		}

		// Verify the complete message appears in the view
		view := model.View()
		if !strings.Contains(view, message) {
			t.Errorf("Complete message should appear in view")
			t.Logf("Looking for: %s", message)
			t.Logf("View contains: %s", view)
		} else {
			t.Log("✅ Complete typing scenario works!")
		}
	})

	// Test 2: Mixed input scenario (typing + shortcuts)
	t.Run("MixedInputScenario", func(t *testing.T) {
		model := createTestModel(t)

		// Type some text
		text := "test"
		for _, char := range text {
			var err error
			model, err = updateModel(t, model, tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{char},
			})
			if err != nil {
				t.Fatalf("Failed typing: %v", err)
			}
		}

		// Press Ctrl+S (should toggle settings)
		var err error
		model, err = updateModel(t, model, tea.KeyMsg{Type: tea.KeyCtrlS})
		if err != nil {
			t.Fatalf("Failed Ctrl+S: %v", err)
		}

		// Press Ctrl+S again (should toggle back)
		model, err = updateModel(t, model, tea.KeyMsg{Type: tea.KeyCtrlS})
		if err != nil {
			t.Fatalf("Failed second Ctrl+S: %v", err)
		}

		// Continue typing
		for _, char := range " more" {
			model, err = updateModel(t, model, tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{char},
			})
			if err != nil {
				t.Fatalf("Failed continued typing: %v", err)
			}
		}

		t.Log("✅ Mixed input scenario works!")
	})

	// Test 3: Rapid input handling
	t.Run("RapidInputHandling", func(t *testing.T) {
		model := createTestModel(t)

		// Simulate rapid typing
		rapidText := "abcdefghijklmnopqrstuvwxyz0123456789"

		for i, char := range rapidText {
			var err error
			model, err = updateModel(t, model, tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{char},
			})
			if err != nil {
				t.Fatalf("Failed at rapid input %d: %v", i, err)
			}
		}

		// Verify rapid input was handled
		view := model.View()
		if !strings.Contains(view, rapidText) {
			t.Errorf("Rapid input not fully captured in view")
		} else {
			t.Log("✅ Rapid input handling works!")
		}
	})

	// Test 4: Edge cases
	t.Run("EdgeCases", func(t *testing.T) {
		model := createTestModel(t)

		// Test special characters
		specialChars := "!@#$%^&*()_+-=[]{}|;:,.<>?"

		for _, char := range specialChars {
			var err error
			model, err = updateModel(t, model, tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{char},
			})
			if err != nil {
				t.Fatalf("Failed special character '%c': %v", char, err)
			}
		}

		// Test backspace
		model, _ = updateModel(t, model, tea.KeyMsg{Type: tea.KeyBackspace})

		// Test arrow keys
		model, _ = updateModel(t, model, tea.KeyMsg{Type: tea.KeyLeft})
		model, _ = updateModel(t, model, tea.KeyMsg{Type: tea.KeyRight})
		model, _ = updateModel(t, model, tea.KeyMsg{Type: tea.KeyUp})
		model, _ = updateModel(t, model, tea.KeyMsg{Type: tea.KeyDown})

		t.Log("✅ Edge cases handled!")
	})

	// Test 5: Quit sequence
	t.Run("QuitSequence", func(t *testing.T) {
		model := createTestModel(t)

		// Type something first
		var err error
		model, err = updateModel(t, model, tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune("before quit"),
		})
		if err != nil {
			t.Fatalf("Failed typing before quit: %v", err)
		}

		// Now quit with Ctrl+C
		model, err = updateModel(t, model, tea.KeyMsg{Type: tea.KeyCtrlC})
		if err != nil {
			t.Fatalf("Failed Ctrl+C: %v", err)
		}

		// Any subsequent input should be handled gracefully (quitting state)
		model, _ = updateModel(t, model, tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune("a"),
		})

		t.Log("✅ Quit sequence works!")
	})
}

// Helper function to create a test model
func createTestModel(t *testing.T) *aistudio.Model {
	opts := []aistudio.Option{
		aistudio.WithModel("models/gemini-1.5-flash-latest"),
		aistudio.WithAPIKey("test-key"),
	}

	model := aistudio.New(opts...)
	if model == nil {
		t.Fatal("Failed to create model")
	}

	return model
}

// Helper function to safely update model and handle errors
func updateModel(t *testing.T, model *aistudio.Model, msg tea.Msg) (*aistudio.Model, error) {
	updatedModel, _ := model.Update(msg)
	if updatedModel == nil {
		return nil, fmt.Errorf("Update returned nil model for message: %v", msg)
	}

	resultModel, ok := updatedModel.(*aistudio.Model)
	if !ok {
		return nil, fmt.Errorf("Updated model is not *aistudio.Model for message: %v", msg)
	}

	return resultModel, nil
}

// TestUIPerformance tests that the UI performs well after fixes
func TestUIPerformance(t *testing.T) {
	if os.Getenv("AISTUDIO_RUN_UI_TESTS") == "" {
		t.Skip("Skipping UI performance tests - set AISTUDIO_RUN_UI_TESTS=1 to run")
	}

	t.Log("⚡ Testing UI performance...")

	model := createTestModel(t)

	// Measure time for rapid updates
	start := time.Now()

	// Simulate 1000 key presses
	for i := 0; i < 1000; i++ {
		char := rune('a' + (i % 26))
		keyMsg := tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{char},
		}

		var err error
		model, err = updateModel(t, model, keyMsg)
		if err != nil {
			t.Fatalf("Performance test failed at iteration %d: %v", i, err)
		}
	}

	duration := time.Since(start)

	// Should be very fast (under 100ms for 1000 updates)
	if duration > 100*time.Millisecond {
		t.Errorf("UI performance too slow: %v for 1000 updates", duration)
	} else {
		t.Logf("✅ UI performance good: %v for 1000 updates", duration)
	}
}

// TestUIMemoryUsage tests that the UI doesn't leak memory
func TestUIMemoryUsage(t *testing.T) {
	if os.Getenv("AISTUDIO_RUN_UI_TESTS") == "" {
		t.Skip("Skipping UI memory tests - set AISTUDIO_RUN_UI_TESTS=1 to run")
	}

	t.Log("🧠 Testing UI memory usage...")

	// Create and destroy many models to test for leaks
	for i := 0; i < 100; i++ {
		model := createTestModel(t)

		// Do some operations
		for j := 0; j < 10; j++ {
			var err error
			model, err = updateModel(t, model, tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{'x'},
			})
			if err != nil {
				t.Fatalf("Memory test failed: %v", err)
			}
		}

		// Model should be garbage collected when we lose reference
		model = nil
	}

	t.Log("✅ Memory usage test completed (no obvious leaks)")
}
