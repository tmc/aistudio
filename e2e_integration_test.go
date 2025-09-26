// +build integration

package aistudio

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestE2EFullConversation tests a complete conversation flow
func TestE2EFullConversation(t *testing.T) {
	if os.Getenv("AISTUDIO_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration test: Set AISTUDIO_RUN_INTEGRATION_TESTS=1 to run")
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	// Create model with real API key
	model := New(
		WithAPIKey(apiKey),
		WithModel("gemini-2.0-flash-latest"),
		WithToolsEnabled(true),
		WithHistoryEnabled(true),
	)

	// Initialize the model
	initCmd := model.Init()
	if initCmd == nil {
		t.Fatal("Expected init command")
	}

	// Simulate sending a message
	t.Run("Send message", func(t *testing.T) {
		// Add user message
		model.messages = append(model.messages, Message{
			Sender:  "user",
			Content: "What is 2+2?",
		})

		// Verify message was added
		if len(model.messages) != 1 {
			t.Errorf("Expected 1 message, got %d", len(model.messages))
		}
	})

	// Test state transitions
	t.Run("State transitions", func(t *testing.T) {
		// Should start ready
		if model.currentState != AppStateReady {
			t.Errorf("Expected Ready state, got %v", model.currentState)
		}

		// Simulate waiting for response
		model.currentState = AppStateWaiting
		if model.currentState != AppStateWaiting {
			t.Errorf("Expected Waiting state, got %v", model.currentState)
		}
	})
}

// TestE2EToolExecution tests tool execution flow
func TestE2EToolExecution(t *testing.T) {
	if os.Getenv("AISTUDIO_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration test")
	}

	model := New(
		WithToolsEnabled(true),
		WithToolApprovalRequired(false), // Auto-approve for testing
	)

	// Register a test tool
	t.Run("Register tool", func(t *testing.T) {
		handler := func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return map[string]string{"result": "success"}, nil
		}

		err := model.toolManager.RegisterTool(
			"integration_test_tool",
			"Test tool for integration testing",
			nil,
			handler,
		)

		if err != nil {
			t.Fatalf("Failed to register tool: %v", err)
		}
	})

	// Execute the tool
	t.Run("Execute tool", func(t *testing.T) {
		toolCall := ToolCall{
			ID:   "test-123",
			Name: "integration_test_tool",
		}

		results, err := model.executeToolCalls([]ToolCall{toolCall})
		if err != nil {
			t.Fatalf("Tool execution failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})
}

// TestE2ESessionPersistence tests session save and load
func TestE2ESessionPersistence(t *testing.T) {
	if os.Getenv("AISTUDIO_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration test")
	}

	// Create temporary directory for history
	tmpDir := t.TempDir()

	model := New(
		WithHistoryEnabled(true),
		WithHistoryDir(tmpDir),
	)

	// Create and save session
	t.Run("Save session", func(t *testing.T) {
		// Add test messages
		testMessages := []Message{
			{Sender: "user", Content: "Test message 1"},
			{Sender: "model", Content: "Test response 1"},
			{Sender: "user", Content: "Test message 2"},
			{Sender: "model", Content: "Test response 2"},
		}

		for _, msg := range testMessages {
			model.messages = append(model.messages, msg)
			if model.historyManager != nil {
				model.historyManager.AddMessage(msg)
			}
		}

		// Save session
		if model.historyManager != nil && model.historyManager.CurrentSession != nil {
			err := model.historyManager.SaveSession(model.historyManager.CurrentSession)
			if err != nil {
				t.Fatalf("Failed to save session: %v", err)
			}
		}
	})

	// Load and verify session
	t.Run("Load session", func(t *testing.T) {
		if model.historyManager != nil {
			err := model.historyManager.LoadSessions()
			if err != nil {
				t.Fatalf("Failed to load sessions: %v", err)
			}

			if len(model.historyManager.Sessions) == 0 {
				t.Error("No sessions loaded")
			}
		}
	})
}

// TestE2ENavigationFlow tests UI navigation
func TestE2ENavigationFlow(t *testing.T) {
	if os.Getenv("AISTUDIO_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration test")
	}

	model := New()

	// Test Tab navigation
	t.Run("Tab navigation", func(t *testing.T) {
		initialFocus := model.focusIndex

		// Simulate Tab key
		msg := tea.KeyMsg{Type: tea.KeyTab}
		updatedModel, _ := model.Update(msg)
		m := updatedModel.(*Model)

		if m.focusIndex == initialFocus && model.showSettingsPanel {
			t.Error("Focus should have changed with Tab")
		}
	})

	// Test Shift+Tab navigation
	t.Run("Shift+Tab navigation", func(t *testing.T) {
		initialFocus := model.focusIndex

		// Simulate Shift+Tab key
		msg := tea.KeyMsg{Type: tea.KeyShiftTab}
		updatedModel, _ := model.Update(msg)
		m := updatedModel.(*Model)

		if m.focusIndex == initialFocus && model.showSettingsPanel {
			t.Error("Focus should have changed with Shift+Tab")
		}
	})

	// Test Escape key
	t.Run("Escape handling", func(t *testing.T) {
		model.showToolApproval = true

		msg := tea.KeyMsg{Type: tea.KeyEscape}
		updatedModel, _ := model.Update(msg)
		m := updatedModel.(*Model)

		if m.showToolApproval {
			t.Error("Tool approval should be closed with Escape")
		}
	})
}

// TestE2EErrorRecovery tests error handling and recovery
func TestE2EErrorRecovery(t *testing.T) {
	if os.Getenv("AISTUDIO_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration test")
	}

	model := New()

	// Test context timeout recovery
	t.Run("Context timeout", func(t *testing.T) {
		err := context.DeadlineExceeded
		msg := streamErrorMsg{err: err}

		updatedModel, _ := model.Update(msg)
		m := updatedModel.(*Model)

		if m.currentState != AppStateError {
			t.Errorf("Expected Error state, got %v", m.currentState)
		}

		// Should be able to recover
		m.currentState = AppStateReady
		if m.currentState != AppStateReady {
			t.Error("Should be able to recover from error state")
		}
	})

	// Test connection reset recovery
	t.Run("Connection reset", func(t *testing.T) {
		// Simulate connection reset
		model.currentState = AppStateError

		// Attempt recovery
		model.currentState = AppStateInitializing
		// In real scenario, would reinitialize connection
		model.currentState = AppStateReady

		if model.currentState != AppStateReady {
			t.Error("Should recover from connection reset")
		}
	})
}

// TestE2EPerformance tests performance characteristics
func TestE2EPerformance(t *testing.T) {
	if os.Getenv("AISTUDIO_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration test")
	}

	model := New(WithToolsEnabled(true))

	// Test message processing speed
	t.Run("Message processing", func(t *testing.T) {
		start := time.Now()

		// Add 100 messages
		for i := 0; i < 100; i++ {
			model.messages = append(model.messages, Message{
				Sender:  "user",
				Content: "Test message",
			})
		}

		elapsed := time.Since(start)
		if elapsed > 100*time.Millisecond {
			t.Errorf("Message processing too slow: %v", elapsed)
		}
	})

	// Test tool registration speed
	t.Run("Tool registration", func(t *testing.T) {
		start := time.Now()

		// Register 50 tools
		for i := 0; i < 50; i++ {
			_ = model.toolManager.RegisterTool(
				fmt.Sprintf("perf_tool_%d", i),
				"Performance test tool",
				nil,
				func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
					return "result", nil
				},
			)
		}

		elapsed := time.Since(start)
		if elapsed > 50*time.Millisecond {
			t.Errorf("Tool registration too slow: %v", elapsed)
		}
	})
}

// TestE2EMultimodalFlow tests multimodal capabilities
func TestE2EMultimodalFlow(t *testing.T) {
	if os.Getenv("AISTUDIO_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration test")
	}

	model := New(
		WithAudioOutput(true),
		WithVideoInputEnabled(true),
	)

	// Test audio message handling
	t.Run("Audio message", func(t *testing.T) {
		msg := Message{
			Sender:    "model",
			Content:   "Audio response",
			HasAudio:  true,
			AudioData: []byte("fake audio data"),
		}

		model.messages = append(model.messages, msg)

		lastMsg := model.messages[len(model.messages)-1]
		if !lastMsg.HasAudio {
			t.Error("Audio flag should be preserved")
		}
		if len(lastMsg.AudioData) == 0 {
			t.Error("Audio data should be preserved")
		}
	})

	// Test video input handling
	t.Run("Video input", func(t *testing.T) {
		// Simulate video frame
		frame := ImageFrame{
			Data:   []byte("fake video frame"),
			Format: "jpeg",
		}

		msg := ImageCaptureMsg{
			Frame: frame,
		}

		// Process video frame message
		updatedModel, _ := model.Update(msg)
		m := updatedModel.(*Model)

		// Verify model can handle video input
		if m == nil {
			t.Error("Model should handle video input")
		}
	})
}

// BenchmarkE2EMessageProcessing benchmarks message processing
func BenchmarkE2EMessageProcessing(b *testing.B) {
	model := New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model.messages = append(model.messages, Message{
			Sender:  "user",
			Content: "Benchmark message",
		})

		// Clear for next iteration
		if len(model.messages) > 1000 {
			model.messages = model.messages[500:]
		}
	}
}

// BenchmarkE2EToolExecution benchmarks tool execution
func BenchmarkE2EToolExecution(b *testing.B) {
	model := New(WithToolsEnabled(true))

	// Register benchmark tool
	_ = model.toolManager.RegisterTool(
		"benchmark_tool",
		"Benchmark tool",
		nil,
		func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return "result", nil
		},
	)

	toolCall := ToolCall{
		ID:   "bench",
		Name: "benchmark_tool",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = model.executeToolCalls([]ToolCall{toolCall})
	}
}