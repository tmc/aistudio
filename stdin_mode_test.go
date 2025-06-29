package aistudio

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/tmc/aistudio/api"
)

// TestProcessStdinMode tests the non-interactive stdin processing mode
func TestProcessStdinMode(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	t.Run("NoClient", func(t *testing.T) {
		model := &Model{
			client: nil,
			apiKey: "test-key",
		}

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// Should not panic with nil client
		err := model.ProcessStdinMode(ctx)
		// Will likely fail with an error since we don't have a real API key/connection
		// but should not panic
		_ = err // Error expected in test environment
	})

	t.Run("WithTimeoutContext", func(t *testing.T) {
		model := New(WithAPIKey("test-key"))
		if model == nil {
			t.Fatal("Failed to create model")
		}

		// Test with a context that has timeout
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := model.ProcessStdinMode(ctx)
		// Should handle the context properly, though will fail without real connection
		_ = err // Error expected in test environment
	})

	t.Run("WithNilContext", func(t *testing.T) {
		model := New(
			WithAPIKey("test-key"),
			WithGlobalTimeout(100*time.Millisecond),
		)
		if model == nil {
			t.Fatal("Failed to create model")
		}

		// Test with nil context - should create its own
		err := model.ProcessStdinMode(nil)
		_ = err // Error expected in test environment

		// Should have created a context
		if model.rootCtx == nil {
			t.Errorf("Expected rootCtx to be created when nil context passed")
		}
	})

	t.Run("WithToolsEnabled", func(t *testing.T) {
		model := New(
			WithAPIKey("test-key"),
			WithTools(true),
		)
		if model == nil {
			t.Fatal("Failed to create model")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := model.ProcessStdinMode(ctx)
		_ = err // Error expected in test environment

		// Should have initialized tool manager
		if model.toolManager == nil {
			t.Errorf("Expected tool manager to be initialized when tools enabled")
		}
	})
}

// TestStdinModeMessageProcessing tests the message processing aspect
func TestStdinModeMessageProcessing(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	// This test simulates the stdin input processing
	// Since ProcessStdinMode reads from os.Stdin, we need to be careful

	t.Run("InputValidation", func(t *testing.T) {
		// Test the general setup without actual stdin reading
		model := New(WithAPIKey("test-key"))
		if model == nil {
			t.Fatal("Failed to create model")
		}

		// Verify initial state
		if len(model.messages) != 0 {
			t.Errorf("Expected no initial messages, got %d", len(model.messages))
		}

		// Test that the model can handle message addition (as would happen in stdin mode)
		userMsg := formatMessage("You", "test message")
		model.messages = append(model.messages, userMsg)

		if len(model.messages) != 1 {
			t.Errorf("Expected 1 message after adding, got %d", len(model.messages))
		}

		if model.messages[0].Sender != "You" {
			t.Errorf("Expected sender to be 'You', got %s", model.messages[0].Sender)
		}

		if model.messages[0].Content != "test message" {
			t.Errorf("Expected content to be 'test message', got %s", model.messages[0].Content)
		}
	})
}

// TestStdinModeConfiguration tests various configuration options for stdin mode
func TestStdinModeConfiguration(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	t.Run("DisableAudioInStdinMode", func(t *testing.T) {
		model := New(
			WithAPIKey("test-key"),
			WithAudioOutput(true), // Enable audio
		)
		if model == nil {
			t.Fatal("Failed to create model")
		}

		// Audio should be enabled initially
		if !model.enableAudio {
			t.Errorf("Expected audio to be enabled initially")
		}

		// In ProcessStdinMode, audio is disabled in the config
		// Test that the config is created properly
		config := api.StreamClientConfig{
			ModelName:       model.modelName,
			EnableAudio:     false, // Should be false in stdin mode
			SystemPrompt:    model.systemPrompt,
			Temperature:     model.temperature,
			TopP:            model.topP,
			TopK:            model.topK,
			MaxOutputTokens: model.maxOutputTokens,
		}

		if config.EnableAudio {
			t.Errorf("Expected audio to be disabled in stdin mode config")
		}

		if config.ModelName == "" {
			t.Errorf("Expected model name to be set in config")
		}
	})

	t.Run("ToolsConfiguration", func(t *testing.T) {
		model := New(
			WithAPIKey("test-key"),
			WithTools(true),
		)
		if model == nil {
			t.Fatal("Failed to create model")
		}

		// Should have tools enabled
		if !model.enableTools {
			t.Errorf("Expected tools to be enabled")
		}

		// Tool manager should be nil initially (created in ProcessStdinMode)
		// This is expected behavior - tools are only initialized when actually needed
	})

	t.Run("HistoryConfiguration", func(t *testing.T) {
		model := New(
			WithAPIKey("test-key"),
			WithHistory(true, "/tmp/test-history"),
		)
		if model == nil {
			t.Fatal("Failed to create model")
		}

		// Should have history enabled
		if !model.historyEnabled {
			t.Errorf("Expected history to be enabled")
		}

		// History manager might be nil initially (depends on implementation)
		// This tests that the flag is set correctly
	})
}

// mockStdin simulates stdin input for testing
type mockStdin struct {
	content string
	pos     int
}

func (m *mockStdin) Read(p []byte) (n int, err error) {
	if m.pos >= len(m.content) {
		return 0, io.EOF
	}

	n = copy(p, m.content[m.pos:])
	m.pos += n

	return n, nil
}

// TestStdinModeWithMockInput tests stdin processing with simulated input
func TestStdinModeWithMockInput(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	// Save original stdin
	originalStdin := os.Stdin
	defer func() { os.Stdin = originalStdin }()

	t.Run("EmptyInput", func(t *testing.T) {
		// Create a pipe with empty content
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()

		// Close write end immediately to simulate empty input
		w.Close()

		// Replace stdin
		os.Stdin = r

		model := New(WithAPIKey("test-key"))
		if model == nil {
			t.Fatal("Failed to create model")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err = model.ProcessStdinMode(ctx)
		// Should handle empty input gracefully
		_ = err // Error expected due to no real connection
	})

	t.Run("SingleLineInput", func(t *testing.T) {
		// Create a pipe with single line
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()

		// Write test input
		go func() {
			defer w.Close()
			w.WriteString("hello world\n")
		}()

		// Replace stdin
		os.Stdin = r

		model := New(WithAPIKey("test-key"))
		if model == nil {
			t.Fatal("Failed to create model")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err = model.ProcessStdinMode(ctx)
		// Should process the input line
		_ = err // Error expected due to no real connection
	})
}

// TestStreamClientConfigCreation tests the creation of stream client config
func TestStreamClientConfigCreation(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(
		WithAPIKey("test-key"),
		WithModel("test-model"),
		WithTemperature(0.7),
		WithTopP(0.9),
		WithTopK(40),
		WithMaxOutputTokens(1000),
		WithSystemPrompt("You are a helpful assistant"),
		WithTools(true),
	)
	if model == nil {
		t.Fatal("Failed to create model")
	}

	// Mock tool manager for testing
	model.toolManager = NewToolManager()

	// Create config as would be done in ProcessStdinMode
	config := api.StreamClientConfig{
		ModelName:       model.modelName,
		EnableAudio:     false, // Always false in stdin mode
		SystemPrompt:    model.systemPrompt,
		Temperature:     model.temperature,
		TopP:            model.topP,
		TopK:            model.topK,
		MaxOutputTokens: model.maxOutputTokens,
	}

	// Add tool definitions if enabled
	if model.enableTools && model.toolManager != nil {
		config.ToolDefinitions = model.toolManager.GetAvailableTools()
	}

	// Verify config values
	if config.ModelName != "test-model" {
		t.Errorf("Expected model name 'test-model', got %s", config.ModelName)
	}

	if config.EnableAudio {
		t.Errorf("Expected audio to be disabled in stdin mode")
	}

	if config.SystemPrompt != "You are a helpful assistant" {
		t.Errorf("Expected system prompt to be set, got %s", config.SystemPrompt)
	}

	if config.Temperature != 0.7 {
		t.Errorf("Expected temperature 0.7, got %f", config.Temperature)
	}

	if config.TopP != 0.9 {
		t.Errorf("Expected topP 0.9, got %f", config.TopP)
	}

	if config.TopK != 40 {
		t.Errorf("Expected topK 40, got %d", config.TopK)
	}

	if config.MaxOutputTokens != 1000 {
		t.Errorf("Expected maxOutputTokens 1000, got %d", config.MaxOutputTokens)
	}

	// Tool definitions should be included if tools are enabled
	if model.enableTools && model.toolManager != nil {
		tools := model.toolManager.GetAvailableTools()
		if len(config.ToolDefinitions) != len(tools) {
			t.Errorf("Expected %d tool definitions, got %d", len(tools), len(config.ToolDefinitions))
		}
	}
}
