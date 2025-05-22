package audioplayer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/tmc/aistudio/internal/helpers" // Use internal helper for WAV header
)

// StdinPlayer plays audio by piping raw PCM data (optionally with a WAV header)
// to the standard input of an external command (e.g., ffplay, aplay).
type StdinPlayer struct {
	command string // The full command string (e.g., "ffplay -autoexit ... -i -")
	config  Config
	cmdName string // Just the command name (e.g., "ffplay")
	cmdArgs []string
}

// NewStdinPlayer creates a new StdinPlayer instance.
// The command string should include a placeholder like '-' for stdin.
// Deprecated: This implementation is not currently used in the codebase.
// It is kept for compatibility with the Player interface and potential future use.
// The application has moved to a different audio playback approach using the Model in audioplayer.go,
// but these interface implementations are preserved for potential reuse in future audio backends.
func NewStdinPlayer(command string, config Config) (*StdinPlayer, error) {
	if command == "" {
		return nil, errors.New("audio player command cannot be empty")
	}
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, errors.New("invalid audio player command format")
	}

	// Basic check if command exists
	if _, err := exec.LookPath(parts[0]); err != nil {
		return nil, fmt.Errorf("audio player command '%s' not found in PATH: %w", parts[0], err)
	}

	log.Printf("[StdinPlayer] Initialized with command: %q", command)
	return &StdinPlayer{
		command: command,
		config:  config,
		cmdName: parts[0],
		cmdArgs: parts[1:],
	}, nil
}

// Play implements the Player interface by writing to the command's stdin.
// Deprecated: This implementation is not currently used in the codebase.
// It is kept for compatibility with the Player interface and potential future use.
// The application has moved to a different audio playback approach using the Model in audioplayer.go,
// but these interface implementations are preserved for potential reuse in future audio backends.
func (p *StdinPlayer) Play(ctx context.Context, audioData []byte) error {
	if len(audioData) == 0 {
		return errors.New("cannot play empty audio data")
	}

	startTime := time.Now()
	chunkSize := len(audioData)
	needsWav := p.RequiresWAVHeader()

	// Prepare buffer with potential WAV header
	bufferSize := chunkSize
	if needsWav {
		bufferSize += 44 // Standard WAV header size
	}
	audioBuffer := bytes.NewBuffer(make([]byte, 0, bufferSize))

	// Write WAV header if needed
	if needsWav {
		wavHeader := helpers.CreateWavHeader(chunkSize, p.config.Channels, p.config.SampleRate, p.config.BitsPerSample)
		if _, err := audioBuffer.Write(wavHeader); err != nil {
			return fmt.Errorf("failed to write WAV header to buffer: %w", err)
		}
	}

	// Write audio data
	if _, err := audioBuffer.Write(audioData); err != nil {
		return fmt.Errorf("failed to write audio data to buffer: %w", err)
	}

	// Prepare command execution
	cmd := exec.CommandContext(ctx, p.cmdName, p.cmdArgs...)
	cmd.Stdin = audioBuffer
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if helpers.IsAudioTraceEnabled() {
		log.Printf("[StdinPlayer] Executing: %q with %d bytes (Header: %t)", p.command, audioBuffer.Len(), needsWav)
	}

	// Run the command and wait for completion or context cancellation
	err := cmd.Run()
	duration := time.Since(startTime)

	if err != nil {
		// Check if the error is due to context cancellation
		if ctx.Err() == context.Canceled {
			log.Printf("[StdinPlayer] Playback cancelled via context for %q after %v", p.command, duration)
			return ctx.Err() // Return context error
		}
		// Log other errors
		errMsg := stderr.String()
		log.Printf("[StdinPlayer] Error executing %q: %v. Duration: %v. Stderr: %s", p.command, err, duration, errMsg)
		return fmt.Errorf("audio player command failed: %w (stderr: %s)", err, errMsg)
	}

	if helpers.IsAudioTraceEnabled() {
		log.Printf("[StdinPlayer] Playback completed OK for %q. Duration: %v, Size: %d bytes", p.command, duration, chunkSize)
	}

	return nil
}

// Cleanup implements the Player interface. No-op for StdinPlayer.
// Deprecated: This implementation is not currently used in the codebase.
// It is kept for compatibility with the Player interface and potential future use.
// The application has moved to a different audio playback approach using the Model in audioplayer.go,
// but these interface implementations are preserved for potential reuse in future audio backends.
func (p *StdinPlayer) Cleanup() error {
	return nil
}

// RequiresWAVHeader checks if the player likely needs a WAV header.
// Currently heuristics based on common player names.
// Deprecated: This implementation is not currently used in the codebase.
// It is kept for compatibility with the Player interface and potential future use.
// The application has moved to a different audio playback approach using the Model in audioplayer.go,
// but these interface implementations are preserved for potential reuse in future audio backends.
func (p *StdinPlayer) RequiresWAVHeader() bool {
	// ffplay and ffmpeg generally handle raw PCM better if format flags are correct,
	// but adding a WAV header is often more robust if flags are uncertain or missing.
	// aplay specifically needs format flags for raw PCM.
	// Let's assume WAV is safer unless it's a known raw PCM player like aplay.
	if p.cmdName == "aplay" {
		// aplay works best with raw PCM and correct flags, not WAV header via stdin.
		return false
	}
	// For ffplay, ffmpeg, paplay, etc., assume WAV header is safer/more compatible via stdin.
	return true
}

// EstimatedLatency provides a rough estimate. Stdin players might have some startup overhead.
// Deprecated: This implementation is not currently used in the codebase.
// It is kept for compatibility with the Player interface and potential future use.
// The application has moved to a different audio playback approach using the Model in audioplayer.go,
// but these interface implementations are preserved for potential reuse in future audio backends.
func (p *StdinPlayer) EstimatedLatency() time.Duration {
	// Estimate based on typical process startup and buffering
	return 50 * time.Millisecond
}
