package aistudio_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tmc/aistudio/api"
)

// TestE2EConnection runs end-to-end connectivity tests using the actual binary
// This test requires a real API key and will be skipped if none is provided
//
// The test suite verifies critical aspects of the aistudio client:
// 1. Binary initialization and connection setup
// 2. Message sending and receiving
// 3. Connection stability and keepalive behavior
// 4. Library-level API interaction
//
// Each test is designed to validate a specific component of the client's functionality
// while maintaining reasonable runtime performance.
func TestE2EConnection(t *testing.T) {
	// Skip these tests unless the AISTUDIO_RUN_E2E_TESTS environment variable is set
	// They require a valid API key with exact model access and may fail in CI environments
	if os.Getenv("AISTUDIO_RUN_E2E_TESTS") == "" {
		t.Skip("Skipping E2E tests - set AISTUDIO_RUN_E2E_TESTS=1 to run")
	}
	// Skip if no API key available
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping E2E connection test: No GEMINI_API_KEY environment variable provided")
	}

	// Run subtests for different aspects of connectivity
	t.Run("BinaryInitialization", func(t *testing.T) {
		testBinaryInitialization(t, apiKey)
	})

	t.Run("MessageSending", func(t *testing.T) {
		testMessageSending(t, apiKey)
	})

	t.Run("KeepAliveStability", func(t *testing.T) {
		testKeepAliveStability(t, apiKey)
	})

	t.Run("LibraryConnection", func(t *testing.T) {
		testLibraryConnection(t, apiKey)
	})
}

// Test that binary initializes without connection errors
func testBinaryInitialization(t *testing.T, apiKey string) {
	t.Log("Building aistudio binary")
	binaryPath := "/Volumes/tmc/go/src/github.com/tmc/aistudio/aistudio_test_binary"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/aistudio")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build aistudio: %v\nOutput: %s", err, output)
	}
	defer os.Remove(binaryPath) // Clean up after test

	t.Log("Testing binary initialization with API key")
	helpCmd := exec.Command(binaryPath, "--api-key", apiKey, "--help")
	output, err := helpCmd.CombinedOutput()

	if err != nil {
		t.Fatalf("Failed to run help command: %v\nOutput: %s", err, output)
	}

	// Verify that help output contains expected text
	if !strings.Contains(string(output), "Interactive chat with Gemini") {
		t.Errorf("Help output missing expected content")
	}

	t.Log("Binary initializes successfully with real API key")
}

// Test sending a message to the model and receiving a response
func testMessageSending(t *testing.T, apiKey string) {
	t.Log("Testing message sending")

	// Get absolute path to the binary
	binaryPath := "/Volumes/tmc/go/src/github.com/tmc/aistudio/aistudio_test_binary"
	// Create a command that will send a simple message via stdin
	cmd := exec.Command(binaryPath, "--api-key", apiKey, "--stdin")

	// Prepare stdin pipe to send messages
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to get stdin pipe: %v", err)
	}

	// Capture stdout to get response
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	// Start the command
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start command: %v", err)
	}

	// Write a test message to stdin
	testMessage := "Say hello in a short sentence.\n"
	_, err = io.WriteString(stdin, testMessage)
	if err != nil {
		t.Fatalf("Failed to write to stdin: %v", err)
	}
	stdin.Close()

	// Wait for the command to complete with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// Set a timeout
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Command failed: %v", err)
		}
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		t.Fatalf("Command timed out")
	}

	// Check stdout for model response
	response := stdout.String()
	t.Logf("Got response (truncated): %s", truncateString(response, 100))

	// Look for the rt: prefix that indicates a model response
	if !strings.Contains(response, "rt:") {
		t.Errorf("No model response found in output")
	}

	t.Log("Successfully received response from model")
}

// Test that keepalive maintains connection stability
func testKeepAliveStability(t *testing.T, apiKey string) {
	t.Log("Testing connection stability with keepalive")

	// Get absolute path to the binary
	binaryPath := "/Volumes/tmc/go/src/github.com/tmc/aistudio/aistudio_test_binary"
	// We'll use the binary with a long timeout to test stability
	cmd := exec.Command(binaryPath, "--api-key", apiKey, "--stdin")

	// Prepare stdin and stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to get stdin pipe: %v", err)
	}

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	// Start the command
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start command: %v", err)
	}

	// Send three messages with 10-second pauses in between
	// This will test if the connection stays alive during idle periods
	for i := 0; i < 3; i++ {
		time.Sleep(10 * time.Second)
		t.Logf("Sending message %d after 10s pause", i+1)

		testMessage := fmt.Sprintf("Ping %d: What time is it?\n", i+1)
		_, err = io.WriteString(stdin, testMessage)
		if err != nil {
			t.Fatalf("Failed to write to stdin: %v", err)
		}
	}
	stdin.Close()

	// Wait for the command to complete with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// Allow a longer timeout since we're testing multiple messages
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Command failed: %v", err)
		}
	case <-time.After(60 * time.Second):
		cmd.Process.Kill()
		t.Fatalf("Command timed out")
	}

	// Check stdout for all the responses
	response := stdout.String()
	t.Logf("Final response (truncated): %s", truncateString(response, 100))

	// Verify we got responses to all pings
	rtCount := strings.Count(response, "rt:")
	if rtCount < 3 {
		t.Errorf("Expected 3 responses, got %d", rtCount)
	}

	t.Log("Connection remained stable for multiple messages with pauses")
}

// Test direct library connection
func testLibraryConnection(t *testing.T, apiKey string) {
	t.Log("Testing direct library connection")

	// Create a client with the API key
	client := &api.Client{
		APIKey: apiKey,
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initialize the client
	if err := client.InitClient(ctx); err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}
	defer client.Close()

	// Set up config
	config := &api.StreamClientConfig{
		ModelName: "gemini-1.0-pro", // Use a more broadly available model for testing
	}

	// Open a stream
	bidiStream, err := client.InitBidiStream(ctx, config)
	if err != nil {
		t.Fatalf("Failed to initialize stream: %v", err)
	}

	// Send a test message
	testMessage := "Hello, this is a connection test."
	if err := client.SendMessageToBidiStream(bidiStream, testMessage); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Receive response
	gotResponse := false
	timeout := time.After(20 * time.Second)
	for !gotResponse {
		select {
		case <-timeout:
			t.Fatalf("Timed out waiting for response")
		default:
			resp, err := bidiStream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				t.Fatalf("Error receiving response: %v", err)
			}

			// Extract output
			output := api.ExtractBidiOutput(resp)
			if output.Text != "" {
				t.Logf("Received response: %s", truncateString(output.Text, 100))
				gotResponse = true
			}

			// Break if turn is complete
			if output.TurnComplete {
				break
			}
		}
	}

	if !gotResponse {
		t.Errorf("Did not receive any text response from the model")
	}

	t.Log("Library connection working correctly")
}

// Helper function to truncate strings for logging
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// TestE2EResponseContent tests the quality and properties of model responses
// This is a more detailed test of response content and formatting
func TestE2EResponseContent(t *testing.T) {
	// Skip these tests unless the AISTUDIO_RUN_E2E_TESTS environment variable is set
	// They require a valid API key with exact model access and may fail in CI environments
	if os.Getenv("AISTUDIO_RUN_E2E_TESTS") == "" {
		t.Skip("Skipping E2E response tests - set AISTUDIO_RUN_E2E_TESTS=1 to run")
	}
	// Skip if no API key available
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping E2E response test: No GEMINI_API_KEY environment variable provided")
	}

	// Create a client with the API key
	client := &api.Client{
		APIKey: apiKey,
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Initialize the client
	if err := client.InitClient(ctx); err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}
	defer client.Close()

	// Set up config with default model
	config := &api.StreamClientConfig{
		ModelName: "gemini-1.0-pro", // Use a more broadly available model for testing
	}

	// Open a stream
	t.Log("Testing model response content properties...")
	stream, err := client.InitBidiStream(ctx, config)
	if err != nil {
		t.Fatalf("Failed to initialize stream: %v", err)
	}

	// Test multi-paragraph formatting with a prompt designed to elicit structured response
	formatTestQuery := "Please explain cloud computing in 3 short paragraphs with a numbered list of 3 benefits at the end."
	if err := client.SendMessageToBidiStream(stream, formatTestQuery); err != nil {
		t.Fatalf("Failed to send format test query: %v", err)
	}

	// Collect the complete response
	var fullText string
	var paragraphCount int
	var listItemCount int

	timeout := time.After(30 * time.Second)
	done := false

	for !done {
		select {
		case <-timeout:
			t.Fatalf("Timed out waiting for response")
		default:
			resp, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					done = true
					break
				}
				t.Fatalf("Error receiving response: %v", err)
			}

			// Extract output
			output := api.ExtractBidiOutput(resp)
			fullText += output.Text

			// Count paragraphs (text separated by blank lines)
			if strings.Contains(output.Text, "\n\n") {
				paragraphCount++
			}

			// Count list items (lines starting with numbers)
			for _, line := range strings.Split(output.Text, "\n") {
				if strings.TrimSpace(line) != "" && strings.Contains(line, "1.") ||
					strings.Contains(line, "2.") || strings.Contains(line, "3.") {
					listItemCount++
				}
			}

			// Break if turn is complete
			if output.TurnComplete {
				done = true
				break
			}
		}
	}

	// Log the response for debugging
	t.Logf("Response length: %d characters", len(fullText))
	t.Logf("Response preview: %s", truncateString(fullText, 200))

	// Check response formatting
	if len(fullText) < 200 {
		t.Errorf("Response too short: %d chars", len(fullText))
	}

	// Verify content mentions cloud computing
	if !strings.Contains(strings.ToLower(fullText), "cloud computing") {
		t.Errorf("Response doesn't mention the requested topic")
	}

	// Check for paragraphs
	if strings.Count(fullText, "\n\n") < 2 {
		t.Logf("Warning: Expected at least 3 paragraphs, formatting may not be ideal")
	}

	// Check for numbered list items
	listItems := 0
	for _, line := range strings.Split(fullText, "\n") {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "1.") || strings.HasPrefix(trimmedLine, "2.") ||
			strings.HasPrefix(trimmedLine, "3.") {
			listItems++
		}
	}

	if listItems < 3 {
		t.Logf("Warning: Expected 3 list items, found approximately %d", listItems)
	}

	t.Log("Response content test completed")
}

// TestE2EConnectionSetup is a test that verifies the test itself works
// This is useful to ensure the test machinery itself functions
func TestE2EConnectionSetup(t *testing.T) {
	buildCmd := exec.Command("go", "build", "-o", "aistudio_setup_test", "./cmd/aistudio")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build binary for setup test: %v\nOutput: %s", err, output)
	}
	defer os.Remove("aistudio_setup_test")

	helpCmd := exec.Command("./aistudio_setup_test", "--help")
	if output, err := helpCmd.CombinedOutput(); err != nil {
		t.Fatalf("Test binary cannot run --help: %v\nOutput: %s", err, output)
	}

	t.Log("E2E connection test infrastructure is working correctly")
}

// If main function is called directly, run the tests with API key validation
func init() {
	// If running this file directly (go run e2e_connection_test.go)
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Println("Warning: No GEMINI_API_KEY environment variable provided")
		log.Println("E2E connection tests will be skipped when run with 'go test'")
		log.Println("To run with 'go test', use: GEMINI_API_KEY=your_key go test -v .")
	} else {
		log.Printf("API key found (length: %d). E2E tests will run when executed.", len(apiKey))
	}
}
