package audio

import (
	"testing"
	"time"
)

func TestDefaultProactiveConfig_Basic(t *testing.T) {
	config := DefaultProactiveConfig()

	if !config.Enabled {
		t.Error("Expected default config to be enabled")
	}

	if config.RelevanceThreshold != 0.7 {
		t.Errorf("Expected relevance threshold to be 0.7, got %f", config.RelevanceThreshold)
	}

	if config.ContextWindow != 30*time.Second {
		t.Errorf("Expected context window to be 30s, got %v", config.ContextWindow)
	}

	if config.BufferDuration != 5*time.Second {
		t.Errorf("Expected buffer duration to be 5s, got %v", config.BufferDuration)
	}

	if len(config.DeviceKeywords) == 0 {
		t.Error("Expected device keywords to be set")
	}
}

func TestNewProactiveAudioManager_Basic(t *testing.T) {
	config := DefaultProactiveConfig()
	pam := NewProactiveAudioManager(config)

	if pam == nil {
		t.Fatal("Expected ProactiveAudioManager to be created")
	}

	// Check basic initialization
	if pam.contextWindow == 0 {
		t.Error("Expected contextWindow to be set")
	}

	if pam.enabled != config.Enabled {
		t.Error("Expected enabled state to match config")
	}

	if pam.relevanceThreshold != config.RelevanceThreshold {
		t.Error("Expected relevance threshold to match config")
	}
}

func TestAudioInput_Basic(t *testing.T) {
	input := AudioInput{
		Data:       []byte("test audio data"),
		Timestamp:  time.Now(),
		Transcript: "test transcript",
		Energy:     0.5,
		IsSpeech:   true,
	}

	if len(input.Data) == 0 {
		t.Error("Expected data to be set")
	}

	if input.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}

	if input.Transcript != "test transcript" {
		t.Error("Expected transcript to match")
	}
}

func TestContextEntry_Basic(t *testing.T) {
	entry := ContextEntry{
		Timestamp:  time.Now(),
		Type:       "user",
		Content:    "test content",
		IsRelevant: true,
	}

	if entry.Type != "user" {
		t.Errorf("Expected type to be 'user', got %s", entry.Type)
	}

	if entry.Content != "test content" {
		t.Errorf("Expected content to be 'test content', got %s", entry.Content)
	}

	if !entry.IsRelevant {
		t.Error("Expected entry to be marked as relevant")
	}
}

func TestRelevanceResult_Basic(t *testing.T) {
	result := RelevanceResult{
		IsRelevant:    true,
		Score:         0.8,
		Reason:        "High confidence",
		ShouldRespond: true,
		ResponseType:  "immediate",
	}

	if !result.IsRelevant {
		t.Error("Expected result to be relevant")
	}

	if result.Score != 0.8 {
		t.Errorf("Expected score to be 0.8, got %f", result.Score)
	}

	if result.ResponseType != "immediate" {
		t.Errorf("Expected response type to be 'immediate', got %s", result.ResponseType)
	}
}