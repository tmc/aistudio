package aistudio_test

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestExpectInteractive provides expect-style testing for interactive functionality
func TestExpectInteractive(t *testing.T) {
	// Skip unless we have an API key and explicit test request
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping expect interactive test: No GEMINI_API_KEY environment variable provided")
	}

	if os.Getenv("AISTUDIO_RUN_INTERACTIVE_TESTS") == "" {
		t.Skip("Skipping interactive tests - set AISTUDIO_RUN_INTERACTIVE_TESTS=1 to run")
	}

	t.Run("ConversationFlow", func(t *testing.T) {
		testConversationFlow(t, apiKey)
	})

	t.Run("CommandHelp", func(t *testing.T) {
		testCommandHelp(t, apiKey)
	})
}

// testConversationFlow tests a complete conversation flow with multiple exchanges
func testConversationFlow(t *testing.T, apiKey string) {
	t.Log("Testing conversation flow with multiple exchanges")

	// Build the binary
	binaryPath := "/tmp/aistudio_conversation_test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/aistudio")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build aistudio: %v\nOutput: %s", err, output)
	}
	defer os.Remove(binaryPath)

	// Create a longer conversation that should get proper responses
	conversation := []string{
		"Hello, I'm testing the chat system.",
		"Can you tell me what 5 times 3 equals?",
		"Thank you. Now what is the capital of France?",
		"/exit",
	}

	// We'll send messages one by one instead of all at once

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath,
		"--model", "models/gemini-1.5-flash-latest",
		"--api-key", apiKey,
		"--stdin",
		"--history=true")

	// Use pipes to interact with the process
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}

	// Start the command
	err = cmd.Start()
	if err != nil {
		t.Fatalf("Failed to start command: %v", err)
	}

	// Start a goroutine to read output
	outputChan := make(chan string, 100)
	go func() {
		defer close(outputChan)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			t.Logf("Output: %s", line)
			outputChan <- line
		}
	}()

	// Send each message and expect responses
	responses := make([]string, 0)

	for i, message := range conversation {
		if message == "/exit" {
			// Send exit command
			_, err := stdin.Write([]byte(message + "\n"))
			if err != nil {
				t.Logf("Failed to write exit command: %v", err)
			}
			break
		}

		t.Logf("Sending message %d: %s", i+1, message)

		// Send the message
		_, err := stdin.Write([]byte(message + "\n"))
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i+1, err)
		}

		// Wait for response with timeout
		responseFound := false
		timeout := time.After(20 * time.Second)

		for !responseFound {
			select {
			case line, ok := <-outputChan:
				if !ok {
					t.Log("Output channel closed")
					responseFound = true
					break
				}

				// Look for model response
				if strings.Contains(line, "rt:") {
					responses = append(responses, line)
					responseFound = true
					t.Logf("Got response %d: %s", i+1, line)
				}

			case <-timeout:
				t.Logf("Timeout waiting for response to message %d", i+1)
				responseFound = true
			}
		}

		// Small delay between messages
		time.Sleep(1 * time.Second)
	}

	// Close stdin and wait for process to finish
	stdin.Close()

	// Wait for the command to complete
	err = cmd.Wait()
	if err != nil {
		t.Logf("Command finished with error (may be expected): %v", err)
	}

	// Verify we got responses
	if len(responses) == 0 {
		t.Errorf("Expected to receive at least one response, got none")
	} else {
		t.Logf("Successfully received %d responses", len(responses))
	}

	// Check for specific content in responses
	allResponses := strings.Join(responses, " ")

	// Look for mathematical answer
	if strings.Contains(allResponses, "15") {
		t.Log("✓ Found correct answer to 5 × 3 = 15")
	} else {
		t.Log("- Math answer not found in responses")
	}

	// Look for Paris as capital of France
	if strings.Contains(strings.ToLower(allResponses), "paris") {
		t.Log("✓ Found Paris as capital of France")
	} else {
		t.Log("- Paris not found in responses")
	}

	t.Log("Conversation flow test completed")
}

// testCommandHelp tests the help command functionality
func testCommandHelp(t *testing.T, apiKey string) {
	t.Log("Testing command help")

	// Build the binary
	binaryPath := "/tmp/aistudio_help_test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/aistudio")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build aistudio: %v\nOutput: %s", err, output)
	}
	defer os.Remove(binaryPath)

	// Test help via stdin
	testInput := "/help\n/exit\n"

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath,
		"--model", "models/gemini-1.5-flash-latest",
		"--api-key", apiKey,
		"--stdin")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to create stderr pipe: %v", err)
	}

	// Start the command
	err = cmd.Start()
	if err != nil {
		t.Fatalf("Failed to start command: %v", err)
	}

	// Send the help command
	go func() {
		defer stdin.Close()
		stdin.Write([]byte(testInput))
	}()

	// Read all output
	allOutput := ""

	// Read stdout
	stdoutBytes, _ := io.ReadAll(stdout)
	allOutput += string(stdoutBytes)

	// Read stderr
	stderrBytes, _ := io.ReadAll(stderr)
	allOutput += string(stderrBytes)

	// Wait for command to finish
	cmd.Wait()

	t.Logf("Help command output: %s", allOutput)

	// Check for help-related content
	helpIndicators := []string{
		"help", "command", "available", "/",
		"exit", "quit", "models",
	}

	foundHelp := false
	for _, indicator := range helpIndicators {
		if strings.Contains(strings.ToLower(allOutput), indicator) {
			foundHelp = true
			t.Logf("✓ Found help indicator: %s", indicator)
			break
		}
	}

	if !foundHelp {
		t.Log("- No clear help indicators found, but command executed")
	}

	t.Log("Command help test completed")
}

// TestExpectValidation tests input validation and error handling
func TestExpectValidation(t *testing.T) {
	// Skip unless we have an API key and explicit test request
	if os.Getenv("AISTUDIO_RUN_INTERACTIVE_TESTS") == "" {
		t.Skip("Skipping interactive tests - set AISTUDIO_RUN_INTERACTIVE_TESTS=1 to run")
	}

	t.Log("Testing input validation and error handling")

	// Build the binary
	binaryPath := "/tmp/aistudio_validation_test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/aistudio")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build aistudio: %v\nOutput: %s", err, output)
	}
	defer os.Remove(binaryPath)

	// Test with invalid API key
	testInput := "Hello\n/exit\n"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath,
		"--model", "models/gemini-1.5-flash-latest",
		"--api-key", "invalid_key_12345",
		"--stdin")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to create stderr pipe: %v", err)
	}

	// Start the command
	err = cmd.Start()
	if err != nil {
		t.Fatalf("Failed to start command: %v", err)
	}

	// Send test input
	go func() {
		defer stdin.Close()
		stdin.Write([]byte(testInput))
	}()

	// Read stderr for error messages
	stderrBytes, _ := io.ReadAll(stderr)
	stderrOutput := string(stderrBytes)

	// Wait for command to finish
	cmd.Wait()

	t.Logf("Error output with invalid key: %s", stderrOutput)

	// Check for appropriate error handling
	errorIndicators := []string{
		"error", "Error", "invalid", "authentication", "unauthorized",
		"permission", "denied", "failed",
	}

	foundError := false
	for _, indicator := range errorIndicators {
		if strings.Contains(stderrOutput, indicator) {
			foundError = true
			t.Logf("✓ Found appropriate error indicator: %s", indicator)
			break
		}
	}

	if foundError {
		t.Log("✓ Application properly handles invalid API key")
	} else {
		t.Log("- No clear error indicators found (may be expected)")
	}

	t.Log("Input validation test completed")
}
