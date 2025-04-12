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
