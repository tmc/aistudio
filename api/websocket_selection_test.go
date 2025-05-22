package api

import (
	"testing"
)

// TestWebSocketProtocolSelection verifies that the WebSocket selection logic works correctly
func TestWebSocketProtocolSelection(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		webSocketOn bool
		shouldUseWS bool
	}{
		{"Live model with WS enabled", "gemini-2.0-flash-live-001", true, true},
		{"Live model with WS disabled", "gemini-2.0-flash-live-001", false, false},
		{"New live model with WS enabled", "gemini-2.5-flash-live", true, true},
		{"New live model with WS disabled", "gemini-2.5-flash-live", false, false},
		{"Non-live model with WS enabled", "gemini-1.5-flash", true, false},
		{"Non-live model with WS disabled", "gemini-1.5-flash", false, false},
		{"Prefixed live model with WS enabled", "models/gemini-2.0-flash-live-001", true, true},
		{"Prefixed live model with WS disabled", "models/gemini-2.0-flash-live-001", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Check if it's a live model
			isLive := IsLiveModel(tc.model)

			// Determine if WebSocket should be used
			shouldUseWS := tc.webSocketOn && isLive

			// Verify against expected result
			if shouldUseWS != tc.shouldUseWS {
				t.Errorf("For %s (ws:%v): Got shouldUseWS=%v, expected %v",
					tc.model, tc.webSocketOn, shouldUseWS, tc.shouldUseWS)
			}

			// Test with StreamClientConfig
			config := &StreamClientConfig{
				ModelName:       tc.model,
				EnableWebSocket: tc.webSocketOn,
			}

			configShouldUseWS := config.EnableWebSocket && IsLiveModel(config.ModelName)
			if configShouldUseWS != tc.shouldUseWS {
				t.Errorf("With StreamClientConfig %s (ws:%v): Got shouldUseWS=%v, expected %v",
					tc.model, tc.webSocketOn, configShouldUseWS, tc.shouldUseWS)
			}
		})
	}
}
