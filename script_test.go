package aistudio_test

import (
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/tmc/aistudio/internal/testing/scripttest"
)

func TestScripts(t *testing.T) {
	scriptDir := "testdata/script" // Define the directory containing scripts

	// Find all script files in the directory
	scriptFiles, err := filepath.Glob(filepath.Join(scriptDir, "*.txt"))
	if err != nil {
		t.Fatalf("Failed to find script files: %v", err)
	}
	if len(scriptFiles) == 0 {
		t.Fatalf("No script files found in %s", scriptDir)
	}

	// Run each script file as a subtest
	for _, scriptPath := range scriptFiles {
		// Capture scriptPath in local variable for the closure
		localScriptPath := scriptPath
		// Extract a short name for the subtest
		scriptName := strings.TrimSuffix(filepath.Base(localScriptPath), filepath.Ext(localScriptPath))

		t.Run(scriptName, func(t *testing.T) {
			// Wrap the scripttest.Run call in a recovery function to prevent panics within this subtest
			defer func() {
				if r := recover(); r != nil {
					// Use t.Errorf for clearer failure reporting and include stack trace
					t.Errorf("Recovered from panic in scripttest.Run(%s): %v\nStack trace:\n%s", localScriptPath, r, debug.Stack())
					// t.Fail() is implicit with t.Errorf
				}
			}()

			t.Logf("Running script: %s", localScriptPath)
			// Call scripttest.Run for the specific script file
			// Pass the script path and potentially recording flags if needed (here defaulting to false)
			scripttest.Run(t, localScriptPath)
			t.Logf("Script completed: %s", localScriptPath)
		})
	}
}
