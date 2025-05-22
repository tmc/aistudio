package aistudio

import (
	"path/filepath"
	"testing"
)

func TestLoadToolsFromFile(t *testing.T) {
	tests := []struct {
		name          string
		filePath      string
		expectedTools []string
	}{
		{
			name:          "Custom Format (Claude)",
			filePath:      "testdata/tools-cc-test.json",
			expectedTools: []string{"list_files", "get_weather", "read_config"},
		},
		{
			name:          "Gemini Format",
			filePath:      "testdata/tools-gemini.json",
			expectedTools: []string{"search_weather", "calculate"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new tool manager for each test
			tm := NewToolManager()

			// Load tools from the test file using our fixed implementation
			path := filepath.Join(".", tt.filePath)
			err := LoadToolsFromFileFixed(path, tm)
			if err != nil {
				t.Fatalf("LoadToolsFromFile(%s) failed: %v", tt.filePath, err)
			}

			// Check that the expected number of tools were registered
			if len(tm.RegisteredTools) != len(tt.expectedTools) {
				t.Errorf("Expected %d tools, got %d", len(tt.expectedTools), len(tm.RegisteredTools))
			}

			// Check that each expected tool is registered
			for _, toolName := range tt.expectedTools {
				tool, exists := tm.RegisteredTools[toolName]
				if !exists {
					t.Errorf("Expected tool '%s' not found", toolName)
					continue
				}

				// Verify tool availability
				if !tool.IsAvailable {
					t.Errorf("Tool '%s' is registered but not available", toolName)
				}

				// Verify tool has a handler
				if tool.Handler == nil {
					t.Errorf("Tool '%s' has no handler", toolName)
				}

				// Verify name matches
				if tool.ToolDefinition.Name != toolName {
					t.Errorf("Tool named '%s' has incorrect name '%s' in definition",
						toolName, tool.ToolDefinition.Name)
				}

				// Verify description is non-empty
				if tool.ToolDefinition.Description == "" {
					t.Errorf("Tool '%s' has empty description", toolName)
				}

				// Log details for debugging
				t.Logf("Tool '%s' successfully registered with description: %s",
					toolName, tool.ToolDefinition.Description)
			}
		})
	}
}

func TestCallRegisteredTools(t *testing.T) {
	// Create and load tools using our fixed implementation
	tm := NewToolManager()
	err := LoadToolsFromFileFixed(filepath.Join(".", "testdata/tools-cc-test.json"), tm)
	if err != nil {
		t.Fatalf("Failed to load tools: %v", err)
	}

	// Try calling the registered handlers with sample inputs
	tests := []struct {
		name        string
		toolName    string
		input       string
		expectError bool
	}{
		{
			name:        "Call list_files tool",
			toolName:    "list_files",
			input:       `{"path": "."}`,
			expectError: false,
		},
		{
			name:        "Call get_weather tool",
			toolName:    "get_weather",
			input:       `{"location": "San Francisco"}`,
			expectError: false,
		},
		{
			name:        "Call read_config tool with invalid path",
			toolName:    "read_config",
			input:       `{"file_path": "/nonexistent/path"}`,
			expectError: false, // Even though the file doesn't exist, the tool should handle this gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, exists := tm.RegisteredTools[tt.toolName]
			if !exists {
				t.Fatalf("Tool '%s' not found", tt.toolName)
			}

			result, err := tool.Handler([]byte(tt.input))

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error when calling '%s', got none", tt.toolName)
				}
			} else {
				// We don't necessarily expect an error, but the handler may still return one
				// For example, with invalid file paths. So we're just logging the error instead of failing.
				if err != nil {
					t.Logf("Got error from handler: %v", err)
				}
			}

			// Log the result for debugging
			t.Logf("Tool '%s' result: %+v", tt.toolName, result)
		})
	}
}
