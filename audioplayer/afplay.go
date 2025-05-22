package audioplayer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/tmc/aistudio/internal/helpers" // Use internal helper
)

// AfplayPlayer plays audio using macOS's `afplay` command by writing to a temporary WAV file.
type AfplayPlayer struct {
	config Config
}

// NewAfplayPlayer creates a new AfplayPlayer with the given configuration.
func NewAfplayPlayer(config Config) *AfplayPlayer {
	if config.SampleRate == 0 {
		config = DefaultConfig
	}
	return &AfplayPlayer{
		config: config,
	}
}

// Play implements the Player interface.
func (p *AfplayPlayer) Play(ctx context.Context, audioData []byte) error {
	if len(audioData) == 0 {
		return errors.New("cannot play empty audio data")
	}

	startTime := time.Now()
	chunkSize := len(audioData)

	// Always log for debugging
	log.Printf("[AfplayPlayer CRITICAL] Starting playback of %d bytes with config=%+v", chunkSize, p.config)

	// 1. Create Temp File
	fileStartTime := time.Now()
	tmpFile, err := os.CreateTemp("", "aistudio-afplay-*.wav")
	if err != nil {
		log.Printf("[AfplayPlayer CRITICAL] Failed to create temp file: %v", err)
		return fmt.Errorf("afplay failed to create temp file: %w", err)
	}
	tempFilePath := tmpFile.Name()
	// Schedule cleanup using defer, but check error
	defer func() {
		if removeErr := os.Remove(tempFilePath); removeErr != nil {
			// Log error if removal fails, but don't return it as the primary error
			log.Printf("[AfplayPlayer WARNING] Failed to remove temp file %s: %v", tempFilePath, removeErr)
		}
	}()

	// 2. Write Header and Data (use buffered writer)
	bufWriter := bufio.NewWriterSize(tmpFile, 32*1024) // 32KB buffer
	wavHeader := helpers.CreateWavHeader(chunkSize, p.config.Channels, p.config.SampleRate, p.config.BitsPerSample)
	_, errHead := bufWriter.Write(wavHeader)
	_, errData := bufWriter.Write(audioData)
	errFlush := bufWriter.Flush()
	errClose := tmpFile.Close() // Close file before playing

	log.Printf("[AfplayPlayer CRITICAL] Created temp file at: %s", tempFilePath)

	if err = errors.Join(errHead, errData, errFlush, errClose); err != nil {
		log.Printf("[AfplayPlayer CRITICAL] Failed writing temp file %s: %v", tempFilePath, err)
		return fmt.Errorf("afplay failed writing temp file %s: %w", tempFilePath, err)
	}
	fileWriteDuration := time.Since(fileStartTime)

	// 3. Execute afplay command with context
	// Use -q 1 for higher quality/priority if needed, though impact varies.
	cmd := exec.CommandContext(ctx, "afplay", "-q", "1", tempFilePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	playStartTime := time.Now()

	// Always log command execution for debugging
	log.Printf("[AfplayPlayer CRITICAL] Executing: afplay -q 1 %s (Size: %d bytes, FileWrite: %v)", tempFilePath, chunkSize, fileWriteDuration)

	// Run the command and wait for completion or context cancellation
	err = cmd.Run()
	playDuration := time.Since(playStartTime)
	totalDuration := time.Since(startTime)

	// 4. Handle results
	if err != nil {
		// Check if the error is due to context cancellation
		if ctx.Err() == context.Canceled {
			log.Printf("[AfplayPlayer CRITICAL] Playback cancelled via context for %s after %v (PlayDuration: %v)", tempFilePath, totalDuration, playDuration)
			return ctx.Err() // Return context error
		}
		// Log other errors
		errMsg := stderr.String()
		log.Printf("[AfplayPlayer CRITICAL] Playback failed for %s: %v (stderr: %s). PlayDuration: %v, TotalDuration: %v",
			tempFilePath, err, errMsg, playDuration, totalDuration)
		return fmt.Errorf("afplay execution failed: %w (stderr: %s)", err, errMsg)
	}

	// Always log successful playback for debugging
	log.Printf("[AfplayPlayer CRITICAL] Playback completed OK for %s. Size=%d, PlayDuration=%v, FileWrite=%v, Total=%v",
		tempFilePath, chunkSize, playDuration, fileWriteDuration, totalDuration)

	return nil
}

// Cleanup implements the Player interface. No-op needed as temp files are handled by Play.
func (p *AfplayPlayer) Cleanup() error {
	return nil
}

// RequiresWAVHeader indicates that afplay needs a WAV file (header included).
func (p *AfplayPlayer) RequiresWAVHeader() bool {
	return true // afplay operates on files, expects standard formats like WAV
}

// EstimatedLatency provides a rough estimate. File I/O adds latency.
func (p *AfplayPlayer) EstimatedLatency() time.Duration {
	// Estimate includes temp file creation, write, and afplay startup
	return 100 * time.Millisecond
}
