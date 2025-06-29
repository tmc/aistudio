package aistudio

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tmc/aistudio/api"
)

// TestHandleStreamMsg tests the stream message handling functionality
func TestHandleStreamMsg(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(WithAPIKey("test-key"))
	if model == nil {
		t.Fatal("Failed to create model")
	}

	// Test initStreamMsg
	t.Run("InitStreamMsg", func(t *testing.T) {
		msg := initStreamMsg{}
		updatedModel, cmd := model.handleStreamMsg(msg)

		if updatedModel == nil {
			t.Errorf("handleStreamMsg should not return nil model")
		}

		if cmd == nil {
			t.Errorf("handleStreamMsg should return a command for initStreamMsg")
		}

		// Check that messages were added
		model = updatedModel.(*Model)
		if len(model.messages) == 0 {
			t.Errorf("Expected messages to be added for initStreamMsg")
		}
	})

	// Test streamErrorMsg
	t.Run("StreamErrorMsg", func(t *testing.T) {
		model.streamRetryAttempt = 1
		msg := streamErrorMsg{err: fmt.Errorf("test connection error")}
		updatedModel, cmd := model.handleStreamMsg(msg)

		if updatedModel == nil {
			t.Errorf("handleStreamMsg should not return nil model")
		}

		// Should have error handling
		model = updatedModel.(*Model)
		if model.currentState != AppStateInitializing && model.currentState != AppStateError {
			t.Errorf("Expected state to be AppStateInitializing or AppStateError, got %v", model.currentState)
		}

		// Should have commands for retry or error handling
		_ = cmd // Commands may vary based on error type
	})

	// Test streamClosedMsg
	t.Run("StreamClosedMsg", func(t *testing.T) {
		model.currentState = AppStateReady
		msg := streamClosedMsg{}
		updatedModel, cmd := model.handleStreamMsg(msg)

		if updatedModel == nil {
			t.Errorf("handleStreamMsg should not return nil model")
		}

		model = updatedModel.(*Model)
		// Should handle clean closure or initiate reconnection
		_ = cmd // May or may not have commands depending on state
	})

	// Test sentMsg
	t.Run("SentMsg", func(t *testing.T) {
		msg := sentMsg{}
		updatedModel, cmd := model.handleStreamMsg(msg)

		if updatedModel == nil {
			t.Errorf("handleStreamMsg should not return nil model")
		}

		// Should continue receiving
		if cmd == nil {
			t.Errorf("Expected command to continue receiving after sent message")
		}
	})

	// Test sendErrorMsg
	t.Run("SendErrorMsg", func(t *testing.T) {
		msg := sendErrorMsg{err: fmt.Errorf("send failed")}
		updatedModel, cmd := model.handleStreamMsg(msg)

		if updatedModel == nil {
			t.Errorf("handleStreamMsg should not return nil model")
		}

		model = updatedModel.(*Model)
		if model.err == nil {
			t.Errorf("Expected error to be set after sendErrorMsg")
		}

		if model.currentState != AppStateReady {
			t.Errorf("Expected state to be AppStateReady after send error, got %v", model.currentState)
		}

		// Should have error message in messages
		found := false
		for _, msg := range model.messages {
			if msg.Sender == "System" && strings.Contains(msg.Content, "Error:") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected error message to be added to messages")
		}

		_ = cmd
	})
}

// TestPlaybackTickCmd tests the playback ticker command
func TestPlaybackTickCmd(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	cmd := playbackTickCmd()
	if cmd == nil {
		t.Errorf("playbackTickCmd should not return nil")
	}

	// Execute the command to get the message
	msg := cmd()
	if _, ok := msg.(playbackTickMsg); !ok {
		t.Errorf("playbackTickCmd should return playbackTickMsg, got %T", msg)
	}
}

// TestListenForUIUpdatesCmd tests the UI updates listener
func TestListenForUIUpdatesCmd(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(WithAPIKey("test-key"))
	if model == nil {
		t.Fatal("Failed to create model")
	}

	// Test that the command returns a function
	cmd := model.listenForUIUpdatesCmd()
	if cmd == nil {
		t.Errorf("listenForUIUpdatesCmd should not return nil")
	}

	// We can't easily test the blocking nature without complex setup
	// This test mainly ensures the function exists and returns a command
}

// TestAutoSendCmd tests the auto-send command
func TestAutoSendCmd(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(WithAPIKey("test-key"))
	if model == nil {
		t.Fatal("Failed to create model")
	}

	model.autoSendDelay = 100 * time.Millisecond
	cmd := model.autoSendCmd()
	if cmd == nil {
		t.Errorf("autoSendCmd should not return nil")
	}

	// The command should return an autoSendMsg after the delay
	// We can't easily test the timing without making the test slow
}

// TestMonitorConnection tests connection monitoring setup
func TestMonitorConnection(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(WithAPIKey("test-key"))
	if model == nil {
		t.Fatal("Failed to create model")
	}

	// Set up a context for monitoring
	ctx, cancel := context.WithCancel(context.Background())
	model.streamCtx = ctx
	defer cancel()

	// Test that monitorConnection doesn't panic
	// We'll run it in a goroutine and cancel quickly
	done := make(chan bool)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("monitorConnection panicked: %v", r)
			}
			done <- true
		}()
		model.monitorConnection()
	}()

	// Cancel context after short delay to stop monitoring
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Wait for goroutine to finish
	select {
	case <-done:
		// Good, monitoring stopped
	case <-time.After(1 * time.Second):
		t.Errorf("monitorConnection did not stop within reasonable time")
	}
}

// TestNewModel tests the NewModel constructor
func TestNewModel(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	t.Run("ValidOptions", func(t *testing.T) {
		model, err := NewModel(WithAPIKey("test-key"))
		if err != nil {
			t.Errorf("NewModel should not return error with valid options: %v", err)
		}
		if model == nil {
			t.Errorf("NewModel should not return nil model with valid options")
		}
	})

	t.Run("NoOptions", func(t *testing.T) {
		model, err := NewModel()
		if err == nil {
			t.Errorf("NewModel should return error with no options")
		}
		if model != nil {
			t.Errorf("NewModel should return nil model when error occurs")
		}
	})
}

// mockStreamResponseMsg creates a mock stream response for testing
func mockStreamResponseMsg(text string) streamResponseMsg {
	return streamResponseMsg{
		output: api.StreamOutput{
			Text:         text,
			TurnComplete: false,
		},
	}
}

// mockBidiStreamResponseMsg creates a mock bidi stream response for testing
func mockBidiStreamResponseMsg(text string, turnComplete bool) bidiStreamResponseMsg {
	return bidiStreamResponseMsg{
		output: api.StreamOutput{
			Text:         text,
			TurnComplete: turnComplete,
		},
	}
}

// TestStreamResponseMsg tests handling of stream response messages
func TestStreamResponseMsg(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(WithAPIKey("test-key"))
	if model == nil {
		t.Fatal("Failed to create model")
	}

	t.Run("TextResponse", func(t *testing.T) {
		msg := mockStreamResponseMsg("Hello, world!")
		updatedModel, cmd := model.handleStreamMsg(msg)

		if updatedModel == nil {
			t.Errorf("handleStreamMsg should not return nil model")
		}

		model = updatedModel.(*Model)

		// Should have added a message
		found := false
		for _, message := range model.messages {
			if message.Content == "Hello, world!" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected message content to be added")
		}

		// Should continue receiving unless stream is closed
		// Note: cmd might be nil if stream is not active, which is valid behavior
		_ = cmd
	})
}

// TestBidiStreamResponseMsg tests handling of bidirectional stream responses
func TestBidiStreamResponseMsg(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(WithAPIKey("test-key"))
	if model == nil {
		t.Fatal("Failed to create model")
	}

	t.Run("TurnComplete", func(t *testing.T) {
		msg := mockBidiStreamResponseMsg("Response complete", true)
		updatedModel, cmd := model.handleStreamMsg(msg)

		if updatedModel == nil {
			t.Errorf("handleStreamMsg should not return nil model")
		}

		model = updatedModel.(*Model)

		// Should transition to ready state when turn complete
		if model.currentState != AppStateReady {
			t.Errorf("Expected state to be AppStateReady after turn complete, got %v", model.currentState)
		}

		// Should have added the message
		found := false
		for _, message := range model.messages {
			if strings.Contains(message.Content, "Response complete") {
				found = true
				break
			}
		}
		if !found {
			// Debug: print all messages to see what was actually added
			t.Logf("Available messages:")
			for i, msg := range model.messages {
				t.Logf("  [%d] Sender: %s, Content: %s", i, msg.Sender, msg.Content)
			}
			t.Errorf("Expected message content to be added")
		}

		_ = cmd
	})

	t.Run("TurnIncomplete", func(t *testing.T) {
		msg := mockBidiStreamResponseMsg("Partial response", false)
		updatedModel, cmd := model.handleStreamMsg(msg)

		if updatedModel == nil {
			t.Errorf("handleStreamMsg should not return nil model")
		}

		model = updatedModel.(*Model)

		// Should stay in responding state when turn incomplete
		if model.currentState != AppStateResponding {
			t.Errorf("Expected state to be AppStateResponding for incomplete turn, got %v", model.currentState)
		}

		_ = cmd
	})
}
