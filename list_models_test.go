package aistudio

import (
	"testing"

	"github.com/tmc/aistudio/api"
)

func TestDefaultModelIsValid(t *testing.T) {
	// Skip this test if no API key is available
	client := &api.Client{}

	// Test validating the default model
	valid, err := client.ValidateModel(DefaultModel)
	if err != nil {
		t.Fatalf("ValidateModel failed: %v", err)
	}

	if !valid {
		t.Errorf("DefaultModel '%s' is not valid according to ValidateModel", DefaultModel)
	}
}
