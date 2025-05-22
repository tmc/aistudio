package audioplayer

import (
	"context"
	"testing"
	"time"

	"github.com/tmc/aistudio/internal/helpers"
)

func TestNewAfplayPlayer(t *testing.T) {
	// Test with default config
	player := NewAfplayPlayer(Config{})
	if player == nil {
		t.Fatal("NewAfplayPlayer() returned nil")
	}
	if player.config.SampleRate != DefaultConfig.SampleRate {
		t.Errorf("NewAfplayPlayer() with empty config should use default sample rate, got %d, want %d",
			player.config.SampleRate, DefaultConfig.SampleRate)
	}

	// Test with custom config
	customConfig := Config{
		SampleRate:    44100,
		Channels:      2,
		BitsPerSample: 24,
		Format:        "s24le",
	}
	player = NewAfplayPlayer(customConfig)
	if player.config.SampleRate != customConfig.SampleRate {
		t.Errorf("NewAfplayPlayer() should use provided sample rate, got %d, want %d",
			player.config.SampleRate, customConfig.SampleRate)
	}
	if player.config.Channels != customConfig.Channels {
		t.Errorf("NewAfplayPlayer() should use provided channels, got %d, want %d",
			player.config.Channels, customConfig.Channels)
	}
	if player.config.BitsPerSample != customConfig.BitsPerSample {
		t.Errorf("NewAfplayPlayer() should use provided bits per sample, got %d, want %d",
			player.config.BitsPerSample, customConfig.BitsPerSample)
	}
	if player.config.Format != customConfig.Format {
		t.Errorf("NewAfplayPlayer() should use provided format, got %s, want %s",
			player.config.Format, customConfig.Format)
	}
}

func TestAfplayPlayerRequiresWAVHeader(t *testing.T) {
	player := NewAfplayPlayer(DefaultConfig)
	if !player.RequiresWAVHeader() {
		t.Error("RequiresWAVHeader() should return true for AfplayPlayer")
	}
}

func TestAfplayPlayerEstimatedLatency(t *testing.T) {
	player := NewAfplayPlayer(DefaultConfig)
	latency := player.EstimatedLatency()
	if latency <= 0 {
		t.Error("EstimatedLatency() should return a positive duration")
	}
}

func TestAfplayPlayerCleanup(t *testing.T) {
	player := NewAfplayPlayer(DefaultConfig)
	err := player.Cleanup()
	if err != nil {
		t.Errorf("Cleanup() should return nil, got: %v", err)
	}
}

func TestAfplayPlayerPlay(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping afplay test in short mode")
	}

	player := NewAfplayPlayer(DefaultConfig)

	// Test with empty audio data
	err := player.Play(context.Background(), []byte{})
	if err == nil {
		t.Error("Play() with empty audio data should return an error")
	}

	// Create a small test WAV file (just the data part, not the header)
	// 1 second of silence at 8kHz mono 16-bit
	audioData := make([]byte, 16000)
	for i := range audioData {
		audioData[i] = 0 // Silence
	}

	// The actual Play test is skipped in normal CI environments
	// since it requires the afplay command and would produce sound
	// This is more of an integration test than a unit test
	t.Run("PlaySound", func(t *testing.T) {
		t.Skip("Skipping actual audio playback test (requires afplay command)")

		// Set a timeout for the test (just in case)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Mock a WAV header for testing
		wavData := append(helpers.CreateWavHeader(len(audioData), 1, 8000, 16), audioData...)

		err := player.Play(ctx, wavData)
		if err != nil {
			t.Errorf("Play() returned an error: %v", err)
		}
	})

	// Test cancellation
	t.Run("Cancellation", func(t *testing.T) {
		t.Skip("Skipping cancellation test (requires afplay command)")

		ctx, cancel := context.WithCancel(context.Background())

		// Cancel after a short delay
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()

		// Create a longer audio sample that would take several seconds to play
		longAudio := make([]byte, 160000) // 10 seconds at 8kHz mono 16-bit

		err := player.Play(ctx, longAudio)
		if err == nil || err != context.Canceled {
			t.Errorf("Play() with cancelled context should return context.Canceled, got: %v", err)
		}
	})
}
