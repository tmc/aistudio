package aistudio

import (
	"testing"
	// "github.com/tmc/aistudio/internal/testing/scripttest"
)

// TestPlaceholder is a placeholder test to ensure builds pass during development
func TestPlaceholder(t *testing.T) {
	// This test just passes so the package has at least one test
}

// TestScripts is a test for running the script tests
func TestScripts(t *testing.T) {
	// Skip for now until script test infrastructure is stable
	t.Skip("Skipping script tests until script test infrastructure is stable")
	// scripttest.Run(t)
}

// TestModelInit tests that the model initialization calls initCmd properly
func TestModelInit(t *testing.T) {
	model := New()

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
