package aistudio

import (
	"strings"
	"testing"
	// "github.com/tmc/aistudio/internal/testing/scripttest"
)

// TestPlaceholder is a placeholder test to ensure builds pass during development
func TestPlaceholder(t *testing.T) {
	// This test just passes so the package has at least one test
}

// TestModelInit tests that the model initialization calls initCmd properly
func TestModelInit(t *testing.T) {
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
	if !strings.Contains(content, "Tool Call Approval Required") {
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
