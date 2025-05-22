package audioplayer

import (
	"context"
	"testing"
)

func TestNewStdinPlayer(t *testing.T) {
	// Test with empty command
	player, err := NewStdinPlayer("", DefaultConfig)
	if err == nil {
		t.Error("NewStdinPlayer() with empty command should return an error")
	}
	if player != nil {
		t.Error("NewStdinPlayer() with empty command should return nil player")
	}

	// Test with invalid command format
	player, err = NewStdinPlayer("   ", DefaultConfig)
	if err == nil {
		t.Error("NewStdinPlayer() with invalid command format should return an error")
	}
	if player != nil {
		t.Error("NewStdinPlayer() with invalid command format should return nil player")
	}

	// Test with known command that should exist on most systems
	// Using 'echo' as it's likely to exist on all systems
	player, err = NewStdinPlayer("echo test", DefaultConfig)
	if err != nil {
		t.Errorf("NewStdinPlayer() with valid command 'echo' returned error: %v", err)
	}
	if player == nil {
		t.Error("NewStdinPlayer() with valid command 'echo' returned nil player")
	}

	// Verify command parsing
	if player != nil {
		if player.cmdName != "echo" {
			t.Errorf("NewStdinPlayer() cmdName = %s, want 'echo'", player.cmdName)
		}
		if len(player.cmdArgs) != 1 || player.cmdArgs[0] != "test" {
			t.Errorf("NewStdinPlayer() cmdArgs = %v, want ['test']", player.cmdArgs)
		}
		if player.command != "echo test" {
			t.Errorf("NewStdinPlayer() command = %s, want 'echo test'", player.command)
		}
	}
}

func TestStdinPlayerRequiresWAVHeader(t *testing.T) {
	// Create players with different commands
	aplayPlayer, err := NewStdinPlayer("aplay", DefaultConfig)
	if err != nil {
		t.Skip("Skipping test as 'aplay' command not found")
	}

	// aplay should not require WAV header
	if aplayPlayer.RequiresWAVHeader() {
		t.Error("aplay should not require WAV header")
	}

	// Test with ffplay if available
	ffplayPlayer, err := NewStdinPlayer("ffplay -autoexit -", DefaultConfig)
	if err != nil {
		t.Log("Skipping ffplay test as command not found")
		return
	}

	// ffplay should require WAV header
	if !ffplayPlayer.RequiresWAVHeader() {
		t.Error("ffplay should require WAV header")
	}
}

func TestStdinPlayerEstimatedLatency(t *testing.T) {
	player, _ := NewStdinPlayer("echo test", DefaultConfig)
	latency := player.EstimatedLatency()
	if latency <= 0 {
		t.Error("EstimatedLatency() should return a positive duration")
	}
}

func TestStdinPlayerCleanup(t *testing.T) {
	player, _ := NewStdinPlayer("echo test", DefaultConfig)
	err := player.Cleanup()
	if err != nil {
		t.Errorf("Cleanup() should return nil, got: %v", err)
	}
}

func TestStdinPlayerPlay(t *testing.T) {
	player, _ := NewStdinPlayer("echo test", DefaultConfig)

	// Test with empty audio data
	err := player.Play(context.Background(), []byte{})
	if err == nil {
		t.Error("Play() with empty audio data should return an error")
	}

	// Create audio sample
	audioData := []byte{1, 2, 3, 4}

	// Test Play with context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = player.Play(ctx, audioData)
	if err != context.Canceled {
		t.Errorf("Play() with cancelled context should return context.Canceled, got: %v", err)
	}

	// Full Play test would require an actual audio player command
	// that's available on all systems, which is difficult to guarantee.
	// Most of the important logic is covered by the above tests.
}
