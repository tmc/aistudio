// Package scripttest provides testing helpers for aistudio tests.
package scripttest

import (
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/tmc/aistudio/internal/httprr"
	"rsc.io/script/scripttest"
)

// Variable to track if we should record HTTP interactions

// Run runs script tests for aistudio, adding HTTP recording and replay capability.
func Run(t *testing.T, record ...bool) {
	// Create a specific directory for HTTP recordings
	recordDir := filepath.Join("testdata", "recordings")
	if err := os.MkdirAll(recordDir, 0755); err != nil {
		t.Fatalf("Failed to create recording directory: %v", err)
	}

	// Initialize HTTP record/replay
	isRecord := false
	if len(record) > 0 && record[0] {
		isRecord = true
	}

	recordPath := filepath.Join(recordDir, "httprr.txt")

	// Set up recording mode if provided
	if isRecord {
		// Set the flag for httprr if we're forcing recording
		flag.Set("httprecord", ".*")
	}

	// Create a basic HTTP trace file if it doesn't exist
	if _, err := os.Stat(recordPath); os.IsNotExist(err) {
		err := os.WriteFile(recordPath, []byte("httprr trace v1\n"), 0644)
		if err != nil {
			t.Fatalf("Failed to create initial HTTP record file: %v", err)
		}
	}

	rr, err := httprr.Open(recordPath, http.DefaultTransport)
	if err != nil {
		t.Fatalf("Failed to initialize HTTP record/replay: %v", err)
	}
	defer rr.Close()

	// Set up a testing directory in testdata
	testDir := filepath.Join("testdata", "script")

	// Set an environment variable to make the HTTP transport available to tests
	if err := os.Setenv("AISTUDIO_HTTP_TRANSPORT", "configured"); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}

	// Use a simpler approach - just pass the directory to scripttest.Run
	// This relies on the standard default engine and state initialization
	scripttest.Run(t, nil, nil, testDir, nil)
}
