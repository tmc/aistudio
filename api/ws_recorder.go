package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WSRecorder records WebSocket traffic for testing
type WSRecorder struct {
	FilePath    string
	Recording   []WSMessage
	RecordMode  bool
	ReplayIndex int
	mu          sync.Mutex
}

// WSMessage represents a recorded WebSocket message
type WSMessage struct {
	Direction   string          `json:"direction"`    // "send" or "receive"
	Payload     json.RawMessage `json:"payload"`      // Message content
	Timestamp   int64           `json:"timestamp"`    // Unix timestamp
	ElapsedMs   int64           `json:"elapsed_ms"`   // Milliseconds since first message
	MessageType int             `json:"message_type"` // WebSocket message type
}

// NewWSRecorder creates a new WebSocket recorder
func NewWSRecorder(path string, record bool) (*WSRecorder, error) {
	recorder := &WSRecorder{
		FilePath:   path,
		RecordMode: record,
		Recording:  make([]WSMessage, 0),
	}

	// Create directory if it doesn't exist and we're in record mode
	if record {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}

	if !record {
		// Load existing recording in replay mode
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read recording file %s: %v", path, err)
		}
		if err := json.Unmarshal(data, &recorder.Recording); err != nil {
			return nil, fmt.Errorf("failed to parse recording file %s: %v", path, err)
		}

		fmt.Printf("Loaded WebSocket recording with %d messages from %s\n",
			len(recorder.Recording), path)
	} else {
		fmt.Printf("Creating new WebSocket recording at %s\n", path)
	}

	return recorder, nil
}

// RecordSend records a sent message
func (r *WSRecorder) RecordSend(msg []byte, messageType int) error {
	if !r.RecordMode {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UnixNano() / int64(time.Millisecond)
	var elapsedMs int64 = 0

	if len(r.Recording) > 0 {
		firstMsg := r.Recording[0]
		elapsedMs = now - (firstMsg.Timestamp * 1000)
	}

	r.Recording = append(r.Recording, WSMessage{
		Direction:   "send",
		Payload:     json.RawMessage(msg),
		Timestamp:   now / 1000, // Convert to seconds
		ElapsedMs:   elapsedMs,
		MessageType: messageType,
	})

	return r.saveRecording()
}

// RecordReceive records a received message
func (r *WSRecorder) RecordReceive(msg []byte, messageType int) error {
	if !r.RecordMode {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UnixNano() / int64(time.Millisecond)
	var elapsedMs int64 = 0

	if len(r.Recording) > 0 {
		firstMsg := r.Recording[0]
		elapsedMs = now - (firstMsg.Timestamp * 1000)
	}

	r.Recording = append(r.Recording, WSMessage{
		Direction:   "receive",
		Payload:     json.RawMessage(msg),
		Timestamp:   now / 1000, // Convert to seconds
		ElapsedMs:   elapsedMs,
		MessageType: messageType,
	})

	return r.saveRecording()
}

// GetNextMessage gets the next message for replay
func (r *WSRecorder) GetNextMessage() (string, []byte, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ReplayIndex >= len(r.Recording) {
		return "", nil, 0, fmt.Errorf("end of recording reached")
	}

	msg := r.Recording[r.ReplayIndex]
	r.ReplayIndex++

	// Simulate delay if not the first message
	if r.ReplayIndex > 1 && len(r.Recording) > 1 {
		// Sleep based on time between this message and previous one
		prevMsg := r.Recording[r.ReplayIndex-2]
		delay := msg.ElapsedMs - prevMsg.ElapsedMs

		// Cap at 1000ms to avoid excessive delays in tests
		if delay > 1000 {
			delay = 1000
		}

		if delay > 0 {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
	}

	return msg.Direction, []byte(msg.Payload), msg.MessageType, nil
}

// saveRecording saves the recording to the file
func (r *WSRecorder) saveRecording() error {
	// Save recording to file
	data, err := json.MarshalIndent(r.Recording, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal recording: %v", err)
	}

	if err := os.WriteFile(r.FilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write recording file: %v", err)
	}

	return nil
}

// Close finalizes the recording
func (r *WSRecorder) Close() error {
	if r.RecordMode {
		fmt.Printf("Saved WebSocket recording with %d messages to %s\n",
			len(r.Recording), r.FilePath)
	}
	return nil
}

// GetReceivedMessages returns all received messages
func (r *WSRecorder) GetReceivedMessages() []WSMessage {
	r.mu.Lock()
	defer r.mu.Unlock()

	msgs := make([]WSMessage, 0)
	for _, msg := range r.Recording {
		if msg.Direction == "receive" {
			msgs = append(msgs, msg)
		}
	}

	return msgs
}
