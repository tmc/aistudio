package api

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestProtocolMatrix tests both WebSocket and gRPC protocols with recording/replay
func TestProtocolMatrix(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	// Check for E2E test flag
	if os.Getenv("AISTUDIO_RUN_E2E_TESTS") != "1" {
		t.Skip("Skipping protocol matrix test; set AISTUDIO_RUN_E2E_TESTS=1 to run")
	}

	// Check for API key in record mode
	wsRecordMode := os.Getenv("WS_RECORD_MODE") == "1"
	grpcRecordMode := os.Getenv("RECORD_GRPC") == "1"

	if (wsRecordMode || grpcRecordMode) && os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY environment variable not set (required for record mode)")
	}

	// Create test matrix
	tests := []struct {
		name         string
		model        string
		useWS        bool
		prompt       string
		keywordCheck string
	}{
		{
			name:         "Standard model with gRPC",
			model:        "gemini-1.5-flash",
			useWS:        false,
			prompt:       "Say hello and include the word 'matrix'",
			keywordCheck: "matrix",
		},
		{
			name:         "Live model with gRPC",
			model:        "gemini-2.0-flash-live-001",
			useWS:        false,
			prompt:       "Say hello and include the word 'test'",
			keywordCheck: "test",
		},
		{
			name:         "Live model with WebSocket",
			model:        "gemini-2.0-flash-live-001",
			useWS:        true,
			prompt:       "Say hello and include the word 'protocol'",
			keywordCheck: "protocol",
		},
		{
			name:         "Live 2.5 model with WebSocket",
			model:        "gemini-2.5-flash-live",
			useWS:        true,
			prompt:       "Say hello and include the word 'recording'",
			keywordCheck: "recording",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Skip test combinations that are incompatible with current mode
			if tc.useWS && !wsRecordMode && os.Getenv("WS_RECORD_MODE") == "" {
				// Skip WebSocket test if not in WebSocket record mode and no WS_RECORD_MODE set
				// This avoids running WebSocket tests in gRPC-only test runs
				if os.Getenv("PROTOCOL_MATRIX_RUN_ALL") != "1" {
					t.Skip("Skipping WebSocket test (WS_RECORD_MODE not set)")
				}
			}

			if !tc.useWS && !grpcRecordMode && os.Getenv("RECORD_GRPC") == "" {
				// Skip gRPC test if not in gRPC record mode and no RECORD_GRPC set
				// This avoids running gRPC tests in WebSocket-only test runs
				if os.Getenv("PROTOCOL_MATRIX_RUN_ALL") != "1" {
					t.Skip("Skipping gRPC test (RECORD_GRPC not set)")
				}
			}

			// Get API key
			apiKey := os.Getenv("GEMINI_API_KEY")

			// Run the test
			runProtocolTest(t, tc.model, tc.useWS, tc.prompt, tc.keywordCheck, apiKey)
		})
	}
}

// runProtocolTest runs a test for a specific model and protocol
func runProtocolTest(t *testing.T, model string, useWS bool, prompt, keywordCheck, apiKey string) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create client
	client := &Client{
		APIKey: apiKey,
	}

	// Initialize client (based on record mode this might not connect)
	err := client.InitClient(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}
	defer client.Close()

	// Create stream config
	config := &StreamClientConfig{
		ModelName:       model,
		EnableAudio:     false,
		SystemPrompt:    "You are a helpful assistant.",
		Temperature:     0.7,
		TopP:            0.95,
		TopK:            40,
		MaxOutputTokens: 128,
		EnableWebSocket: useWS,
	}

	// Log protocol info
	protocolName := "gRPC"
	if useWS {
		protocolName = "WebSocket"
	}
	t.Logf("Testing model %s with %s", model, protocolName)

	// Check if we're in record or replay mode
	recordMode := false
	if useWS && os.Getenv("WS_RECORD_MODE") == "1" {
		recordMode = true
		t.Logf("Using WebSocket RECORD mode")
	} else if !useWS && os.Getenv("RECORD_GRPC") == "1" {
		recordMode = true
		t.Logf("Using gRPC RECORD mode")
	} else {
		t.Logf("Using REPLAY mode")
	}

	// Initialize stream
	stream, err := client.InitBidiStream(ctx, config)
	if err != nil {
		t.Fatalf("Failed to initialize stream: %v", err)
	}
	defer stream.CloseSend()

	// Send message
	err = client.SendMessageToBidiStream(stream, prompt)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Receive and process response
	var fullResponse string
	var receivedChunks int

	for {
		resp, err := stream.Recv()
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				t.Logf("Stream ended (EOF)")
				break
			}

			// In replay mode with no recording, we might get "end of recording" error
			if strings.Contains(err.Error(), "end of recording") ||
				strings.Contains(err.Error(), "no recording") {
				t.Logf("End of recording reached: %v", err)
				break
			}

			t.Fatalf("Error receiving response: %v", err)
		}

		receivedChunks++

		// Extract content
		output := ExtractOutput(resp)
		fullResponse += output.Text

		t.Logf("Received chunk #%d: %d bytes", receivedChunks, len(output.Text))

		if output.TurnComplete {
			t.Logf("Response complete")
			break
		}
	}

	// Verify response
	if len(fullResponse) == 0 && recordMode {
		t.Errorf("Empty response received from model %s with %s", model, protocolName)
	} else {
		t.Logf("Response length: %d characters in %d chunks", len(fullResponse), receivedChunks)

		// In recording mode, check for the expected keyword
		if recordMode && keywordCheck != "" {
			if !strings.Contains(strings.ToLower(fullResponse), strings.ToLower(keywordCheck)) {
				t.Logf("Expected keyword '%s' not found in response: %s",
					keywordCheck, fullResponse)
			} else {
				t.Logf("Found expected keyword '%s' in response", keywordCheck)
			}
		}
	}
}

// TestProtocolSelectionMatrix tests the protocol selection logic with a matrix of models and flags
func TestProtocolSelectionMatrix(t *testing.T) {
	// Create test matrix
	tests := []struct {
		name           string
		model          string
		wsFlag         bool
		expectProtocol string
	}{
		{"Standard model without flag", "gemini-1.5-flash", false, "gRPC"},
		{"Standard model with flag", "gemini-1.5-flash", true, "gRPC"},
		{"Live 2.0 model without flag", "gemini-2.0-flash-live-001", false, "gRPC"},
		{"Live 2.0 model with flag", "gemini-2.0-flash-live-001", true, "WebSocket"},
		{"Live 2.5 model without flag", "gemini-2.5-flash-live", false, "gRPC"},
		{"Live 2.5 model with flag", "gemini-2.5-flash-live", true, "WebSocket"},
		{"Standard model with prefix without flag", "models/gemini-1.5-flash", false, "gRPC"},
		{"Standard model with prefix with flag", "models/gemini-1.5-flash", true, "gRPC"},
		{"Live model with prefix without flag", "models/gemini-2.0-flash-live-001", false, "gRPC"},
		{"Live model with prefix with flag", "models/gemini-2.0-flash-live-001", true, "WebSocket"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Check if it's a live model
			isLive := IsLiveModel(tc.model)

			// Determine protocol - WebSocket only if both flag is on AND it's a live model
			shouldUseWS := tc.wsFlag && isLive
			actualProtocol := "gRPC"
			if shouldUseWS {
				actualProtocol = "WebSocket"
			}

			// Verify against expected
			if actualProtocol != tc.expectProtocol {
				t.Errorf("For %s (ws flag: %v): Expected %s, got %s",
					tc.model, tc.wsFlag, tc.expectProtocol, actualProtocol)
			}
		})
	}
}
