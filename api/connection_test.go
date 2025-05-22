package api_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tmc/aistudio/api"
)

// TestConnectionStability tests that connections remain stable
// This test will be skipped if no API key is available
func TestConnectionStability(t *testing.T) {
	// Skip these tests unless the AISTUDIO_RUN_E2E_TESTS environment variable is set
	// They require a valid API key with exact model access and may fail in CI environments
	if os.Getenv("AISTUDIO_RUN_E2E_TESTS") == "" {
		t.Skip("Skipping connection stability tests - set AISTUDIO_RUN_E2E_TESTS=1 to run")
	}
	// Skip if API_KEY is not set and we're not recording
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping connection test: No API Key provided")
	}

	// Create client with the API key
	client := &api.Client{
		APIKey: apiKey,
	}

	// Create a context with timeout (2 minutes for the entire test)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Initialize the client
	if err := client.InitClient(ctx); err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}
	defer client.Close()

	t.Log("Client initialized successfully")

	// Set up config
	config := &api.StreamClientConfig{
		ModelName: "models/gemini-1.5-flash-latest",
	}

	// Open a stream
	bidiStream, err := client.InitBidiStream(ctx, config)
	if err != nil {
		t.Fatalf("Failed to initialize stream: %v", err)
	}
	t.Log("Stream established successfully")

	// Test connection stability by sending keepalive messages
	for i := 0; i < 3; i++ {
		// Wait for a moment between pings
		time.Sleep(5 * time.Second)

		// Send a ping message - just an empty message to check the connection
		if err := client.SendMessageToBidiStream(bidiStream, "ping"); err != nil {
			t.Fatalf("Failed to send keepalive message %d: %v", i+1, err)
		}

		t.Logf("Sent keepalive message %d successfully", i+1)
	}

	// Test complete
	t.Log("Connection stayed stable for the duration of the test")
}
