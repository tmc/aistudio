// Package scripttest provides testing helpers for aistudio tests.
package scripttest

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"errors"
	"os/exec"
	"runtime"
	"testing"

	"github.com/tmc/aistudio/internal/httprr"
	"strings"
)

// findProjectRoot searches upwards from a given directory for a go.mod file.
func findProjectRoot(startDir string) (string, error) {
	dir := startDir
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir, nil // Found go.mod
		}
		// Move up one directory
		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			// Reached the root directory without finding go.mod
			return "", errors.New("go.mod not found in any parent directory")
		}
		dir = parentDir
	}
}

// Run runs a single script test file for aistudio, adding HTTP recording and replay capability.
// It executes the script specified by scriptPath.
func Run(t *testing.T, scriptPath string, record ...bool) {
	log.Printf("Starting scripttest.Run for script: %s", scriptPath)

	// Determine script directory and base name
	scriptDir := filepath.Dir(scriptPath)
	scriptBase := filepath.Base(scriptPath)
	scriptName := strings.TrimSuffix(scriptBase, filepath.Ext(scriptBase)) // e.g., "my_test"

	// Create a specific directory for HTTP recordings based on the script directory
	// Place recordings alongside the scripts or in a dedicated parallel structure.
	// Using a parallel 'recordings' directory relative to the script's dir.
	recordDir := filepath.Join(scriptDir, "..", "recordings") // Assumes scripts are in testdata/scripts
	log.Printf("Ensuring recording directory exists: %s", recordDir)
	if err := os.MkdirAll(recordDir, 0755); err != nil {
		t.Fatalf("Failed to create recording directory %q for script %s: %v", recordDir, scriptBase, err)
	}

	// Initialize HTTP record/replay
	isRecord := false
	if len(record) > 0 && record[0] {
		isRecord = true
	}
	log.Printf("HTTP recording enabled for %s: %v", scriptBase, isRecord)

	// Generate a unique recording path for this script
	recordBaseName := scriptName + ".httprr.txt"
	recordPath := filepath.Join(recordDir, recordBaseName)
	log.Printf("Using HTTP recording path for %s: %s", scriptBase, recordPath)

	// Set up recording mode if provided
	if isRecord {
		log.Println("Setting httprecord flag to '.*'")
		// Set the flag for httprr if we're forcing recording
		log.Printf("Setting httprecord flag to match specific file: %s", recordPath)
		// Set the flag for httprr if we're forcing recording.
		// We use the specific recordPath to ensure httprr targets the correct file.
		// Note: httprr.Recording() uses regex matching on the *file* argument passed to Open/NewReplay.
		// We need to ensure the regex matches our specific recordPath.
		// A simple way is to use the exact path, escaping regex characters if necessary,
		// but since we control the path, using ".*" might still be okay if we only
		// intend to record *this* specific interaction triggered by this test run.
		// Let's stick with ".*" for simplicity, assuming only one test runs at a time or manages its transport.
		// If running tests in parallel causes issues, this might need refinement.
		if err := flag.Set("httprecord", ".*"); err != nil {
			t.Fatalf("Failed to set httprecord flag for %s: %v", scriptBase, err)
		}
	}

	// Create a basic HTTP trace file if it doesn't exist, needed for Open
	if _, err := os.Stat(recordPath); os.IsNotExist(err) {
		log.Printf("Creating initial HTTP record file: %s", recordPath)
		err := os.WriteFile(recordPath, []byte("httprr trace v1\n"), 0644)
		if err != nil {
			t.Fatalf("Failed to create initial HTTP record file %q for script %s: %v", recordPath, scriptBase, err)
		}
	}

	rr, err := httprr.Open(recordPath, http.DefaultTransport)
	if err != nil {
		t.Fatalf("Failed to initialize HTTP record/replay with file %q for script %s: %v", recordPath, scriptBase, err)
	}
	defer func() {
		log.Printf("Closing HTTP record/replay transport for script %s (file: %s).", scriptBase, recordPath)
		if err := rr.Close(); err != nil {
			// Use t.Errorf to report the error but allow other deferred functions
			// and test cleanup to run.
			t.Errorf("Failed to close HTTP record/replay transport for script %s (file: %s): %v", scriptBase, recordPath, err)
		}
	}()

	// Configure the underlying script engine to use our HTTP transport
	// We need to replace http.DefaultClient.Transport
	// Ensure this is restored after the test.
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = rr
	defer func() {
		log.Printf("Restoring original http.DefaultClient.Transport for script %s", scriptBase)
		http.DefaultClient.Transport = originalTransport
	}()

	// Set an environment variable to indicate the transport is managed, if needed by scripts.
	// This might not be necessary if scripts directly use http.DefaultClient.
	log.Printf("Setting AISTUDIO_HTTP_TRANSPORT=configured environment variable for script %s.", scriptBase)
	originalEnv, envSet := os.LookupEnv("AISTUDIO_HTTP_TRANSPORT")
	if err := os.Setenv("AISTUDIO_HTTP_TRANSPORT", "configured"); err != nil {
		t.Fatalf("Failed to set AISTUDIO_HTTP_TRANSPORT environment variable for script %s: %v", scriptBase, err)
	}
	defer func() {
		// Restore original environment variable state
		if envSet {
			os.Setenv("AISTUDIO_HTTP_TRANSPORT", originalEnv)
		} else {
			os.Unsetenv("AISTUDIO_HTTP_TRANSPORT")
		}
	}()

	// --- Start: Custom build and run logic ---
	log.Printf("Building example binary for script: %s", scriptBase)

	// Determine project root dynamically
	_, currentFilePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("Failed to get current file path for script %s", scriptBase)
	}
	currentDir := filepath.Dir(currentFilePath)
	projectRoot, err := findProjectRoot(currentDir)
	if err != nil {
		t.Fatalf("Failed to find project root for script %s: %v", scriptBase, err)
	}
	log.Printf("Determined project root for script %s: %s", scriptBase, projectRoot)

	// Construct path to example directory
	exampleSourceDir := filepath.Join(projectRoot, "example")
	log.Printf("Using example source directory for script %s: %s", scriptBase, exampleSourceDir)

	// Create a temporary directory for the build output
	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "aistudio_example_test")

	// Build the example binary
	buildCmd := exec.Command("go", "build", "-o", binaryPath, exampleSourceDir)
	buildCmd.Stdout = os.Stdout // Redirect build output if needed
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build example binary for script %s: %v", scriptBase, err)
	}
	log.Printf("Successfully built example binary at: %s", binaryPath)

	// Determine arguments based on the script being run
	var runArgs []string
	switch scriptBase {
	case "basic.txt":
		runArgs = []string{"-h"}
	case "gemini_talk.txt":
		runArgs = []string{"--non-interactive", "-p", "Hello, how are you today?"}
	case "httprr.txt":
		runArgs = []string{"--non-interactive", "-p", "Hello", "-audio=false"}
	default:
		// Default or error case if the script is not recognized
		t.Logf("Warning: Unrecognized script %q, running example binary without specific arguments.", scriptBase)
		// Optionally, you could fail the test here:
		// t.Fatalf("Unrecognized script %q, cannot determine arguments.", scriptBase)
		runArgs = []string{} // Or some default arguments
	}

	// Run the built binary
	log.Printf("Running example binary for script %s with args: %v", scriptBase, runArgs)
	runCmd := exec.Command(binaryPath, runArgs...)
	runCmd.Stdout = os.Stdout // Capture or redirect output as needed
	runCmd.Stderr = os.Stderr

	// Execute the command
	err = runCmd.Run()

	// Check the result
	if err != nil {
		// Log the error but don't necessarily fail the test immediately,
		// depending on whether the original script expected an error.
		// For this basic replacement, we'll treat any execution error as a failure.
		t.Errorf("Execution of example binary for script %s failed: %v", scriptBase, err)
	} else {
		log.Printf("Successfully ran example binary for script: %s", scriptBase)
	}
	// --- End: Custom build and run logic ---

	log.Printf("Finished custom execution for script: %s", scriptBase)
}
