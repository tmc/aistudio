package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"rsc.io/script/scripttest"
)

func main() {
	flag.Parse()

	// Prepare environment
	testDir := filepath.Join("testdata", "script")

	if len(os.Args) > 1 {
		// If a specific script is provided, run only that one
		scriptTest := os.Args[1]
		pathToScript := filepath.Join(testDir, scriptTest)
		if _, err := os.Stat(pathToScript); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Script not found: %s\n", pathToScript)
			os.Exit(1)
		}
		testDir = pathToScript
	}

	fmt.Printf("Running script tests from: %s\n", testDir)

	// Create a test runner function
	ok := true
	testFn := func(t *testing.T) {
		scripttest.Run(t, nil, nil, testDir, nil)
	}

	// Create a dummy test structure to pass to Run
	testing := &testing.T{}
	testing.Run("ScriptTest", testFn)

	if !ok {
		os.Exit(1)
	}
}
