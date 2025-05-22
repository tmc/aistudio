package api

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
)

// TestLiveModelE2E tests end-to-end functionality with Gemini live models
func TestLiveModelE2E(t *testing.T) {
	// Skip test if E2E_TESTS environment variable is not set
	if os.Getenv("AISTUDIO_RUN_E2E_TESTS") != "1" {
		t.Skip("Skipping live model E2E test; set AISTUDIO_RUN_E2E_TESTS=1 to run")
	}

	// Check for API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY environment variable not set")
	}

	// List of tests to run
	tests := []struct {
		name       string
		modelName  string
		prompt     string
		expectWord string
	}{
		{
			name:       "Standard model baseline",
			modelName:  "models/gemini-1.5-flash",
			prompt:     "Hello, please include the word 'testing' in your response.",
			expectWord: "testing",
		},
		{
			name:       "Live model basic",
			modelName:  "models/gemini-2.0-flash-live-001",
			prompt:     "Hello, please include the word 'connectivity' in your response.",
			expectWord: "connectivity",
		},
		{
			name:       "Live model with code",
			modelName:  "models/gemini-2.0-flash-live-001",
			prompt:     "Write a simple Python function that prints 'Hello World'. Include the word 'function' in your explanation.",
			expectWord: "function",
		},
	}

	// Run each test
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runLiveModelTest(t, apiKey, tt.modelName, tt.prompt, tt.expectWord)
		})
	}
}

// runLiveModelTest runs a test with a specific model and prompt
func runLiveModelTest(t *testing.T, apiKey, modelName, prompt, expectWord string) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create client
	client := &Client{
		APIKey: apiKey,
	}

	if err := client.InitClient(ctx); err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}
	defer client.Close()

	// Create stream config
	config := &StreamClientConfig{
		ModelName:       modelName,
		Temperature:     0.2,
		MaxOutputTokens: 200,
	}

	// Initialize stream based on model type
	var stream generativelanguagepb.GenerativeService_StreamGenerateContentClient
	var err error

	isLive := IsLiveModel(modelName)
	if isLive {
		t.Logf("Testing with live model: %s", modelName)
		stream, err = client.initLiveStream(ctx, config)
	} else {
		t.Logf("Testing with standard model: %s", modelName)
		stream, err = client.InitBidiStream(ctx, config)
	}

	if err != nil {
		t.Fatalf("Failed to initialize stream: %v", err)
	}
	defer stream.CloseSend()

	// Send message using existing method
	err = client.SendMessageToBidiStream(stream, prompt)
	if err != nil {
		t.Fatalf("Failed to send message to stream: %v", err)
	}

	// Receive and accumulate response
	var fullResponse string
	var receivedChunks int
	var tokensUsed int32

	t.Logf("Waiting for response...")
	for {
		resp, err := stream.Recv()
		if err != nil {
			// End of stream is expected
			t.Logf("Stream ended: %v", err)
			break
		}

		receivedChunks++

		// Extract text from response
		output := ExtractBidiOutput(resp)
		fullResponse += output.Text
		tokensUsed = output.TotalTokenCount

		t.Logf("Received chunk #%d: %d bytes", receivedChunks, len(output.Text))

		if output.TurnComplete {
			t.Logf("Turn complete")
			break
		}
	}

	// Check for non-empty response
	if fullResponse == "" {
		t.Errorf("Empty response from model %s", modelName)
	} else {
		t.Logf("Response length: %d characters, chunks: %d, tokens: %d", len(fullResponse), receivedChunks, tokensUsed)
		t.Logf("Response sample: %s...", truncateLiveModelString(fullResponse, 100))
	}

	// Check for expected word in response
	lowerResponse := strings.ToLower(fullResponse)
	if !strings.Contains(lowerResponse, strings.ToLower(expectWord)) {
		t.Errorf("Response doesn't contain the expected '%s' keyword", expectWord)
	} else {
		t.Logf("Successfully found '%s' in the response", expectWord)
	}
}

// truncateLiveModelString truncates a string to the specified length and adds ellipsis if needed
func truncateLiveModelString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// TestLiveModelDetection tests that IsLiveModel correctly identifies live models
func TestLiveModelDetection(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"Standard model", "gemini-1.5-flash", false},
		{"Standard model with prefix", "models/gemini-1.5-flash", false},
		{"Live model 2.0", "gemini-2.0-flash-live-001", true},
		{"Live model 2.0 with prefix", "models/gemini-2.0-flash-live-001", true},
		{"Live model 2.5", "gemini-2.5-flash-live", true},
		{"Live model 2.5 with prefix", "models/gemini-2.5-flash-live", true},
		{"Random model with live in name", "random-live-model", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLiveModel(tt.model)
			if result != tt.expected {
				t.Errorf("IsLiveModel(%s) = %v, want %v", tt.model, result, tt.expected)
			}
		})
	}
}
