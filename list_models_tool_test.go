package aistudio

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestListModelsToolHandlerWithFilter(t *testing.T) {
	// Create a tool manager
	tm := NewToolManager()

	// Initialize it with default tools (including the list_models tool)
	err := tm.RegisterDefaultTools()
	if err != nil {
		t.Fatalf("Failed to register default tools: %v", err)
	}

	// Get the list_models tool
	listModelsTool, exists := tm.RegisteredTools["list_models"]
	if !exists {
		t.Fatal("list_models tool not found in registered tools")
	}

	// Create a simple test case with a filter for gemini-2.0 models
	testArgs := json.RawMessage(`{"filter": "gemini-2.0"}`)

	// Execute the tool handler
	result, err := listModelsTool.Handler(testArgs)
	if err != nil {
		t.Fatalf("Failed to execute list_models tool: %v", err)
	}

	// Log the raw result for debugging
	resultBytes, _ := json.MarshalIndent(result, "", "  ")
	t.Logf("ListModels tool result: %s", string(resultBytes))

	// Check if the result is valid
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Unexpected result type: %T", result)
	}

	// Check if the operation was successful
	success, ok := resultMap["success"].(bool)
	if !ok || !success {
		errMsg := "unknown error"
		if errStr, ok := resultMap["error"].(string); ok {
			errMsg = errStr
		}
		t.Fatalf("Tool execution failed: %s", errMsg)
	}

	// Check if we got some models
	modelsInterface, ok := resultMap["models"]
	if !ok {
		t.Fatalf("No models field found in result: %v", resultMap)
	}

	// Try first as []string (from fallback list)
	models, ok := modelsInterface.([]string)
	if !ok {
		// Try as []interface{} (from API)
		modelsArray, ok := modelsInterface.([]interface{})
		if !ok {
			t.Fatalf("Models field is not a recognized array type. Type: %T, Value: %v", modelsInterface, modelsInterface)
		}

		if len(modelsArray) == 0 {
			t.Fatalf("Models list is empty")
		}

		// Convert to string slice
		models = make([]string, 0, len(modelsArray))
		for _, m := range modelsArray {
			if mStr, ok := m.(string); ok {
				models = append(models, mStr)
			}
		}
	}

	if len(models) == 0 {
		t.Fatalf("Models list is empty after conversion")
	}

	t.Logf("Found %d models matching filter 'gemini-2.0'", len(models))

	// Check if we got filtered results (all should contain gemini-2.0)
	for _, model := range models {
		if model == "" {
			t.Errorf("Found empty model name in results")
			continue
		}

		if !contains(model, "gemini-2.0") {
			t.Errorf("Model %s doesn't match filter gemini-2.0", model)
		}
	}
}

func TestListModelsToolHandlerWithAPIVersions(t *testing.T) {
	// Skip if real testing is not possible
	t.Skip("Skipping test that requires real API connectivity")

	// Create a tool manager
	tm := NewToolManager()

	// Initialize it with default tools (including the list_models tool)
	err := tm.RegisterDefaultTools()
	if err != nil {
		t.Fatalf("Failed to register default tools: %v", err)
	}

	// Get the list_models tool
	listModelsTool, exists := tm.RegisteredTools["list_models"]
	if !exists {
		t.Fatal("list_models tool not found in registered tools")
	}

	// Create a test case with specific API versions
	testArgs := json.RawMessage(`{"api_versions": ["alpha", "beta"]}`)

	// Execute the tool handler
	result, err := listModelsTool.Handler(testArgs)
	if err != nil {
		t.Fatalf("Failed to execute list_models tool: %v", err)
	}

	// Log the result for inspection
	resultBytes, _ := json.MarshalIndent(result, "", "  ")
	t.Logf("ListModels tool result: %s", string(resultBytes))
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	if s == "" || substr == "" {
		return false
	}

	// Convert both to lowercase for case-insensitive comparison
	sLower := strings.ToLower(s)
	substrLower := strings.ToLower(substr)

	return strings.Contains(sLower, substrLower)
}
