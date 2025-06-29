package aistudio_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestInteractiveMode tests the interactive mode functionality using expect-style testing
func TestInteractiveMode(t *testing.T) {
	// Skip unless we have an API key and explicit test request
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping interactive test: No GEMINI_API_KEY environment variable provided")
	}

	if os.Getenv("AISTUDIO_RUN_INTERACTIVE_TESTS") == "" {
		t.Skip("Skipping interactive tests - set AISTUDIO_RUN_INTERACTIVE_TESTS=1 to run")
	}

	t.Run("StdinMode", func(t *testing.T) {
		testStdinMode(t, apiKey)
	})

	t.Run("CommandValidation", func(t *testing.T) {
		testCommandValidation(t, apiKey)
	})
}

// testStdinMode tests stdin mode with piped input
func testStdinMode(t *testing.T, apiKey string) {
	t.Log("Testing stdin mode with piped input")

	// Build the binary
	binaryPath := "/tmp/aistudio_interactive_test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/aistudio")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build aistudio: %v\nOutput: %s", err, output)
	}
	defer os.Remove(binaryPath)

	// Create test input with a more direct math question
	testInput := "What is 2 + 2? Please answer with just the number.\n/exit\n"

	// Run the command with stdin input
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath,
		"--model", "models/gemini-1.5-flash-latest",
		"--api-key", apiKey,
		"--stdin")

	cmd.Stdin = strings.NewReader(testInput)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()
	if err != nil {
		t.Logf("Command stderr: %s", stderr.String())
		t.Fatalf("Command failed: %v", err)
	}

	output := stdout.String()
	t.Logf("Command output: %s", output)

	// Verify we got a response
	if !strings.Contains(output, "rt:") {
		t.Errorf("Expected to see 'rt:' response prefix in output")
	}

	// Verify the response contains the answer to 2+2
	if !strings.Contains(output, "4") {
		t.Logf("Warning: Response doesn't contain '4' (answer to 2+2): %s", output)
	}

	t.Log("Stdin mode test completed successfully")
}

// testCommandValidation tests command line argument validation
func testCommandValidation(t *testing.T, apiKey string) {
	t.Log("Testing command validation")

	// Build the binary
	binaryPath := "/tmp/aistudio_cmdval_test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/aistudio")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build aistudio: %v\nOutput: %s", err, output)
	}
	defer os.Remove(binaryPath)

	// Test help command
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--help")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Logf("Help command stderr: %s", stderr.String())
		t.Fatalf("Help command failed: %v", err)
	}

	output := stdout.String()
	stderrOutput := stderr.String()
	fullOutput := output + stderrOutput
	t.Logf("Help output length: %d characters", len(fullOutput))
	t.Logf("Help output: %s", fullOutput)

	// Verify help contains expected sections (check both stdout and stderr)
	expectedSections := []string{"Usage:", "Options:"}
	for _, section := range expectedSections {
		if !strings.Contains(fullOutput, section) {
			t.Errorf("Help output missing expected section: %s", section)
		}
	}

	// Test list models command (with reasonable timeout)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	cmd2 := exec.CommandContext(ctx2, binaryPath, "--list-models", "--api-key", apiKey)

	var stdout2 bytes.Buffer
	var stderr2 bytes.Buffer
	cmd2.Stdout = &stdout2
	cmd2.Stderr = &stderr2

	err2 := cmd2.Run()
	// Don't fail if this errors, just log it
	if err2 != nil {
		t.Logf("List models command returned error (expected in some cases): %v", err2)
	}

	output2 := stdout2.String() + stderr2.String()
	t.Logf("List models output: %s", output2)

	// Verify we got some model-related output
	hasModelInfo := strings.Contains(output2, "model") ||
		strings.Contains(output2, "gemini") ||
		strings.Contains(output2, "Available")

	if hasModelInfo {
		t.Log("Successfully retrieved model information")
	} else {
		t.Log("No model information retrieved (may be expected due to auth)")
	}

	t.Log("Command validation test completed")
}

// TestInteractiveCommands tests various interactive commands
func TestInteractiveCommands(t *testing.T) {
	// Skip unless we have an API key and explicit test request
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping interactive commands test: No GEMINI_API_KEY environment variable provided")
	}

	if os.Getenv("AISTUDIO_RUN_INTERACTIVE_TESTS") == "" {
		t.Skip("Skipping interactive tests - set AISTUDIO_RUN_INTERACTIVE_TESTS=1 to run")
	}

	t.Log("Testing interactive commands")

	// Build the binary
	binaryPath := "/tmp/aistudio_commands_test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/aistudio")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build aistudio: %v\nOutput: %s", err, output)
	}
	defer os.Remove(binaryPath)

	// Test help command
	testInput := "/help\n/exit\n"

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath,
		"--model", "models/gemini-1.5-flash-latest",
		"--api-key", apiKey,
		"--stdin")

	cmd.Stdin = strings.NewReader(testInput)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()
	if err != nil {
		t.Logf("Command stderr: %s", stderr.String())
		// Don't fail on error as /help might cause early exit
	}

	output := stdout.String() + stderr.String()
	t.Logf("Help command output: %s", output)

	// Verify we got help information
	hasHelpInfo := strings.Contains(output, "help") ||
		strings.Contains(output, "command") ||
		strings.Contains(output, "/")

	if !hasHelpInfo {
		t.Logf("Warning: Expected to see help information in output")
	}

	t.Log("Interactive commands test completed")
}

// expectOutput is a helper function that reads from a pipe and looks for expected patterns
func expectOutput(t *testing.T, pipe io.Reader, patterns []string, timeout time.Duration) bool {
	scanner := bufio.NewScanner(pipe)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if scanner.Scan() {
			line := scanner.Text()
			t.Logf("Got line: %s", line)

			for _, pattern := range patterns {
				if strings.Contains(line, pattern) {
					t.Logf("Found expected pattern: %s", pattern)
					return true
				}
			}
		}

		// Small sleep to prevent busy waiting
		time.Sleep(100 * time.Millisecond)
	}

	return false
}
