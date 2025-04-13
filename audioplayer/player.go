package audioplayer

import (
	"context"
	"time"
)

// Player is the interface for audio playback implementations.
type Player interface {
	// Play takes audio data (expected format: PCM S16LE, 24kHz, mono)
	// and plays it, blocking until playback is complete or an error occurs.
	// The context can be used for cancellation.
	Play(ctx context.Context, audioData []byte) error

	// Cleanup performs any necessary resource cleanup for the player.
	Cleanup() error

	// RequiresWAVHeader indicates if the player needs a WAV header prepended to raw PCM data.
	RequiresWAVHeader() bool

	// EstimatedLatency returns an estimate of the player's startup latency.
	// This can help the buffering logic make better decisions.
	EstimatedLatency() time.Duration
}

// Config holds configuration common to audio players.
type Config struct {
	SampleRate    int
	Channels      int
	BitsPerSample int
	Format        string // e.g., "s16le"
}

// DefaultConfig provides standard audio configuration.
var DefaultConfig = Config{
	SampleRate:    24000,
	Channels:      1,
	BitsPerSample: 16,
	Format:        "s16le", // Signed 16-bit Little Endian
}
