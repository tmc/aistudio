package aistudio

import (
	"fmt"
	"testing"
	"time"
)

// TestWithBackend tests the WithBackend option
func TestWithBackend(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	t.Run("ValidBackend", func(t *testing.T) {
		model := New(
			WithAPIKey("test-key"),
			WithBackend(BackendVertexAI),
		)

		if model == nil {
			t.Fatal("Failed to create model")
		}

		// Backend should be set
		if model.backend != BackendVertexAI {
			t.Errorf("Expected backend to be BackendVertexAI, got %s", model.backend)
		}
	})

	t.Run("GeminiBackend", func(t *testing.T) {
		model := New(
			WithAPIKey("test-key"),
			WithBackend(BackendGeminiAPI),
		)

		if model == nil {
			t.Fatal("Failed to create model")
		}

		// Backend should be set
		if model.backend != BackendGeminiAPI {
			t.Errorf("Expected backend to be BackendGeminiAPI, got %s", model.backend)
		}
	})

	t.Run("MultipleBackends", func(t *testing.T) {
		// Test that last backend wins
		model := New(
			WithAPIKey("test-key"),
			WithBackend(BackendGeminiAPI),
			WithBackend(BackendVertexAI),
		)

		if model == nil {
			t.Fatal("Failed to create model")
		}

		// Last backend should win
		if model.backend != BackendVertexAI {
			t.Errorf("Expected backend to be BackendVertexAI (last set), got %s", model.backend)
		}
	})
}

// TestWithVertexAI tests the WithVertexAI option
func TestWithVertexAI(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	t.Run("EnableVertexAI", func(t *testing.T) {
		model := New(
			WithAPIKey("test-key"),
			WithVertexAI(true, "test-project", "us-central1"),
		)

		if model == nil {
			t.Fatal("Failed to create model")
		}

		// Backend should be set to VertexAI
		if model.backend != BackendVertexAI {
			t.Errorf("Expected backend to be BackendVertexAI")
		}

		// Project and location should be set
		if model.projectID != "test-project" {
			t.Errorf("Expected projectID to be 'test-project', got %s", model.projectID)
		}

		if model.location != "us-central1" {
			t.Errorf("Expected location to be 'us-central1', got %s", model.location)
		}
	})

	t.Run("DisableVertexAI", func(t *testing.T) {
		model := New(
			WithAPIKey("test-key"),
			WithVertexAI(false, "", ""),
		)

		if model == nil {
			t.Fatal("Failed to create model")
		}

		// Backend should be set to GeminiAPI
		if model.backend != BackendGeminiAPI {
			t.Errorf("Expected backend to be BackendGeminiAPI")
		}
	})

	t.Run("MultipleVertexAISettings", func(t *testing.T) {
		// Test that last setting wins
		model := New(
			WithAPIKey("test-key"),
			WithVertexAI(true, "project1", "us-west1"),
			WithVertexAI(false, "project2", "us-east1"),
		)

		if model == nil {
			t.Fatal("Failed to create model")
		}

		// Last setting should win
		if model.backend != BackendGeminiAPI {
			t.Errorf("Expected backend to be BackendGeminiAPI (last set)")
		}

		// Should still have the project and location from last call
		if model.projectID != "project2" {
			t.Errorf("Expected projectID to be 'project2', got %s", model.projectID)
		}
	})
}

// TestWithSystemPrompt tests the WithSystemPrompt option
func TestWithSystemPrompt(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	t.Run("ValidSystemPrompt", func(t *testing.T) {
		prompt := "You are a helpful coding assistant."
		model := New(
			WithAPIKey("test-key"),
			WithSystemPrompt(prompt),
		)

		if model == nil {
			t.Fatal("Failed to create model")
		}

		if model.systemPrompt != prompt {
			t.Errorf("Expected system prompt %q, got %q", prompt, model.systemPrompt)
		}
	})

	t.Run("EmptySystemPrompt", func(t *testing.T) {
		model := New(
			WithAPIKey("test-key"),
			WithSystemPrompt(""),
		)

		if model == nil {
			t.Fatal("Failed to create model")
		}

		if model.systemPrompt != "" {
			t.Errorf("Expected empty system prompt, got %q", model.systemPrompt)
		}
	})
}

// TestWithTemperature tests the WithTemperature option
func TestWithTemperature(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	t.Run("ValidTemperature", func(t *testing.T) {
		temp := float32(0.7)
		model := New(
			WithAPIKey("test-key"),
			WithTemperature(temp),
		)

		if model == nil {
			t.Fatal("Failed to create model")
		}

		if model.temperature != temp {
			t.Errorf("Expected temperature %f, got %f", temp, model.temperature)
		}
	})

	t.Run("ZeroTemperature", func(t *testing.T) {
		model := New(
			WithAPIKey("test-key"),
			WithTemperature(float32(0.0)),
		)

		if model == nil {
			t.Fatal("Failed to create model")
		}

		if model.temperature != 0.0 {
			t.Errorf("Expected temperature 0.0, got %f", model.temperature)
		}
	})

	t.Run("HighTemperature", func(t *testing.T) {
		model := New(
			WithAPIKey("test-key"),
			WithTemperature(float32(2.0)),
		)

		if model == nil {
			t.Fatal("Failed to create model")
		}

		if model.temperature != 2.0 {
			t.Errorf("Expected temperature 2.0, got %f", model.temperature)
		}
	})
}

// TestWithTopP tests the WithTopP option
func TestWithTopP(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(
		WithAPIKey("test-key"),
		WithTopP(float32(0.9)),
	)

	if model == nil {
		t.Fatal("Failed to create model")
	}

	if model.topP != 0.9 {
		t.Errorf("Expected topP 0.9, got %f", model.topP)
	}
}

// TestWithTopK tests the WithTopK option
func TestWithTopK(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(
		WithAPIKey("test-key"),
		WithTopK(int32(40)),
	)

	if model == nil {
		t.Fatal("Failed to create model")
	}

	if model.topK != 40 {
		t.Errorf("Expected topK 40, got %d", model.topK)
	}
}

// TestWithMaxOutputTokens tests the WithMaxOutputTokens option
func TestWithMaxOutputTokens(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(
		WithAPIKey("test-key"),
		WithMaxOutputTokens(int32(2048)),
	)

	if model == nil {
		t.Fatal("Failed to create model")
	}

	if model.maxOutputTokens != 2048 {
		t.Errorf("Expected maxOutputTokens 2048, got %d", model.maxOutputTokens)
	}
}

// TestWithVoice tests the WithVoice option
func TestWithVoice(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(
		WithAPIKey("test-key"),
		WithAudioOutput(true, "alloy"),
	)

	if model == nil {
		t.Fatal("Failed to create model")
	}

	if model.voiceName != "alloy" {
		t.Errorf("Expected voice 'alloy', got %s", model.voiceName)
	}
}

// TestWithAutoSend tests the WithAutoSend option
func TestWithAutoSend(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	delayStr := "5s"
	model := New(
		WithAPIKey("test-key"),
		WithAutoSend(delayStr),
	)

	if model == nil {
		t.Fatal("Failed to create model")
	}

	if !model.autoSendEnabled {
		t.Errorf("Expected autoSend to be enabled")
	}

	expectedDelay := 5 * time.Second
	if model.autoSendDelay != expectedDelay {
		t.Errorf("Expected autoSend delay %v, got %v", expectedDelay, model.autoSendDelay)
	}
}

// TestWithHistoryEnabled tests the WithHistoryEnabled option
func TestWithHistoryEnabled(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(
		WithAPIKey("test-key"),
		WithHistory(true, "/tmp/aistudio-history-test"),
	)

	if model == nil {
		t.Fatal("Failed to create model")
	}

	if !model.historyEnabled {
		t.Errorf("Expected history to be enabled")
	}
}

// TestWithToolsEnabled tests the WithToolsEnabled option
func TestWithToolsEnabled(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(
		WithAPIKey("test-key"),
		WithTools(true),
	)

	if model == nil {
		t.Fatal("Failed to create model")
	}

	if !model.enableTools {
		t.Errorf("Expected tools to be enabled")
	}
}

// TestWithToolApprovalRequired tests the WithToolApprovalRequired option
func TestWithToolApprovalRequired(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(
		WithAPIKey("test-key"),
		WithToolApproval(true),
	)

	if model == nil {
		t.Fatal("Failed to create model")
	}

	if !model.requireApproval {
		t.Errorf("Expected tool approval to be required")
	}
}

// Note: WithShowLogo and WithWebSocketEnabled are not implemented yet

// TestMultipleOptions tests combining multiple options
func TestMultipleOptions(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(
		WithAPIKey("test-key"),
		WithModel("test-model"),
		WithBackend(BackendVertexAI),
		WithVertexAI(true, "test-project", "us-central1"),
		WithTemperature(float32(0.8)),
		WithTopP(float32(0.95)),
		WithTopK(int32(50)),
		WithMaxOutputTokens(int32(1024)),
		WithSystemPrompt("You are a test assistant"),
		WithHistory(true, "/tmp/test-history"),
		WithTools(true),
		WithToolApproval(false),
		WithAudioOutput(true),
		WithGlobalTimeout(30*time.Second),
	)

	if model == nil {
		t.Fatal("Failed to create model")
	}

	// Verify all options were applied
	if model.apiKey != "test-key" {
		t.Errorf("Expected API key 'test-key', got %s", model.apiKey)
	}

	if model.modelName != "test-model" {
		t.Errorf("Expected model 'test-model', got %s", model.modelName)
	}

	if model.backend != BackendVertexAI {
		t.Errorf("Expected backend BackendVertexAI, got %s", model.backend)
	}

	if model.temperature != 0.8 {
		t.Errorf("Expected temperature 0.8, got %f", model.temperature)
	}

	if model.topP != 0.95 {
		t.Errorf("Expected topP 0.95, got %f", model.topP)
	}

	if model.topK != 50 {
		t.Errorf("Expected topK 50, got %d", model.topK)
	}

	if model.maxOutputTokens != 1024 {
		t.Errorf("Expected maxOutputTokens 1024, got %d", model.maxOutputTokens)
	}

	if model.voiceName != DefaultVoice {
		t.Errorf("Expected voice '%s', got %s", DefaultVoice, model.voiceName)
	}

	if model.systemPrompt != "You are a test assistant" {
		t.Errorf("Expected system prompt 'You are a test assistant', got %s", model.systemPrompt)
	}

	if !model.historyEnabled {
		t.Errorf("Expected history to be enabled")
	}

	if !model.enableTools {
		t.Errorf("Expected tools to be enabled")
	}

	if model.requireApproval {
		t.Errorf("Expected tool approval to be disabled")
	}

	if !model.enableAudio {
		t.Errorf("Expected audio to be enabled")
	}

	// showLogo and enableWebSocket are not set in the updated test

	if model.globalTimeout != 30*time.Second {
		t.Errorf("Expected global timeout 30s, got %v", model.globalTimeout)
	}
}

// TestOptionErrors tests error handling in options
func TestOptionErrors(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	// Create an option that always returns an error
	errorOption := func(m *Model) error {
		return fmt.Errorf("test option error")
	}

	// Model should still be created even with option errors (they're logged as warnings)
	model := New(
		WithAPIKey("test-key"),
		errorOption,
	)

	if model == nil {
		t.Fatal("Expected model to be created despite option error")
	}

	// API key should still be set from the successful option
	if model.apiKey != "test-key" {
		t.Errorf("Expected API key to be set despite option error")
	}
}
