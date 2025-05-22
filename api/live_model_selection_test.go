package api

import (
	"testing"
)

// TestLiveModelSelection verifies that IsLiveModel correctly identifies live models
func TestLiveModelSelection(t *testing.T) {
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
		{"Upper case model name", "GEMINI-2.5-FLASH-LIVE", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsLiveModel(tc.model)
			if result != tc.expected {
				t.Errorf("IsLiveModel(%s) = %v, want %v", tc.model, result, tc.expected)
			}
		})
	}
}
