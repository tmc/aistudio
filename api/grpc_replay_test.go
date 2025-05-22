package api

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveModelProtocolSelection tests the protocol selection logic for live models
func TestLiveModelProtocolSelection(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		webSocketOn bool
		shouldUseWS bool // Whether WebSocket should actually be used
	}{
		{"Live model with WS enabled", "gemini-2.0-flash-live-001", true, true},
		{"Live model with WS disabled", "gemini-2.0-flash-live-001", false, false},
		{"Non-live model with WS enabled", "gemini-1.5-flash", true, false},
		{"Non-live model with WS disabled", "gemini-1.5-flash", false, false},
		{"Live model 2.5 with WS enabled", "gemini-2.5-flash-live", true, true},
		{"Live model 2.5 with WS disabled", "gemini-2.5-flash-live", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Verify basic logic
			isLive := IsLiveModel(tc.model)
			shouldUseWS := tc.webSocketOn && isLive

			if shouldUseWS != tc.shouldUseWS {
				t.Errorf("For %s (ws:%v): Got shouldUseWS=%v, expected %v",
					tc.model, tc.webSocketOn, shouldUseWS, tc.shouldUseWS)
			}
		})
	}
}

// TestLiveModelIntegration runs integration tests with live models if environment variables are set
func TestLiveModelIntegration(t *testing.T) {
	// Skip test if E2E_TESTS environment variable is not set
	if os.Getenv("AISTUDIO_RUN_E2E_TESTS") != "1" {
		t.Skip("Skipping live model integration test; set AISTUDIO_RUN_E2E_TESTS=1 to run")
	}

	// Check for API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY environment variable not set")
	}

	// Create test matrix
	tests := []struct {
		name           string
		model          string
		useWebSocket   bool
		prompt         string
		expectedPhrase string
	}{
		{
			name:           "Standard model with gRPC",
			model:          "gemini-1.5-flash",
			useWebSocket:   false,
			prompt:         "Say hello and include the word 'standard'",
			expectedPhrase: "standard",
		},
		{
			name:           "Live model with gRPC",
			model:          "gemini-2.0-flash-live-001",
			useWebSocket:   false,
			prompt:         "Say hello and include the word 'live'",
			expectedPhrase: "live",
		},
		{
			name:           "Live model with WebSocket",
			model:          "gemini-2.0-flash-live-001",
			useWebSocket:   true,
			prompt:         "Say hello and include the word 'socket'",
			expectedPhrase: "socket",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create context
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Create client
			client := &Client{
				APIKey: apiKey,
			}

			// Initialize client
			err := client.InitClient(ctx)
			if err != nil {
				t.Fatalf("Failed to initialize client: %v", err)
			}
			defer client.Close()

			// Create stream config
			config := &StreamClientConfig{
				ModelName:       tc.model,
				EnableAudio:     false,
				SystemPrompt:    "You are a helpful assistant.",
				Temperature:     0.7,
				TopP:            0.95,
				TopK:            40,
				MaxOutputTokens: 128,
				EnableWebSocket: tc.useWebSocket,
			}

			// Initialize stream
			stream, err := client.InitBidiStream(ctx, config)
			if err != nil {
				t.Fatalf("Failed to initialize stream: %v", err)
			}
			defer stream.CloseSend()

			// Send message
			err = client.SendMessageToBidiStream(stream, tc.prompt)
			if err != nil {
				t.Fatalf("Failed to send message: %v", err)
			}

			// Receive response
			var response string
			for {
				resp, err := stream.Recv()
				if err != nil {
					if strings.Contains(err.Error(), "EOF") {
						break
					}
					t.Fatalf("Error receiving response: %v", err)
				}

				// Extract text
				output := ExtractOutput(resp)
				response += output.Text

				if output.TurnComplete {
					break
				}
			}

			// Verify response
			if !strings.Contains(strings.ToLower(response), strings.ToLower(tc.expectedPhrase)) {
				t.Errorf("Response does not contain expected phrase '%s'. Response: %s",
					tc.expectedPhrase, response)
			} else {
				t.Logf("Response contains expected phrase '%s'", tc.expectedPhrase)
			}
		})
	}
}
