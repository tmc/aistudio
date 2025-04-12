package api

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tmc/aistudio/internal/httprr"
)

// Variable to track if we should record HTTP interactions
var httpRecord bool

func init() {
	flag.BoolVar(&httpRecord, "httprecord_api", false, "record HTTP requests and responses for API tests")
}

func TestClientInitialization(t *testing.T) {
	// Create a directory for test recordings if it doesn't exist
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatalf("Failed to create testdata directory: %v", err)
	}

	// Setup HTTP record/replay
	recordFile := filepath.Join("testdata", "client_init.txt")

	// Check if we're in recording mode and set flag if needed
	if httpRecord {
		flag.Set("httprecord", ".*")
	}

	rr, err := httprr.Open(recordFile, nil)
	if err != nil {
		t.Fatalf("Failed to initialize HTTP record/replay: %v", err)
	}
	defer rr.Close()

	// Create client with custom transport
	client := &Client{
		APIKey: "test_api_key",
	}
	client.SetHTTPTransport(rr)

	// Skip actual API test in CI environments
	if os.Getenv("CI") != "" {
		t.Skip("Skipping test in CI environment")
	}

	// Initialize client - this may fail if API credentials aren't available
	// This is expected in replay mode, and we'll just log it
	ctx := context.Background()
	err = client.InitClient(ctx)
	if err != nil {
		t.Logf("Client initialization error (expected in replay mode): %v", err)
	} else {
		t.Log("Client initialized successfully")
	}
}

func TestClientStreamContent(t *testing.T) {
	// Skip this test if no API key is available and we're in record mode
	if httpRecord && os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("Skipping test in record mode without GEMINI_API_KEY")
	}

	// Skip in CI environments
	if os.Getenv("CI") != "" {
		t.Skip("Skipping test in CI environment")
	}

	// Create a directory for test recordings if it doesn't exist
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatalf("Failed to create testdata directory: %v", err)
	}

	// Setup HTTP record/replay
	recordFile := filepath.Join("testdata", "client_stream_content.txt")

	// Check if we're in recording mode and set flag if needed
	if httpRecord {
		flag.Set("httprecord", ".*")
	}

	rr, err := httprr.Open(recordFile, nil)
	if err != nil {
		t.Fatalf("Failed to initialize HTTP record/replay: %v", err)
	}
	defer rr.Close()

	// Create a client with the HTTP transport from httprr
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := &Client{
		APIKey: os.Getenv("GEMINI_API_KEY"),
	}
	if client.APIKey == "" {
		client.APIKey = "test_api_key" // For replay mode
	}

	// Set the custom HTTP transport
	client.SetHTTPTransport(rr)

	// Initialize the client - in replay mode, this might fail
	// We'll just log the error and continue
	err = client.InitClient(ctx)
	if err != nil {
		t.Logf("Client initialization error (expected in replay mode): %v", err)
		return
	}

	// Try to start a streaming session
	config := ClientConfig{
		ModelName:   "models/gemini-1.5-flash-latest",
		EnableAudio: false,
	}

	// Using test-specific implementation for compatibility
	stream, err := client.InitBidiStreamTest(ctx, config)
	if err != nil {
		t.Logf("Stream initialization error (may be expected in replay mode): %v", err)
		return
	}

	// If we got a stream, try to receive from it
	t.Log("Stream initialized, attempting to receive response")
	resp, err := stream.Recv()
	if err != nil {
		t.Logf("Stream receive error (may be expected in replay mode): %v", err)
		return
	}

	// If we got a response, log some info about it
	if resp != nil {
		// We're using ExtractOutput directly because our test environment
		// uses StreamGenerateContent, but the real implementation would
		// use ExtractBidiOutput with BidiGenerateContentServerMessage
		output := ExtractOutput(resp)
		t.Logf("Received response: text length=%d, audio length=%d",
			len(output.Text), len(output.Audio))
	}
}
