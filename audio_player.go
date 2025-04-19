package aistudio

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea" // Keep import for tea.Msg types
	"github.com/tmc/aistudio/internal/helpers"
)

// Audio-related errors
var (
	errNoAudioPlayer  = errors.New("audio player command not configured")
	errNoAudioData    = errors.New("no audio data provided")
	errAudioQueueFull = errors.New("audio processing queue is full")
)

// nextChunkInfo holds information about the next chunk to be played
// Used for lookahead buffering to eliminate gaps between chunks
type nextChunkInfo struct {
	chunk       AudioChunk
	isReady     bool
	prepStarted bool
}

// startAudioProcessor starts the goroutine that processes audio chunks sequentially
// Defined here as it relates directly to starting playback logic.
func (m *Model) startAudioProcessor() {
	log.Println("Starting audio processor goroutine")

	// Select the processing method based on the mode
	if m.audioPlaybackMode == AudioPlaybackOnDiskFIFO {
		m.startFIFOAudioProcessor()
	} else {
		m.startDirectAudioProcessor()
	}
}

// startFIFOAudioProcessor processes audio chunks using the configured player
// (either temp file for afplay or stdin pipe for others).
func (m *Model) startFIFOAudioProcessor() {
	log.Println("Starting FIFO audio processor with lookahead buffering")

	// Channel to ensure sequential playback
	audioCompleteCh := make(chan struct{}, 1)
	audioCompleteCh <- struct{}{} // Initial token

	// Channel for signaling next chunk preparation
	prepNextCh := make(chan struct{}, 1)

	// Track the next chunk for lookahead
	var nextChunk nextChunkInfo

	// Goroutine to process audio chunks from the channel
	go func() {
		// Ensure uiUpdateChan exists before using it in goroutine
		uiChan := m.uiUpdateChan
		if uiChan == nil {
			log.Println("[ERROR] uiUpdateChan is nil in startFIFOAudioProcessor goroutine!")
			// Cannot proceed without the channel
			return
		}

		for {
			// Get the next chunk from the channel
			var chunk AudioChunk
			var ok bool

			// If we already have a prepared next chunk, use it
			if nextChunk.isReady {
				chunk = nextChunk.chunk
				nextChunk.isReady = false
				nextChunk.prepStarted = false
				ok = true
				if helpers.IsAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Using pre-buffered chunk for Msg #%d, Size=%d bytes",
						chunk.MessageIndex, len(chunk.Data))
				}
			} else {
				// Otherwise get a new chunk from the channel
				select {
				case chunk, ok = <-m.audioChannel:
					if !ok {
						// Channel closed
						log.Println("Audio processor (FIFO) channel closed, stopping.")
						return
					}
				}
			}

			// Add to UI queue (direct update is okay for this less critical state)
			m.audioQueue = append(m.audioQueue, chunk)
			// Send UI update *message* safely
			uiChan <- audioQueueUpdatedMsg{}

			// --- Send Playback Start Message ---
			if m.showAudioStatus && m.currentState != AppStateQuitting {
				uiChan <- audioPlaybackStartedMsg{chunk: chunk}
			}

			playChunk := chunk

			// Process playback in a separate goroutine
			go func(c AudioChunk) {
				// --- Wait for Playback Slot ---
				if helpers.IsAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Processor Wait Start: Waiting for playback slot. Chunk Size=%d bytes, Msg #%d", len(c.Data), c.MessageIndex)
				}
				<-audioCompleteCh

				// Start preparing the next chunk while this one plays
				// Signal the lookahead goroutine to start preparing
				select {
				case prepNextCh <- struct{}{}:
					if helpers.IsAudioTraceEnabled() {
						log.Printf("[AUDIO_PIPE] Signaled to prepare next chunk while playing Msg #%d", c.MessageIndex)
					}
				default:
					// Non-blocking send, it's okay if no one is listening
				}

				// --- Execute Playback ---
				playerErr := m.playAudioChunkFIFO(c) // Call the actual playback execution function

				// --- Signal Completion ---
				// Send completion message via the UI channel
				uiChan <- audioPlaybackCompletedMsg{chunk: c}
				if playerErr != nil {
					uiChan <- audioPlaybackErrorMsg{err: playerErr}
				}

				// Signal that this slot is free
				if helpers.IsAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Processor Wait End: Releasing playback slot for Msg #%d.", c.MessageIndex)
				}
				audioCompleteCh <- struct{}{}

			}(playChunk)

			// Try to peek at the next chunk for lookahead buffering
			select {
			case <-prepNextCh:
				// Check if there's another chunk available without blocking
				select {
				case nextPossibleChunk, hasMore := <-m.audioChannel:
					if hasMore {
						// We have a next chunk, check if it's from the same message
						if nextPossibleChunk.MessageIndex == playChunk.MessageIndex {
							// Same message, prepare it for immediate playback after current chunk
							if helpers.IsAudioTraceEnabled() {
								log.Printf("[AUDIO_PIPE] Pre-buffering next chunk for same Msg #%d, Size=%d bytes",
									nextPossibleChunk.MessageIndex, len(nextPossibleChunk.Data))
							}
							nextChunk.chunk = nextPossibleChunk
							nextChunk.isReady = true
							nextChunk.prepStarted = true
						} else {
							// Different message, put it back in the channel
							if helpers.IsAudioTraceEnabled() {
								log.Printf("[AUDIO_PIPE] Next chunk is for different Msg #%d (current was #%d), not pre-buffering",
									nextPossibleChunk.MessageIndex, playChunk.MessageIndex)
							}
							m.audioChannel <- nextPossibleChunk
						}
					}
				default:
					// No next chunk available yet, that's okay
					if helpers.IsAudioTraceEnabled() {
						log.Printf("[AUDIO_PIPE] No next chunk available for lookahead after Msg #%d", playChunk.MessageIndex)
					}
				}
			default:
				// No preparation signal yet, continue
			}
		}
	}()
}

// startDirectAudioProcessor processes audio chunks directly (less common mode)
func (m *Model) startDirectAudioProcessor() {
	log.Println("Starting Direct audio processor with lookahead buffering")

	audioCompleteCh := make(chan struct{}, 1)
	audioCompleteCh <- struct{}{}

	// Channel for signaling next chunk preparation
	prepNextCh := make(chan struct{}, 1)

	// Track the next chunk for lookahead
	var nextChunk nextChunkInfo

	go func() {
		uiChan := m.uiUpdateChan
		if uiChan == nil {
			log.Println("[ERROR] uiUpdateChan is nil in startDirectAudioProcessor goroutine!")
			return
		}

		for {
			// Get the next chunk from the channel
			var chunk AudioChunk
			var ok bool

			// If we already have a prepared next chunk, use it
			if nextChunk.isReady {
				chunk = nextChunk.chunk
				nextChunk.isReady = false
				nextChunk.prepStarted = false
				ok = true
				if helpers.IsAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Direct: Using pre-buffered chunk for Msg #%d, Size=%d bytes",
						chunk.MessageIndex, len(chunk.Data))
				}
			} else {
				// Otherwise get a new chunk from the channel
				select {
				case chunk, ok = <-m.audioChannel:
					if !ok {
						// Channel closed
						log.Println("Audio processor (Direct) channel closed, stopping.")
						return
					}
				}
			}

			m.audioQueue = append(m.audioQueue, chunk)
			uiChan <- audioQueueUpdatedMsg{}

			if m.showAudioStatus && m.currentState != AppStateQuitting {
				uiChan <- audioPlaybackStartedMsg{chunk: chunk}
			}

			playChunk := chunk

			go func(c AudioChunk) {
				if helpers.IsAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Processor Direct Wait Start: Waiting for slot. Chunk Size=%d bytes, Msg #%d", len(c.Data), c.MessageIndex)
				}
				<-audioCompleteCh

				// Signal to prepare the next chunk while this one plays
				select {
				case prepNextCh <- struct{}{}:
					if helpers.IsAudioTraceEnabled() {
						log.Printf("[AUDIO_PIPE] Direct: Signaled to prepare next chunk while playing Msg #%d", c.MessageIndex)
					}
				default:
					// Non-blocking send
				}

				playerErr := m.playAudioChunkDirect(c) // Call direct playback execution

				// Signal Completion via UI channel
				uiChan <- audioPlaybackCompletedMsg{chunk: c}
				if playerErr != nil {
					uiChan <- audioPlaybackErrorMsg{err: playerErr}
				}

				if helpers.IsAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Processor Direct Wait End: Releasing slot for Msg #%d.", c.MessageIndex)
				}
				audioCompleteCh <- struct{}{}

			}(playChunk)

			// Try to peek at the next chunk for lookahead buffering
			select {
			case <-prepNextCh:
				// Check if there's another chunk available without blocking
				select {
				case nextPossibleChunk, hasMore := <-m.audioChannel:
					if hasMore {
						// We have a next chunk, check if it's from the same message
						if nextPossibleChunk.MessageIndex == playChunk.MessageIndex {
							// Same message, prepare it for immediate playback after current chunk
							if helpers.IsAudioTraceEnabled() {
								log.Printf("[AUDIO_PIPE] Direct: Pre-buffering next chunk for same Msg #%d, Size=%d bytes",
									nextPossibleChunk.MessageIndex, len(nextPossibleChunk.Data))
							}
							nextChunk.chunk = nextPossibleChunk
							nextChunk.isReady = true
							nextChunk.prepStarted = true
						} else {
							// Different message, put it back in the channel
							if helpers.IsAudioTraceEnabled() {
								log.Printf("[AUDIO_PIPE] Direct: Next chunk is for different Msg #%d (current was #%d), not pre-buffering",
									nextPossibleChunk.MessageIndex, playChunk.MessageIndex)
							}
							m.audioChannel <- nextPossibleChunk
						}
					}
				default:
					// No next chunk available yet, that's okay
					if helpers.IsAudioTraceEnabled() {
						log.Printf("[AUDIO_PIPE] Direct: No next chunk available for lookahead after Msg #%d", playChunk.MessageIndex)
					}
				}
			default:
				// No preparation signal yet, continue
			}
		}
	}()
}

// playAudioChunkFIFO executes the actual playback for a given chunk (FIFO mode).
// Returns an error if playback failed.
func (m *Model) playAudioChunkFIFO(c AudioChunk) error {
	startTime := time.Now()
	chunkSize := len(c.Data)

	if helpers.IsAudioTraceEnabled() {
		log.Printf("[AUDIO_PIPE] Processor Play Start: Slot acquired. Playing chunk Size=%d bytes, Msg #%d at %s.",
			chunkSize, c.MessageIndex, startTime.Format(time.RFC3339Nano))
	}

	// Update model state (direct update assumed safe for these flags from processor)
	currentChunk := c
	currentChunk.StartTime = startTime
	currentChunk.IsProcessing = true
	m.currentAudio = &currentChunk // Direct update - primarily for UI access
	m.isAudioProcessing = true     // Direct update - reflects player state

	var cmd *exec.Cmd
	var err error
	var stderr bytes.Buffer

	// Prepare command based on player type
	if m.playerCmd == "afplay" {
		fileStartTime := time.Now()

		// Optimize file creation for sequential chunks
		tmpFile, errCreate := os.CreateTemp("", fmt.Sprintf("aistudio-audio-%d-*.wav", c.MessageIndex))
		if errCreate != nil {
			err = fmt.Errorf("failed to create afplay temp file: %w", errCreate)
			goto HandleErrorFIFO // Use goto for centralized error handling
		}
		tempFilePath := tmpFile.Name()
		defer os.Remove(tempFilePath)

		// Use a larger buffer for better I/O performance
		bufWriter := bufio.NewWriterSize(tmpFile, 32*1024) // 32KB buffer
		wavHeader := helpers.CreateWavHeader(chunkSize, 1, audioSampleRate, 16)
		_, errHead := bufWriter.Write(wavHeader)
		_, errData := bufWriter.Write(c.Data)
		errFlush := bufWriter.Flush()
		errClose := tmpFile.Close()

		if err = errors.Join(errHead, errData, errFlush, errClose); err != nil {
			err = fmt.Errorf("failed writing afplay temp file %s: %w", tempFilePath, err)
			goto HandleErrorFIFO
		}
		fileCreateTime := time.Since(fileStartTime)
		if helpers.IsAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Processor afplay: Created temp file %s (%d bytes) in %v", tempFilePath, chunkSize+44, fileCreateTime)
		}

		// Use higher priority for smoother playback
		cmd = exec.Command("afplay", "-q", "1", tempFilePath)
		cmd.Stderr = &stderr

	} else if m.playerCmd != "" {
		// Pre-allocate buffer with capacity to avoid resizing
		audioBuffer := bytes.NewBuffer(make([]byte, 0, chunkSize+44)) // +44 for possible WAV header
		needsWavHeader := false
		playerBaseCmd := ""
		parts := strings.Fields(m.playerCmd)
		if len(parts) > 0 {
			playerBaseCmd = parts[0]
			if playerBaseCmd == "ffplay" || playerBaseCmd == "ffmpeg" {
				needsWavHeader = true
			}
		}

		if needsWavHeader {
			wavHeader := helpers.CreateWavHeader(chunkSize, 1, audioSampleRate, 16)
			if _, errW := audioBuffer.Write(wavHeader); errW != nil {
				err = fmt.Errorf("failed to write WAV header: %w", errW)
				goto HandleErrorFIFO
			}
		}
		if _, errW := audioBuffer.Write(c.Data); errW != nil {
			err = fmt.Errorf("failed to write audio data to buffer: %w", errW)
			goto HandleErrorFIFO
		}

		if helpers.IsAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Processor Stdin: Executing %q with %d bytes (Header: %t)", m.playerCmd, audioBuffer.Len(), needsWavHeader)
		}
		cmd = exec.Command(parts[0], parts[1:]...)
		cmd.Stdin = audioBuffer
		cmd.Stderr = &stderr
	} else {
		err = errNoAudioPlayer
		goto HandleErrorFIFO
	}

	// Run the command
	err = cmd.Run() // Blocks

HandleErrorFIFO: // Centralized error handling/state update
	endTime := time.Now()
	duration := endTime.Sub(startTime)
	status := "OK"
	errMsg := ""
	finalErr := err // Store original error to return

	if err != nil {
		status = fmt.Sprintf("ERROR (%v)", err)
		errMsg = fmt.Sprintf(" Stderr: %s", stderr.String())
	}

	if helpers.IsAudioTraceEnabled() {
		log.Printf("[AUDIO_PIPE] Processor Play End: Finished chunk Size=%d bytes, Msg #%d. Status: %s. Duration: %s.%s",
			chunkSize, c.MessageIndex, status, duration, errMsg)
	}

	// Update model state (direct update OK here, as it's read by UI loop)
	m.isAudioProcessing = false
	if m.currentAudio != nil && bytes.Equal(m.currentAudio.Data, c.Data) {
		m.currentAudio.IsProcessing = false
		m.currentAudio.IsComplete = true
	}

	return finalErr // Return the execution error status
}

// playAudioChunkDirect executes playback for Direct mode.
// Returns an error if playback failed.
func (m *Model) playAudioChunkDirect(c AudioChunk) error {
	startTime := time.Now()
	chunkSize := len(c.Data)

	if helpers.IsAudioTraceEnabled() {
		log.Printf("[AUDIO_PIPE] Processor Direct Play Start: Slot acquired. Playing chunk Size=%d bytes, Msg #%d at %s.",
			chunkSize, c.MessageIndex, startTime.Format(time.RFC3339Nano))
	}

	// Update model state
	currentChunk := c
	currentChunk.StartTime = startTime
	currentChunk.IsProcessing = true
	m.currentAudio = &currentChunk
	m.isAudioProcessing = true

	var err error
	var stderr bytes.Buffer

	// Execute playback
	if m.playerCmd == "afplay" {
		err = m.playWithAfplayDirect(c.Data) // This logs internally
	} else if m.playerCmd != "" {
		parts := strings.Fields(m.playerCmd)
		if len(parts) > 0 {
			cmdName := parts[0]
			cmdArgs := parts[1:]

			// Pre-allocate buffer with capacity to avoid resizing
			audioBuffer := bytes.NewBuffer(make([]byte, 0, chunkSize))
			if _, errW := audioBuffer.Write(c.Data); errW != nil {
				return fmt.Errorf("failed to write audio data to direct buffer: %w", errW)
			}

			if helpers.IsAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Processor Direct Stdin: Executing %q with %d bytes (Raw PCM)", m.playerCmd, audioBuffer.Len())
			}
			cmd := exec.Command(cmdName, cmdArgs...)
			cmd.Stdin = audioBuffer
			cmd.Stderr = &stderr
			err = cmd.Run()
		} else {
			err = errors.New("invalid player command format")
		}
	} else {
		err = errNoAudioPlayer
	}

	// Log completion
	endTime := time.Now()
	duration := endTime.Sub(startTime)
	status := "OK"
	errMsg := ""
	finalErr := err // Store original error
	if err != nil {
		status = fmt.Sprintf("ERROR (%v)", err)
		errMsg = fmt.Sprintf(" Stderr: %s", stderr.String())
	}
	if helpers.IsAudioTraceEnabled() {
		log.Printf("[AUDIO_PIPE] Processor Direct Play End: Finished chunk Size=%d bytes, Msg #%d. Status: %s. Duration: %s.%s",
			chunkSize, c.MessageIndex, status, duration, errMsg)
	}

	// Update model state
	m.isAudioProcessing = false
	if m.currentAudio != nil && bytes.Equal(m.currentAudio.Data, c.Data) {
		m.currentAudio.IsProcessing = false
		m.currentAudio.IsComplete = true
	}

	return finalErr // Return execution error
}

// playAudioCmd returns a command that handles queueing audio chunks for playback.
// It determines if a chunk is large/replay (play directly) or small (use consolidation).
func (m *Model) playAudioCmd(audioData []byte, text ...string) tea.Cmd {
	return func() tea.Msg {
		if m.playerCmd == "" {
			// Send error message back to UI loop via channel
			m.uiUpdateChan <- audioPlaybackErrorMsg{err: errNoAudioPlayer}
			return nil
		}
		if len(audioData) == 0 {
			return nil // Ignore empty audio data
		}

		associatedText := ""
		if len(text) > 0 {
			associatedText = text[0]
		}
		estimatedDuration := float64(len(audioData)) / 48000.0

		// --- Determine message association and if it's a replay ---
		messageIdx := -1
		isReplay := false
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].HasAudio && len(m.messages[i].AudioData) > 0 && bytes.Equal(m.messages[i].AudioData, audioData) {
				messageIdx = i
				isReplay = m.messages[i].IsPlayed
				break
			}
		}

		// --- Decide playback strategy ---
		isLargeChunk := len(audioData) >= minBufferSizeForPlayback

		if isReplay || isLargeChunk {
			// Play directly by sending straight to channel
			reason := "Replay"
			if isLargeChunk && !isReplay {
				reason = "LargeChunk"
			} else if isLargeChunk && isReplay {
				reason = "LargeReplay"
			}
			if helpers.IsAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Playback Queue Direct (%s): Size=%d bytes, Msg #%d", reason, len(audioData), messageIdx)
			}

			chunk := AudioChunk{
				Data:         audioData,
				Text:         associatedText,
				StartTime:    time.Time{}, // Set by processor
				Duration:     estimatedDuration,
				MessageIndex: messageIdx, // Associate with message
			}

			if m.audioChannel != nil {
				select {
				case m.audioChannel <- chunk:
					if helpers.IsAudioTraceEnabled() {
						log.Printf("[AUDIO_PIPE] Playback Queue Direct: Sent %d bytes to channel.", len(chunk.Data))
					}
					// Return message indicating item was added to queue
					return audioQueueUpdatedMsg{}
				default:
					log.Printf("[AUDIO_PIPE] Playback Queue Direct: ERROR - Channel full, dropping chunk %d bytes", len(audioData))
					// Send error message back to UI loop
					m.uiUpdateChan <- audioPlaybackErrorMsg{err: errAudioQueueFull}
					return nil
				}
			} else {
				log.Printf("[AUDIO_PIPE] Playback Queue Direct: WARNING - Audio channel nil, cannot play.")
				m.uiUpdateChan <- audioPlaybackErrorMsg{err: errors.New("audio channel not initialized")}
				return nil
			}

		} else {
			// Small chunk, use consolidation buffer
			if messageIdx < 0 { // Find index if not found via exact match (likely new stream data)
				if len(m.messages) > 0 {
					messageIdx = len(m.messages) - 1 // Associate with the latest message
					if helpers.IsAudioTraceEnabled() {
						log.Printf("[AUDIO_PIPE] Playback Queue Consolidate: Using fallback Msg #%d for small chunk (%d bytes)", messageIdx, len(audioData))
					}
				} else {
					log.Printf("[AUDIO_PIPE] Playback Queue Consolidate: Error - Cannot consolidate small chunk (%d bytes) with no existing messages.", len(audioData))
					return nil
				}
			}

			if helpers.IsAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Playback Queue Consolidate: Small chunk (%d bytes) for Msg #%d -> using consolidation buffer.", len(audioData), messageIdx)
			}
			// consolidateAndPlayAudio returns a command (potentially nil or flush command)
			// We execute the command immediately to get the resulting message (or nil)
			return m.consolidateAndPlayAudio(audioData, associatedText, messageIdx)()
		}
	}
}

// playWithAfplay handles audio playback using macOS afplay by creating a temp file.
// Returns a tea.Cmd for use in the tea framework.
func (m *Model) playWithAfplay(audioData []byte) tea.Cmd {
	return func() tea.Msg {
		err := m.playWithAfplayDirect(audioData) // Call the direct function
		if err != nil {
			// Send error message back to UI loop via channel
			m.uiUpdateChan <- audioPlaybackErrorMsg{err: err}
		}
		// Let the processor loop send the completion message
		return nil
	}
}

// playWithAfplayDirect executes afplay directly, blocking until completion.
// Used internally by the audio processor goroutine or playWithAfplay command.
// Includes detailed logging. Returns error on failure.
func (m *Model) playWithAfplayDirect(audioData []byte) error {
	audioSize := len(audioData)
	startTime := time.Now() // Overall start time for this operation
	estimatedDuration := float64(audioSize) / 48000.0

	if helpers.IsAudioTraceEnabled() {
		log.Printf("[AFPLAY] Executing: Size=%d bytes (~%.2fs)", audioSize, estimatedDuration)
	}

	// 1. Create Temp File - include message index in filename if available
	fileStartTime := time.Now()
	messageIdx := -1
	if m.currentAudio != nil {
		messageIdx = m.currentAudio.MessageIndex
	}

	// Use message index in filename for better organization
	tmpFilePattern := "aistudio-audio-*.wav"
	if messageIdx >= 0 {
		tmpFilePattern = fmt.Sprintf("aistudio-audio-%d-*.wav", messageIdx)
	}

	tmpFile, err := os.CreateTemp("", tmpFilePattern)
	if err != nil {
		return fmt.Errorf("afplay failed to create temp file: %w", err)
	}
	tempFilePath := tmpFile.Name()
	defer func() {
		if removeErr := os.Remove(tempFilePath); removeErr != nil {
			log.Printf("[AFPLAY WARNING] Failed to remove temp file %s: %v", tempFilePath, removeErr)
		}
	}()

	// 2. Write Header and Data with larger buffer for better I/O performance
	bufWriter := bufio.NewWriterSize(tmpFile, 32*1024) // 32KB buffer
	wavHeader := helpers.CreateWavHeader(audioSize, 1, audioSampleRate, 16)
	_, errHead := bufWriter.Write(wavHeader)
	_, errData := bufWriter.Write(audioData)
	errFlush := bufWriter.Flush()
	errClose := tmpFile.Close() // Close file before playing

	if err = errors.Join(errHead, errData, errFlush, errClose); err != nil {
		log.Printf("[AFPLAY ERROR] Failed writing temp file %s: %v", tempFilePath, err)
		return fmt.Errorf("afplay failed writing file: %w", err)
	}
	fileWriteDuration := time.Since(fileStartTime)

	// 3. Execute afplay with higher quality setting for smoother playback
	cmd := exec.Command("afplay", "-q", "1", tempFilePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	playStartTime := time.Now()
	if helpers.IsAudioTraceEnabled() {
		log.Printf("[AFPLAY] Starting playback command for %s (FileWrite: %v)", tempFilePath, fileWriteDuration)
	}
	err = cmd.Run() // Blocks until playback is complete
	playDuration := time.Since(playStartTime)
	totalDuration := time.Since(startTime)

	// 4. Log Results & Return Error Status
	if err != nil {
		log.Printf("[AFPLAY ERROR] Playback failed: %v (stderr: %s). PlayDuration: %v, TotalDuration: %v",
			err, stderr.String(), playDuration, totalDuration)
		return fmt.Errorf("afplay execution failed: %w", err) // Return the error
	}

	if helpers.IsAudioTraceEnabled() {
		actualPlayRate := 0.0
		if playDuration.Seconds() > 0 {
			actualPlayRate = float64(audioSize) / playDuration.Seconds() / 1024
		}
		throughputRate := 0.0
		if totalDuration.Seconds() > 0 {
			throughputRate = float64(audioSize) / totalDuration.Seconds() / 1024
		}
		log.Printf("[AFPLAY] Playback completed OK. Size=%d, PlayDuration=%v (%.2f KB/s), FileWrite=%v, Total=%v (%.2f KB/s)",
			audioSize, playDuration, actualPlayRate, fileWriteDuration, totalDuration, throughputRate)
	}
	return nil // Success
}
