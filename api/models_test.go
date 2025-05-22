package api

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tmc/aistudio/internal/httprr"
)

func TestValidateModel(t *testing.T) {
	client := &Client{}

	// Test a valid model
	valid, err := client.ValidateModel("gemini-1.5-flash")
	if err != nil {
		t.Fatalf("ValidateModel failed: %v", err)
	}
	if !valid {
		t.Errorf("Expected 'gemini-1.5-flash' to be valid, but it was rejected")
	}

	// Test a valid model with prefix
	valid, err = client.ValidateModel("models/gemini-1.5-flash")
	if err != nil {
		t.Fatalf("ValidateModel failed: %v", err)
	}
	if !valid {
		t.Errorf("Expected 'models/gemini-1.5-flash' to be valid, but it was rejected")
	}

	// Test a completely invalid model (not a gemini model)
	valid, err = client.ValidateModel("gpt-4")
	if err != nil {
		t.Fatalf("ValidateModel failed: %v", err)
	}
	if valid {
		t.Errorf("Expected 'gpt-4' to be invalid, but it was accepted")
	}

	// Test another invalid model
	valid, err = client.ValidateModel("claude-3")
	if err != nil {
		t.Fatalf("ValidateModel failed: %v", err)
	}
	if valid {
		t.Errorf("Expected 'claude-3' to be invalid, but it was accepted")
	}
}

func TestListModels(t *testing.T) {
	client := &Client{}

	// Test listing all models using the hardcoded list
	models, err := client.getStandardModels("")
	if err != nil {
		t.Fatalf("getStandardModels failed: %v", err)
	}
	if len(models) == 0 {
		t.Errorf("Expected non-empty list of models, but got empty list")
	}

	// Test filtering models from hardcoded list
	filteredModels, err := client.getStandardModels("1.5")
	if err != nil {
		t.Fatalf("getStandardModels with filter failed: %v", err)
	}
	for _, model := range filteredModels {
		if !containsSubstring(model, "1.5") {
			t.Errorf("Expected filtered model to contain '1.5', but got: %s", model)
		}
	}

	// Regular ListModels will fall back to hardcoded list if API is unavailable
	apiModels, err := client.ListModels("")
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(apiModels) == 0 {
		t.Errorf("Expected non-empty list of models, but got empty list")
	}
}

func TestClientListModelsAPI(t *testing.T) {
	// Skip this test if no API key is available
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("Skipping API test without GEMINI_API_KEY")
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
	recordFile := filepath.Join("testdata", "client_list_models.txt")

	// Check if we're in recording mode
	recordMode := os.Getenv("HTTP_RECORD") != ""

	if recordMode {
		flag.Set("httprecord", ".*")
	}

	rr, err := httprr.Open(recordFile, nil)
	if err != nil && !recordMode {
		t.Logf("Failed to open recording file: %v", err)
		t.Skip("Skipping test without recording file")
		return
	}
	if err != nil {
		t.Fatalf("Failed to initialize HTTP record/replay: %v", err)
	}
	defer rr.Close()

	// Create a client with the HTTP transport from httprr
	client := &Client{
		APIKey: os.Getenv("GEMINI_API_KEY"),
	}

	// Set the custom HTTP transport
	client.SetHTTPTransport(rr)

	// Initialize the client
	ctx := context.Background()
	err = client.InitClient(ctx)
	if err != nil {
		t.Fatalf("Client initialization failed: %v", err)
	}

	// Test the ListModels function with API
	t.Run("ListModelsAPI", func(t *testing.T) {
		// Try to list all models
		models, err := client.ListModels("")
		if err != nil {
			t.Fatalf("ListModels failed: %v", err)
		}

		// Verify that we got at least some models
		if len(models) == 0 {
			t.Errorf("Expected non-empty list of models, but got empty list")
			return
		}

		// Log the models we found
		t.Logf("Found %d models", len(models))
		for i, model := range models {
			if i < 10 { // Only log the first 10 to avoid excessive output
				t.Logf("Model: %s", model)
			}
		}

		// Check if we found some expected model names
		found := false
		for _, model := range models {
			if strings.Contains(model, "gemini") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("No Gemini models found in the results")
		}
	})

	// Test filtering
	t.Run("FilterModelsAPI", func(t *testing.T) {
		// Try to list models with a filter
		filter := "gemini-2"
		models, err := client.ListModels(filter)
		if err != nil {
			t.Fatalf("ListModels with filter failed: %v", err)
		}

		// Verify that we got filtered results
		t.Logf("Found %d models with filter '%s'", len(models), filter)
		for _, model := range models {
			if !strings.Contains(strings.ToLower(model), strings.ToLower(filter)) {
				t.Errorf("Model %s doesn't match filter %s", model, filter)
			}
		}
	})
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
