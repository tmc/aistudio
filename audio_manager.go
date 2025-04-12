package aistudio

import (
	"errors"
	"log"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Audio buffering window values, thresholds are defined in constants.go

// Reduced buffer window for same-message chunks to minimize gaps
const sameMessageBufferWindow = 200 * time.Millisecond

// consolidateAndPlayAudio adds audio data to the buffer and triggers playback when ready
func (m *Model) consolidateAndPlayAudio(audioData []byte, messageText string, messageIdx int) tea.Cmd {
	chunkSize := len(audioData)
	chunkReceiveTime := time.Now()

	// Add to our recent chunk tracking for adaptation
	m.trackChunkStats(chunkSize, chunkReceiveTime) // Defined below

	// Check if we're in active playback of the same message
	isActivePlayback := m.isAudioProcessing && m.currentAudio != nil &&
		m.currentAudio.MessageIndex == messageIdx

	// Ensure message index is valid
	if messageIdx < 0 || messageIdx >= len(m.messages) {
		log.Printf("[AUDIO CONSOLIDATE] Error: Invalid message index %d", messageIdx)
		// Fallback: Associate with the *last* message if index is bad.
		if len(m.messages) > 0 {
			messageIdx = len(m.messages) - 1
			log.Printf("[AUDIO CONSOLIDATE] Warning: Using last message index %d as fallback", messageIdx)
		} else {
			log.Printf("[AUDIO CONSOLIDATE] Error: No messages exist, cannot consolidate audio.")
			return nil // Cannot proceed without a message to associate with
		}
	}

	// Check if we're continuing existing buffer or starting a new one
	if len(m.consolidatedAudioData) == 0 || m.bufferMessageIdx != messageIdx {
		// --- Start a new buffer ---
		var prevCmd tea.Cmd
		if len(m.consolidatedAudioData) > 0 && m.bufferMessageIdx >= 0 { // Flush previous if exists
			if isAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Consolidate: Flushing previous buffer for msg #%d before starting new for #%d",
					m.bufferMessageIdx, messageIdx)
			}
			prevCmd = m.flushAudioBuffer() // Get flush command for previous buffer
		}

		// Initialize new buffer state
		if isAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Buffer Start: New buffer for Msg #%d at %s. Chunk=%d bytes, Window=%v",
				messageIdx, chunkReceiveTime.Format(time.RFC3339Nano), chunkSize, m.currentBufferWindow)
		}
		m.bufferStartTime = chunkReceiveTime
		m.bufferMessageIdx = messageIdx
		m.consolidatedAudioData = nil // Ensure it's empty

		// Set the flush timer - use shorter window if we're already playing this message
		// to reduce gaps between segments
		bufferWindow := m.currentBufferWindow
		if isActivePlayback {
			bufferWindow = sameMessageBufferWindow
			if isAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Buffer Priority: Using shorter window %v for active playback of Msg #%d",
					bufferWindow, messageIdx)
			}
		}

		if m.bufferTimer != nil {
			m.bufferTimer.Stop()
		}
		flushTime := chunkReceiveTime.Add(bufferWindow)
		if isAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Buffer Timer: Setting timeout for %s (at %s) for Msg #%d",
				bufferWindow, flushTime.Format(time.RFC3339Nano), messageIdx)
		}
		m.bufferTimer = time.AfterFunc(bufferWindow, func() {
			// Timer expired - send message to main loop safely via channel
			if len(m.consolidatedAudioData) > 0 {
				if isAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Buffer Flush Trigger: Timer expired for Msg #%d. Sending flush msg.", m.bufferMessageIdx)
				}
				// Send message safely via uiUpdateChan
				m.uiUpdateChan <- flushAudioBufferMsg{} // Send to channel
			} else {
				if isAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Buffer Flush Trigger: Timer expired for Msg #%d, but buffer is empty. No flush needed.", m.bufferMessageIdx)
				}
			}
		})

		// Add the current chunk to the *new* buffer *after* setting up state
		m.consolidatedAudioData = append(m.consolidatedAudioData, audioData...)
		if isAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Buffer Add (New): Msg #%d, Added=%d bytes, NewTotal=%d bytes",
				messageIdx, chunkSize, len(m.consolidatedAudioData))
		}

		// If we needed to flush a *previous* buffer, execute that command now
		if prevCmd != nil {
			return prevCmd
		}

		// For active playback of same message, use a lower size threshold to reduce gaps
		// Adjust thresholds if using the less efficient afplay
		sizeThreshold := minBufferSizeForPlayback
		if m.playerCmd == "afplay" {
			sizeThreshold = minBufferSizeForPlayback * 2 // Double threshold for afplay
			if isAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Buffer Priority: Using increased size threshold %d bytes for afplay on Msg #%d",
					sizeThreshold, messageIdx)
			}
		}
		
		if isActivePlayback {
			// Use a smaller threshold for chunks of the same message that's playing
			sizeThreshold = continuousPlaybackBufferSize
			// Also adjust for afplay during active playback, but less aggressively
			if m.playerCmd == "afplay" {
				sizeThreshold = continuousPlaybackBufferSize * 3 / 2 // 1.5x threshold for afplay during active playback
				if isAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Buffer Priority: Using adjusted size threshold %d bytes for afplay during active playback of Msg #%d",
						sizeThreshold, messageIdx)
				}
			} else if isAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Buffer Priority: Using lower size threshold %d bytes for active Msg #%d",
					sizeThreshold, messageIdx)
			}
		}

		// Check if the *newly added* chunk triggers a size flush
		if len(m.consolidatedAudioData) >= sizeThreshold {
			if isAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Buffer Flush Trigger: Size threshold reached immediately on new buffer for Msg #%d (%d >= %d).",
					messageIdx, len(m.consolidatedAudioData), sizeThreshold)
			}
			return m.flushAudioBuffer()
		}

	} else {
		// --- Add to existing buffer ---
		bufferAge := time.Since(m.bufferStartTime)

		// Adapt window if needed - but not for active playback (we want consistency)
		if !isActivePlayback && chunkSize < adaptiveBufferThreshold && bufferAge < m.currentBufferWindow/2 {
			m.adaptBufferWindow(chunkSize, true) // Defined below
		}

		// Use shorter window if we're already playing this message to reduce gaps
		bufferWindow := m.currentBufferWindow
		if isActivePlayback {
			bufferWindow = sameMessageBufferWindow
			if isAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Buffer Priority: Using shorter window %v for active playback of Msg #%d",
					bufferWindow, messageIdx)
			}
		}

		// Reset the timer
		if m.bufferTimer != nil {
			m.bufferTimer.Stop()
		}
		flushTime := time.Now().Add(bufferWindow)
		if isAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Buffer Timer: Resetting timeout for %s (at %s) for Msg #%d due to new chunk",
				bufferWindow, flushTime.Format(time.RFC3339Nano), messageIdx)
		}
		m.bufferTimer = time.AfterFunc(bufferWindow, func() {
			// Timer expired - send message to main loop safely
			if len(m.consolidatedAudioData) > 0 {
				if isAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Buffer Flush Trigger: Timer (after reset) expired for Msg #%d. Sending flush msg.", m.bufferMessageIdx)
				}
				// Send message safely via uiUpdateChan
				m.uiUpdateChan <- flushAudioBufferMsg{} // Send to channel
			} else {
				if isAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Buffer Flush Trigger: Timer (after reset) expired for Msg #%d, but buffer empty.", m.bufferMessageIdx)
				}
			}
		})

		// Add data to buffer
		prevSize := len(m.consolidatedAudioData)
		m.consolidatedAudioData = append(m.consolidatedAudioData, audioData...)
		newSize := len(m.consolidatedAudioData)
		if isAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Buffer State: Msg #%d grew %d -> %d bytes (+%d) after %v",
				messageIdx, prevSize, newSize, chunkSize, bufferAge)
		}

		// For active playback of same message, use a lower size threshold to reduce gaps
		// Adjust thresholds if using the less efficient afplay
		sizeThreshold := minBufferSizeForPlayback
		if m.playerCmd == "afplay" {
			sizeThreshold = minBufferSizeForPlayback * 2 // Double threshold for afplay
			if isAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Buffer Priority: Using increased size threshold %d bytes for afplay on Msg #%d",
					sizeThreshold, messageIdx)
			}
		}
		
		if isActivePlayback {
			// Use a smaller threshold for chunks of the same message that's playing
			sizeThreshold = continuousPlaybackBufferSize
			// Also adjust for afplay during active playback, but less aggressively
			if m.playerCmd == "afplay" {
				sizeThreshold = continuousPlaybackBufferSize * 3 / 2 // 1.5x threshold for afplay during active playback
				if isAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Buffer Priority: Using adjusted size threshold %d bytes for afplay during active playback of Msg #%d",
						sizeThreshold, messageIdx)
				}
			} else if isAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Buffer Priority: Using lower size threshold %d bytes for active Msg #%d",
					sizeThreshold, messageIdx)
			}
		}

		// Check for size-based flush
		if newSize >= sizeThreshold {
			if isAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Buffer Flush Trigger: Size threshold reached (%d >= %d) for Msg #%d.",
					newSize, sizeThreshold, messageIdx)
			}
			if m.consecutiveSmallChunks > 3 && !isActivePlayback {
				m.adaptBufferWindow(chunkSize, false)
			}
			return m.flushAudioBuffer() // Execute flush immediately
		}
	}

	return nil // No command needed yet (timer running or buffer too small)
}

// trackChunkStats adds a chunk to our recent tracking data for adaptive buffering
func (m *Model) trackChunkStats(chunkSize int, timestamp time.Time) {
	if chunkSize < adaptiveBufferThreshold {
		m.consecutiveSmallChunks++
	} else {
		m.consecutiveSmallChunks = 0
	}

	m.recentChunkSizes = append(m.recentChunkSizes, chunkSize)
	m.recentChunkTimes = append(m.recentChunkTimes, timestamp)

	maxTracked := 10
	if len(m.recentChunkSizes) > maxTracked {
		m.recentChunkSizes = m.recentChunkSizes[len(m.recentChunkSizes)-maxTracked:]
		m.recentChunkTimes = m.recentChunkTimes[len(m.recentChunkTimes)-maxTracked:]
	}

	if isAudioTraceEnabled() && len(m.recentChunkSizes) >= 3 {
		var totalInterval time.Duration
		validIntervals := 0
		for i := 1; i < len(m.recentChunkTimes); i++ {
			interval := m.recentChunkTimes[i].Sub(m.recentChunkTimes[i-1])
			if interval < 2*time.Second { // Ignore large gaps
				totalInterval += interval
				validIntervals++
			}
		}
		avgInterval := time.Duration(0)
		if validIntervals > 0 {
			avgInterval = totalInterval / time.Duration(validIntervals)
		}

		var totalSize int
		for _, size := range m.recentChunkSizes {
			totalSize += size
		}
		avgSize := 0
		if len(m.recentChunkSizes) > 0 {
			avgSize = totalSize / len(m.recentChunkSizes)
		}

		log.Printf("[AUDIO_PIPE] Stats: AvgSize=%d bytes, AvgInterval=%v (%d valid), ConsecutiveSmall=%d",
			avgSize, avgInterval, validIntervals, m.consecutiveSmallChunks)
	}
}

// adaptBufferWindow adjusts the buffer window based on chunk patterns
func (m *Model) adaptBufferWindow(chunkSize int, increase bool) {
	oldWindow := m.currentBufferWindow

	if increase {
		if m.consecutiveSmallChunks >= 3 {
			newWindow := time.Duration(float64(m.currentBufferWindow) * 1.2) // Increase by 20%
			if newWindow > maxAudioBufferingWindow {
				newWindow = maxAudioBufferingWindow
			}
			m.currentBufferWindow = newWindow
		}
	} else {
		if m.currentBufferWindow > initialAudioBufferingWindow {
			newWindow := time.Duration(float64(m.currentBufferWindow) * 0.9) // Decrease by 10%
			if newWindow < initialAudioBufferingWindow {
				newWindow = initialAudioBufferingWindow
			}
			m.currentBufferWindow = newWindow
		}
	}

	if isAudioTraceEnabled() && m.currentBufferWindow != oldWindow {
		dir := "Increased"
		if !increase {
			dir = "Decreased"
		}
		log.Printf("[AUDIO_PIPE] Adapt Window: %s %v -> %v (ConsecutiveSmall: %d)", dir, oldWindow, m.currentBufferWindow, m.consecutiveSmallChunks)
	}
}

// flushAudioBuffer prepares the consolidated audio buffer for playback and returns the command.
func (m *Model) flushAudioBuffer() tea.Cmd {
	now := time.Now()
	if len(m.consolidatedAudioData) == 0 {
		if isAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Flush Skip: Buffer empty at %s", now.Format(time.RFC3339Nano))
		}
		return nil
	}

	bufferSize := len(m.consolidatedAudioData)
	bufferAge := time.Since(m.bufferStartTime)
	flushMsgIdx := m.bufferMessageIdx // Capture index before reset

	// Check if this is continuing an active playback (same message)
	isActivePlayback := m.isAudioProcessing && m.currentAudio != nil &&
		m.currentAudio.MessageIndex == flushMsgIdx

	// Check throttle condition - but bypass throttling for active playback to reduce gaps
	timeSinceLastFlush := now.Sub(m.lastFlushTime)
	bufferIsSmallEnoughToDelay := bufferSize < minBufferSizeForPlayback

	// Use a shorter minimum time between flushes for active playback
	// Also increase the minimum flush interval slightly for afplay due to its overhead
	minFlushInterval := minTimeBetweenFlushes
	if m.playerCmd == "afplay" && !isActivePlayback {
		// Increase base interval slightly for afplay when not actively playing back
		// the same message (to avoid frequent file creation)
		minFlushInterval = time.Duration(float64(minTimeBetweenFlushes) * 1.5)
		if isAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Flush Interval: Using increased interval %v for afplay on Msg #%d",
				minFlushInterval, flushMsgIdx)
		}
	} else if isActivePlayback {
		minFlushInterval = minTimeBetweenFlushes / 2
		if isAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Flush Interval: Using decreased interval %v for active playback on Msg #%d",
				minFlushInterval, flushMsgIdx)
		}
	}

	if !m.lastFlushTime.IsZero() && timeSinceLastFlush < minFlushInterval &&
		bufferIsSmallEnoughToDelay && !isActivePlayback {
		if isAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Flush Skip: Throttled. LastFlush=%s ago (< %s), BufferSize=%d bytes. Msg #%d",
				timeSinceLastFlush, minFlushInterval, bufferSize, flushMsgIdx)
		}
		// Restart the timer for a bit later
		if m.bufferTimer != nil {
			m.bufferTimer.Stop()
		}
		waitTime := minTimeBetweenFlushes - timeSinceLastFlush
		if isAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Flush Retry: Scheduling retry in %s for Msg #%d", waitTime, flushMsgIdx)
		}
		m.bufferTimer = time.AfterFunc(waitTime, func() {
			// Timer expired - send message safely
			if len(m.consolidatedAudioData) > 0 {
				if isAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Flush Retry Trigger: Timer fired for Msg #%d. Sending flush msg.", m.bufferMessageIdx)
				}
				m.uiUpdateChan <- flushAudioBufferMsg{} // Send to channel
			}
		})
		return nil // No command *yet*, timer rescheduled
	}

	// --- Proceeding with Flush ---
	if isAudioTraceEnabled() {
		log.Printf("[AUDIO_PIPE] Flushing Buffer: Msg #%d, Size=%d bytes, Age=%v (LastFlush: %s ago)",
			flushMsgIdx, bufferSize, bufferAge, timeSinceLastFlush)
	}

	// Get a copy of the data
	audioData := make([]byte, bufferSize)
	copy(audioData, m.consolidatedAudioData)

	// Get message text (ensure index is valid)
	messageText := ""
	if flushMsgIdx >= 0 && flushMsgIdx < len(m.messages) {
		messageText = m.messages[flushMsgIdx].Content
		// Update the *message's* canonical AudioData with the complete flushed buffer
		if !m.messages[flushMsgIdx].HasAudio || len(m.messages[flushMsgIdx].AudioData) < bufferSize {
			if isAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Flush: Updating message #%d final AudioData to %d bytes", flushMsgIdx, bufferSize)
			}
			m.messages[flushMsgIdx].HasAudio = true
			m.messages[flushMsgIdx].AudioData = audioData
		}
	} else {
		log.Printf("[AUDIO_PIPE] Flush Warning: Invalid message index %d during flush", flushMsgIdx)
	}

	// Update flush time *before* returning command
	m.lastFlushTime = now

	// Reset the buffer state
	m.consolidatedAudioData = nil
	if m.bufferTimer != nil {
		m.bufferTimer.Stop()
		m.bufferTimer = nil
	}
	m.bufferMessageIdx = -1
	m.bufferStartTime = time.Time{}

	// Return the command to queue this flushed chunk for playback
	if isAudioTraceEnabled() {
		log.Printf("[AUDIO_PIPE] Queueing Flushed: Sending %d bytes from Msg #%d to playback channel", bufferSize, flushMsgIdx)
	}

	// Set priority flag for chunks from the same message that's currently playing
	// This helps the audio_player.go lookahead buffering system identify related chunks
	isPriority := m.isAudioProcessing && m.currentAudio != nil &&
		m.currentAudio.MessageIndex == flushMsgIdx

	if isPriority && isAudioTraceEnabled() {
		log.Printf("[AUDIO_PIPE] Priority Chunk: Marking chunk from Msg #%d as priority for continuous playback", flushMsgIdx)
	}

	return m.playConsolidatedAudio(audioData, messageText, flushMsgIdx)
}

// playConsolidatedAudio creates the command func to send already-consolidated audio to the audio channel.
func (m *Model) playConsolidatedAudio(audioData []byte, text string, messageIdx int) tea.Cmd {
	return func() tea.Msg { // This func is executed by Bubble Tea when the command runs
		if m.playerCmd == "" {
			return audioPlaybackErrorMsg{err: errors.New("audio player command not configured")}
		}
		if len(audioData) == 0 {
			if isAudioTraceEnabled() {
				log.Printf("[AUDIO_PIPE] Playback Queue Consolidated: Skip empty chunk for Msg #%d", messageIdx)
			}
			return nil // Send nothing if no data
		}

		audioSize := len(audioData)
		estimatedDuration := float64(audioSize) / 48000.0

		if isAudioTraceEnabled() {
			log.Printf("[AUDIO_PIPE] Playback Queue Consolidated: Preparing chunk Size=%d bytes (~%.2fs), Msg #%d", audioSize, estimatedDuration, messageIdx)
		}

		chunk := AudioChunk{
			Data:         audioData,
			Text:         text,
			StartTime:    time.Time{}, // Will be set by processor
			Duration:     estimatedDuration,
			MessageIndex: messageIdx, // Store the message index
		}

		if m.audioChannel != nil {
			select {
			case m.audioChannel <- chunk:
				if isAudioTraceEnabled() {
					log.Printf("[AUDIO_PIPE] Playback Queue Consolidated: Sent %d bytes to channel.", audioSize)
				}
				return audioQueueUpdatedMsg{} // Signal success
			default:
				log.Printf("[AUDIO_PIPE] Playback Queue Consolidated: ERROR - Channel full, dropping chunk %d bytes", audioSize)
				return audioPlaybackErrorMsg{err: errors.New("audio processing queue is full")}
			}
		} else {
			log.Printf("[AUDIO_PIPE] Playback Queue Consolidated: WARNING - Audio channel nil, cannot play.")
			return audioPlaybackErrorMsg{err: errors.New("audio channel not initialized")}
		}
	}
}
