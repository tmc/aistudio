package aistudio

import (
	"log"
	"strings"
	"testing"
)

// TestSetupTestLogging verifies that our test logging helper works correctly
func TestSetupTestLogging(t *testing.T) {
	// Set up test logging
	cleanup := SetupTestLogging(t)
	defer cleanup()

	// Log some test messages
	log.Println("This is a test log message")
	log.Printf("Formatted message: %d", 42)

	// The output should now appear in the test logs when run with -v
	// This test mainly verifies that the helper doesn't panic
}

// TestCaptureLogOutput verifies log capture functionality
func TestCaptureLogOutput(t *testing.T) {
	output := captureLogOutput(func() {
		log.Print("test message")
	})

	if !strings.Contains(output, "test message") {
		t.Errorf("Expected captured output to contain 'test message', got: %s", output)
	}
}

// TestMain can be used to set up logging for all tests in the package
func TestMain(m *testing.M) {
	// For demonstration, we're not setting up global logging here
	// as individual tests should call SetupTestLogging(t) themselves
	m.Run()
}
