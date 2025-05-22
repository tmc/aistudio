package audioplayer

import (
	"context"
	"testing"
	"time"
)

// TestDefaultConfig tests the default config values
func TestDefaultConfig(t *testing.T) {
	if DefaultConfig.SampleRate != 24000 {
		t.Errorf("DefaultConfig.SampleRate = %d, want 24000", DefaultConfig.SampleRate)
	}
	if DefaultConfig.Channels != 1 {
		t.Errorf("DefaultConfig.Channels = %d, want 1", DefaultConfig.Channels)
	}
	if DefaultConfig.BitsPerSample != 16 {
		t.Errorf("DefaultConfig.BitsPerSample = %d, want 16", DefaultConfig.BitsPerSample)
	}
	if DefaultConfig.Format != "s16le" {
		t.Errorf("DefaultConfig.Format = %s, want s16le", DefaultConfig.Format)
	}
}

// MockPlayer implements the Player interface for testing
type MockPlayer struct {
	playFunc              func(ctx context.Context, audioData []byte) error
	cleanupFunc           func() error
	requiresWAVHeaderFunc func() bool
	estimatedLatencyFunc  func() time.Duration

	// For testing purposes
	audioData []byte
	played    bool
	cleaned   bool
}

func NewMockPlayer() *MockPlayer {
	return &MockPlayer{
		playFunc: func(ctx context.Context, audioData []byte) error {
			return nil
		},
		cleanupFunc: func() error {
			return nil
		},
		requiresWAVHeaderFunc: func() bool {
			return false
		},
		estimatedLatencyFunc: func() time.Duration {
			return 0
		},
	}
}

func (p *MockPlayer) Play(ctx context.Context, audioData []byte) error {
	p.audioData = audioData
	p.played = true
	return p.playFunc(ctx, audioData)
}

func (p *MockPlayer) Cleanup() error {
	p.cleaned = true
	return p.cleanupFunc()
}

func (p *MockPlayer) RequiresWAVHeader() bool {
	return p.requiresWAVHeaderFunc()
}

func (p *MockPlayer) EstimatedLatency() time.Duration {
	return p.estimatedLatencyFunc()
}

// TestPlayerInterface tests that our implementations satisfy the Player interface
func TestPlayerInterface(t *testing.T) {
	// Create instances of our implementations
	afplayPlayer := NewAfplayPlayer(DefaultConfig)
	stdinPlayer, _ := NewStdinPlayer("echo test", DefaultConfig)
	mockPlayer := NewMockPlayer()

	// Define a variable of type Player and assign each implementation to it
	var player Player

	// Test AfplayPlayer implements Player
	player = afplayPlayer
	if player == nil {
		t.Error("AfplayPlayer does not implement Player interface")
	}

	// Test StdinPlayer implements Player
	player = stdinPlayer
	if player == nil {
		t.Error("StdinPlayer does not implement Player interface")
	}

	// Test MockPlayer implements Player
	player = mockPlayer
	if player == nil {
		t.Error("MockPlayer does not implement Player interface")
	}
}
