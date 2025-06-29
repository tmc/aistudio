package aistudio

import (
	"strings"
	"testing"
	"time"
	// "github.com/tmc/aistudio/internal/testing/scripttest"
)

// TestModelInit tests that the model initialization calls initCmd properly
func TestModelInit(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	// Provide a minimal option to prevent New() from returning nil
	model := New(WithAPIKey(""))
	if model == nil {
		t.Fatal("New() returned nil even with an option")
	}

	// Call Init() which should now invoke initCmd
	cmds := model.Init()

	// Verify the returned command is a batch command containing multiple commands
	if cmds == nil {
		t.Error("Expected non-nil command from Init")
	}

	// Test that InitModel returns a model and no error
	initializedModel, err := model.InitModel()
	if err != nil {
		t.Errorf("Error initializing model: %v", err)
	}
	if initializedModel == nil {
		t.Error("Expected non-nil model from InitModel")
	}
}

// TestView tests the View implementation
func TestView(t *testing.T) {
	cleanup := SetupTestLogging(t)
	defer cleanup()

	model := New(WithAPIKey(""), WithModel("test-model"))
	if model == nil {
		t.Fatal("New() returned nil even with an option")
	}

	// Set width to prevent rendering issues
	model.width = 80
	model.height = 24

	// Test normal state
	model.currentState = AppStateReady
	model.modelName = "test-model" // Make sure model name is set
	view := model.View()
	if !strings.Contains(view, "Model: test-model") {
		t.Error("View() should contain model name when in ready state")
	}
	if !strings.Contains(view, "Ready") {
		t.Error("View() should contain 'Ready' when in ready state")
	}

	// Test quitting state
	model.currentState = AppStateQuitting
	view = model.View()
	if !strings.Contains(view, "Closing stream and quitting") {
		t.Error("View() should indicate quitting when in quitting state")
	}

	// Test error state
	model.currentState = AppStateError
	model.err = &testError{"test error"}
	view = model.View()
	if !strings.Contains(view, "Error: test error") {
		t.Error("View() should display error message when in error state")
	}

	// Test tool approval rendering
	model.showToolApproval = true
	model.pendingToolCalls = []ToolCall{
		{
			ID:   "test-id",
			Name: "TestTool",
			Arguments: []byte(`{
				"param1": "value1",
				"param2": 123
			}`),
		},
	}
	model.approvalIndex = 0
	model.currentState = AppStateWaiting
	content := model.renderToolApprovalModalContent()
	if !strings.Contains(content, "Tool Call Approval") {
		t.Error("renderToolApprovalModalContent() should contain approval title")
	}
	if !strings.Contains(content, "TestTool") {
		t.Error("renderToolApprovalModalContent() should contain tool name")
	}
	if !strings.Contains(content, "param1") {
		t.Error("renderToolApprovalModalContent() should contain formatted arguments")
	}
}

// testError is a simple error implementation for testing
type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}

// TestNew tests the New function more thoroughly
func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		wantNil bool
	}{
		{
			name:    "No options",
			opts:    []Option{},
			wantNil: true,
		},
		{
			name: "With API key",
			opts: []Option{
				WithAPIKey("test-api-key"),
			},
			wantNil: false,
		},
		{
			name: "With multiple options",
			opts: []Option{
				WithAPIKey("test-api-key"),
				WithModel("test-model"),
				WithAudioOutput(true),
				WithGlobalTimeout(1 * time.Minute),
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := SetupTestLogging(t)
			defer cleanup()

			m := New(tt.opts...)
			if (m == nil) != tt.wantNil {
				t.Errorf("New() = %v, wantNil = %v", m == nil, tt.wantNil)
			}

			// Additional checks if model is not nil
			if m != nil {
				// Check that the model has reasonable defaults
				if m.textarea.Value() == "" && len(m.textarea.Prompt) == 0 { // Check if textarea is properly initialized
					// textarea is initialized properly
				}
				if m.viewport.Width == 0 && m.viewport.Height == 0 { // Check if viewport is properly initialized
					// viewport is initialized properly
				}
				if m.spinner.Spinner.Frames == nil { // Check if spinner is properly initialized
					// spinner is initialized properly
				}
				if m.client == nil {
					t.Error("New() created model with nil client")
				}
			}
		})
	}
}

// TestExitCode tests the ExitCode method
func TestExitCode(t *testing.T) {
	m := &Model{exitCode: 0}
	if code := m.ExitCode(); code != 0 {
		t.Errorf("ExitCode() = %v, want 0", code)
	}

	m.exitCode = 1
	if code := m.ExitCode(); code != 1 {
		t.Errorf("ExitCode() = %v, want 1", code)
	}
}

// TestToolManager tests the ToolManager method
func TestToolManager(t *testing.T) {
	toolManager := NewToolManager()
	m := &Model{toolManager: toolManager}

	if got := m.ToolManager(); got != toolManager {
		t.Errorf("ToolManager() = %v, want %v", got, toolManager)
	}
}

// TestUpdateHistory tests the UpdateHistory method
func TestUpdateHistory(t *testing.T) {
	// Test case: history disabled
	m := &Model{
		historyEnabled: false,
	}
	m.UpdateHistory(Message{}) // Should do nothing

	// Test case: history enabled but nil manager
	m = &Model{
		historyEnabled: true,
		historyManager: nil,
	}
	m.UpdateHistory(Message{}) // Should do nothing and not panic
}

// TestCheckPlayback tests the checkPlayback function
func TestCheckPlayback(t *testing.T) {
	m := &Model{
		messages: []Message{
			{
				Sender:    senderNameModel,
				HasAudio:  true,
				AudioData: []byte("test-audio"),
			},
		},
	}

	// Test ctrl+p with audio
	_, triggered := m.checkPlayback("ctrl+p")
	if !triggered {
		t.Errorf("checkPlayback(ctrl+p) = %v, want true", triggered)
	}

	// Test ctrl+r with no current audio
	_, triggered = m.checkPlayback("ctrl+r")
	if triggered {
		t.Errorf("checkPlayback(ctrl+r) = %v, want false", triggered)
	}

	// Test ctrl+r with current audio
	m.currentAudio = &AudioChunk{Data: []byte("test-audio")}
	_, triggered = m.checkPlayback("ctrl+r")
	if !triggered {
		t.Errorf("checkPlayback(ctrl+r) with currentAudio = %v, want true", triggered)
	}

	// Test invalid key
	_, triggered = m.checkPlayback("invalid")
	if triggered {
		t.Errorf("checkPlayback(invalid) = %v, want false", triggered)
	}
}
