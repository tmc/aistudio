package api

import (
	"testing"
)

// Place your implementation test here if needed

// TestProtocolSelection tests that the appropriate protocol is selected based on model and flags
func TestBasicProtocolSelection(t *testing.T) {
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
		})
	}
}
