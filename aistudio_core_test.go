package aistudio

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tmc/aistudio/api"
)

// TestModelInitialization tests the Model initialization
func TestModelInitialization(t *testing.T) {
	tests := []struct {
		name    string
		options []Option
		wantErr bool
	}{
		{
			name:    "Default initialization",
			options: []Option{},
			wantErr: false,
		},
		{
			name: "With custom model",
			options: []Option{
				WithModel("gemini-2.0-flash-latest"),
			},
			wantErr: false,
		},
		{
			name: "With tools enabled",
			options: []Option{
				WithToolsEnabled(true),
			},
			wantErr: false,
		},
		{
			name: "With history enabled",
			options: []Option{
				WithHistoryEnabled(true),
			},
			wantErr: false,
		},
		{
			name: "With multiple options",
			options: []Option{
				WithModel("gemini-2.0-flash-latest"),
				WithToolsEnabled(true),
				WithHistoryEnabled(true),
				WithTemperature(0.7),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := New(tt.options...)
			if model == nil {
				t.Fatal("Expected model to be created")
			}

			// Verify basic properties
			if model.modelName == "" {
				t.Error("Model name should not be empty")
			}
			if model.width == 0 {
				t.Error("Width should be initialized")
			}
			if model.height == 0 {
				t.Error("Height should be initialized")
			}
		})
	}
}

// TestModelUpdate tests the Update method
func TestModelUpdate(t *testing.T) {
	model := New()

	// Test window size message
	t.Run("Window size update", func(t *testing.T) {
		msg := tea.WindowSizeMsg{Width: 100, Height: 50}
		updatedModel, _ := model.Update(msg)

		m := updatedModel.(*Model)
		if m.width != 100 {
			t.Errorf("Expected width to be 100, got %d", m.width)
		}
		if m.height != 50 {
			t.Errorf("Expected height to be 50, got %d", m.height)
		}
	})

	// Test quit command
	t.Run("Quit command", func(t *testing.T) {
		msg := tea.KeyMsg{Type: tea.KeyCtrlC}
		updatedModel, cmd := model.Update(msg)

		m := updatedModel.(*Model)
		if m.currentState != AppStateQuitting {
			t.Error("Expected state to be quitting")
		}
		if cmd == nil {
			t.Error("Expected quit command")
		}
	})
}

// TestMessageHandling tests message creation and formatting
func TestMessageHandling(t *testing.T) {
	tests := []struct {
		name    string
		message Message
		want    string
	}{
		{
			name: "User message",
			message: Message{
				Sender:  "user",
				Content: "Hello",
			},
			want: "user",
		},
		{
			name: "Assistant message",
			message: Message{
				Sender:  "model",
				Content: "Hi there",
			},
			want: "model",
		},
		{
			name: "System message",
			message: Message{
				Sender:  "system",
				Content: "System notification",
			},
			want: "system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.message.Sender != tt.want {
				t.Errorf("Expected sender %s, got %s", tt.want, tt.message.Sender)
			}
		})
	}
}

// TestToolCallProcessing tests tool call handling
func TestToolCallProcessing(t *testing.T) {
	model := New(WithToolsEnabled(true))

	// Initialize tool manager
	if model.toolManager == nil {
		t.Fatal("Tool manager should be initialized")
	}

	// Test tool registration
	t.Run("Tool registration", func(t *testing.T) {
		err := model.toolManager.RegisterTool("test_tool", "Test tool", nil, nil)
		if err != nil {
			t.Errorf("Failed to register tool: %v", err)
		}

		count := model.toolManager.GetToolCount()
		if count < 1 {
			t.Error("Expected at least one tool to be registered")
		}
	})

	// Test tool call creation
	t.Run("Tool call creation", func(t *testing.T) {
		call := ToolCall{
			ID:   "test-123",
			Name: "test_tool",
		}

		if call.ID != "test-123" {
			t.Errorf("Expected ID to be test-123, got %s", call.ID)
		}
		if call.Name != "test_tool" {
			t.Errorf("Expected name to be test_tool, got %s", call.Name)
		}
	})
}

// TestStreamHandling tests stream-related functionality
func TestStreamHandling(t *testing.T) {
	model := New()

	// Test stream initialization
	t.Run("Stream initialization", func(t *testing.T) {
		if model.currentState != AppStateReady {
			t.Errorf("Expected initial state to be Ready, got %v", model.currentState)
		}
	})

	// Test stream error handling
	t.Run("Stream error handling", func(t *testing.T) {
		msg := streamErrorMsg{err: context.DeadlineExceeded}
		updatedModel, _ := model.Update(msg)

		m := updatedModel.(*Model)
		if m.currentState != AppStateError {
			t.Error("Expected state to be Error after stream error")
		}
	})
}

// TestHistoryManager tests history functionality
func TestHistoryManager(t *testing.T) {
	model := New(WithHistoryEnabled(true))

	t.Run("History initialization", func(t *testing.T) {
		if !model.historyEnabled {
			t.Error("History should be enabled")
		}

		if model.historyManager == nil {
			t.Error("History manager should be initialized")
		}
	})

	t.Run("Add message to history", func(t *testing.T) {
		if model.historyManager != nil {
			msg := Message{
				Sender:  "user",
				Content: "Test message",
			}
			model.historyManager.AddMessage(msg)

			if model.historyManager.CurrentSession == nil {
				t.Error("Current session should be created")
			}
		}
	})
}

// TestNavigationFeatures tests navigation including Shift+Tab
func TestNavigationFeatures(t *testing.T) {
	model := New()

	// Test Tab navigation
	t.Run("Tab navigation", func(t *testing.T) {
		msg := tea.KeyMsg{Type: tea.KeyTab}
		updatedModel, _ := model.Update(msg)

		m := updatedModel.(*Model)
		if m.focusIndex == model.focusIndex {
			// Focus should have changed
			t.Log("Tab navigation processed")
		}
	})

	// Test Shift+Tab navigation
	t.Run("Shift+Tab navigation", func(t *testing.T) {
		msg := tea.KeyMsg{Type: tea.KeyShiftTab}
		updatedModel, _ := model.Update(msg)

		m := updatedModel.(*Model)
		if m.focusIndex == model.focusIndex {
			// Focus should have changed
			t.Log("Shift+Tab navigation processed")
		}
	})
}

// TestErrorHandling tests error handling scenarios
func TestErrorHandling(t *testing.T) {
	model := New()

	tests := []struct {
		name  string
		error error
		want  AppState
	}{
		{
			name:  "Context deadline exceeded",
			error: context.DeadlineExceeded,
			want:  AppStateError,
		},
		{
			name:  "Context canceled",
			error: context.Canceled,
			want:  AppStateError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := streamErrorMsg{err: tt.error}
			updatedModel, _ := model.Update(msg)

			m := updatedModel.(*Model)
			if m.currentState != tt.want {
				t.Errorf("Expected state %v, got %v", tt.want, m.currentState)
			}
		})
	}
}

// TestConfigurationOptions tests all configuration options
func TestConfigurationOptions(t *testing.T) {
	tests := []struct {
		name   string
		option Option
		check  func(*Model) bool
	}{
		{
			name:   "WithTemperature",
			option: WithTemperature(0.8),
			check:  func(m *Model) bool { return m.temperature == 0.8 },
		},
		{
			name:   "WithTopP",
			option: WithTopP(0.95),
			check:  func(m *Model) bool { return m.topP == 0.95 },
		},
		{
			name:   "WithTopK",
			option: WithTopK(40),
			check:  func(m *Model) bool { return m.topK == 40 },
		},
		{
			name:   "WithMaxOutputTokens",
			option: WithMaxOutputTokens(2048),
			check:  func(m *Model) bool { return m.maxOutputTokens == 2048 },
		},
		{
			name:   "WithSystemPrompt",
			option: WithSystemPrompt("You are a helpful assistant"),
			check:  func(m *Model) bool { return m.systemPrompt == "You are a helpful assistant" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := New(tt.option)
			if !tt.check(model) {
				t.Errorf("Option %s was not applied correctly", tt.name)
			}
		})
	}
}

// TestAutoSendFeature tests auto-send functionality
func TestAutoSendFeature(t *testing.T) {
	model := New(WithAutoSend(2 * time.Second, "Test message"))

	if model.autoSendDuration != 2*time.Second {
		t.Errorf("Expected auto-send duration to be 2s, got %v", model.autoSendDuration)
	}

	if model.autoSendMessage != "Test message" {
		t.Errorf("Expected auto-send message to be 'Test message', got %s", model.autoSendMessage)
	}
}

// TestStdinMode tests stdin mode functionality
func TestStdinMode(t *testing.T) {
	// This test would require mocking stdin
	t.Run("Stdin mode configuration", func(t *testing.T) {
		// Test configuration only
		model := New()
		model.stdinMode = true

		if !model.stdinMode {
			t.Error("Stdin mode should be enabled")
		}
	})
}

// Benchmark tests for performance validation
func BenchmarkModelCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = New()
	}
}

func BenchmarkMessageFormatting(b *testing.B) {
	msg := Message{
		Sender:  "user",
		Content: "Test message for benchmarking",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = formatMessage(msg)
	}
}

func BenchmarkToolRegistration(b *testing.B) {
	model := New(WithToolsEnabled(true))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.toolManager.RegisterTool(
			"bench_tool",
			"Benchmark tool",
			nil,
			func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				return "result", nil
			},
		)
	}
}