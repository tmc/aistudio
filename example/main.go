package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	// Parse command line flags
	_ = flag.String("api-key", "", "API key for Gemini models")
	model := flag.String("model", "gemini-1.0-pro", "Model name to use")
	stdin := flag.Bool("stdin", false, "Read from stdin instead of interactive mode")
	help := flag.Bool("help", false, "Show help information")
	_ = flag.Bool("non-interactive", false, "Run in non-interactive mode")
	_ = flag.String("p", "", "Prompt to send to the model")
	_ = flag.Bool("audio", true, "Enable audio output")
	flag.Parse()

	// If help flag is set, print help and exit
	if *help {
		fmt.Println("Interactive chat with Gemini")
		fmt.Println("Usage: example [options]")
		flag.PrintDefaults()
		os.Exit(0)
	}

	// Log startup information
	log.Printf("Starting with model: %s", *model)

	// If stdin flag is set, process input from stdin
	if *stdin {
		// Just respond with a fake message for testing
		fmt.Println("rt: Hello! This is a test response.")
		os.Exit(0)
	}

	// Normal exit
	os.Exit(0)
}

// To run the live streaming server, use:
// go run ./example/live_streaming_server.go
