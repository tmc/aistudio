package aistudio_test

import (
	"testing"

	"github.com/tmc/aistudio/internal/testing/scripttest"
)

func TestScripts(t *testing.T) {
	// Skip until script test infrastructure is stable
	t.Skip("Skipping script tests until script test infrastructure is stable")
	
	// Wrap the scripttest.Run call in a recovery function to prevent panics
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Recovered from panic in TestScripts: %v", r)
			t.Fail()
		}
	}()
	
	// Only run if not skipped
	if !t.Skipped() {
		scripttest.Run(t, false) // Pass false to avoid recording mode
	}
}
