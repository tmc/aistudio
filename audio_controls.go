package aistudio

import (
	"bytes"
	"log"

	tea "github.com/charmbracelet/bubbletea"
)

// PlayLastAudio attempts to play the most recent available audio that hasn't been played yet.
// Returns a tea.Cmd and a boolean indicating whether a playback was triggered.
func (m *Model) PlayLastAudio() (tea.Cmd, bool) {
	var cmds []tea.Cmd
	var playbackTriggered bool

	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].HasAudio && !m.messages[i].IsPlayed && !m.messages[i].IsPlaying {
			if m.enableAudio && len(m.messages[i].AudioData) > 0 {
				log.Printf("[UI] Triggering playback for message #%d", i)
				cmds = append(cmds, m.playAudioCmd(m.messages[i].AudioData, m.messages[i].Content))
				m.messages[i].IsPlaying = true // Optimistic UI update
				m.viewport.SetContent(m.formatAllMessages())
				playbackTriggered = true
				break
			}
		}
	}

	return tea.Batch(cmds...), playbackTriggered
}

// ReplayLastAudio attempts to replay the most recent audio that has already been played.
// Returns a tea.Cmd and a boolean indicating whether a replay was triggered.
func (m *Model) ReplayLastAudio() (tea.Cmd, bool) {
	var cmds []tea.Cmd
	var replayTriggered bool

	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].HasAudio && m.messages[i].IsPlayed && !m.messages[i].IsPlaying {
			if m.enableAudio && len(m.messages[i].AudioData) > 0 {
				log.Printf("[UI] Triggering replay for message #%d", i)
				cmds = append(cmds, m.playAudioCmd(m.messages[i].AudioData, m.messages[i].Content))
				m.messages[i].IsPlaying = true // Optimistic UI update
				m.messages[i].IsPlayed = false
				m.viewport.SetContent(m.formatAllMessages())
				replayTriggered = true
				break
			}
		}
	}

	return tea.Batch(cmds...), replayTriggered
}

// FindMessageWithAudioData finds a message with exactly matching audio data.
// Returns the message index and whether a matching message was found.
func (m *Model) FindMessageWithAudioData(audioData []byte) (int, bool) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].HasAudio && len(m.messages[i].AudioData) > 0 && bytes.Equal(m.messages[i].AudioData, audioData) {
			return i, true
		}
	}
	return -1, false
}

// StopCurrentAudio stops any currently playing audio and clears buffers.
// Returns true if audio was playing and was stopped.
func (m *Model) StopCurrentAudio() bool {
	if !m.isAudioProcessing || !m.enableAudio {
		return false
	}

	log.Println("Explicitly stopping current audio playback")

	// Clear audio buffer to stop ongoing accumulation
	m.consolidatedAudioData = nil

	// Stop any timers
	if m.bufferTimer != nil {
		m.bufferTimer.Stop()
		m.bufferTimer = nil
	}

	// Mark the current message's audio as complete
	if m.currentAudio != nil {
		messageIdx := m.currentAudio.MessageIndex

		// Update the message state if applicable
		if messageIdx >= 0 && messageIdx < len(m.messages) {
			m.messages[messageIdx].IsPlaying = false
			m.messages[messageIdx].IsPlayed = true
		}

		// Clear the current audio
		m.currentAudio.IsComplete = true
		m.currentAudio = nil
	}

	// Mark that we're no longer processing audio
	m.isAudioProcessing = false

	return true
}
