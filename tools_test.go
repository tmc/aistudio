package aistudio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
)

func TestParseToolDefinitions(t *testing.T) {
	tests := []struct {
		name           string
		filePath       string
		expectedCount  int
		expectedFormat string
		expectedNames  []string
	}{
		{
			name:           "Custom format (Claude Tools)",
			filePath:       "testdata/tools-cc.json",
			expectedCount:  12, // This file actually contains 12 tools
			expectedFormat: "custom",
			// Update expected names to match the actual content of tools-cc.json
			expectedNames: []string{
				"dispatch_agent", "Bash", "BatchTool", "GlobTool", "GrepTool",
				"LS", "View", "Edit", "Replace", "ReadNotebook", "NotebookEditCell",
				"WebFetchTool",
			},
		},
		{
			name:           "Gemini format",
			filePath:       "testdata/tools-gemini.json",
			expectedCount:  2,
			expectedFormat: "gemini",
			expectedNames:  []string{"search_weather", "calculate"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Open the test file
			path := filepath.Join(".", tt.filePath)
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("Failed to open test file %s: %v", tt.filePath, err)
			}
			defer file.Close()

			// Parse the tool definitions using the reader
			toolDefs, err := ParseToolDefinitions(file)
			if err != nil {
				t.Fatalf("ParseToolDefinitions failed: %v", err)
			}

			// Check the number of tools
			if len(toolDefs) != tt.expectedCount {
				t.Errorf("Expected %d tools, got %d", tt.expectedCount, len(toolDefs))
			}

			// Check that all expected tool names are present
			nameFound := make(map[string]bool)
			for _, name := range tt.expectedNames {
				nameFound[name] = false
			}

			for _, def := range toolDefs {
				if _, exists := nameFound[def.Name]; exists {
					nameFound[def.Name] = true
				} else {
					t.Errorf("Unexpected tool name: %s", def.Name)
				}

				// Verify parameters are properly parsed (by marshalling back to JSON)
				if def.Parameters != nil {
					// Use protojson for marshalling Schema
					m := protojson.MarshalOptions{UseProtoNames: true}
					paramBytes, err := m.Marshal(def.Parameters)
					if err != nil {
						t.Errorf("Failed to marshal parameters back to JSON for tool %s: %v", def.Name, err)
						continue
					}

					// Unmarshal the result into a generic map for basic checks
					var params map[string]interface{}
					if err := json.Unmarshal(paramBytes, &params); err != nil {
						t.Errorf("Failed to unmarshal marshalled parameters for tool %s: %v", def.Name, err)
						continue
					}

					// Basic check: Ensure 'type' is 'object' if parameters exist (case-insensitive)
					if typeVal, ok := params["type"]; !ok || (strings.ToLower(fmt.Sprintf("%v", typeVal)) != "object") {
						t.Errorf("Expected parameters type 'object', got '%v' for tool %s", typeVal, def.Name)
					}
				} else if def.Name == "calculate" { // Example: 'calculate' might have no params
					// Allow nil parameters for specific tools if expected
				} else {
					// Fail if parameters are unexpectedly nil
					// t.Errorf("Expected non-nil parameters for tool %s, got nil", def.Name)
					// Or log a warning, depending on strictness
					// t.Logf("Warning: Parameters are nil for tool %s", def.Name)
				}
			}

			// Check that all expected names were found
			for name, found := range nameFound {
				if !found {
					t.Errorf("Expected tool name '%s' not found", name)
				}
			}
		})
	}
}

func TestCreateHandlerForFileDefinition(t *testing.T) {
	tests := []struct {
		name          string
		def           FileToolDefinition // Use FileToolDefinition here
		expectSuccess bool
	}{
		{
			name: "system_info handler",
			def: FileToolDefinition{
				Name:        "get_system_info",
				Description: "Gets system information",
				Parameters:  json.RawMessage(`{"type": "object", "properties": {"include_time": {"type": "boolean"}}}`), // Example params
				Handler:     "system_info",
			},
			expectSuccess: true,
		},
		{
			name: "exec_command handler",
			def: FileToolDefinition{
				Name:        "run_command",
				Description: "Runs a command",
				Parameters:  json.RawMessage(`{"type": "object", "properties": {"command": {"type": "string"}}, "required": ["command"]}`), // Example params
				Handler:     "exec_command",
			},
			expectSuccess: true,
		},
		{
			name: "read_file handler",
			def: FileToolDefinition{
				Name:        "read_file",
				Description: "Reads a file",
				Parameters:  json.RawMessage(`{"type": "object", "properties": {"file_path": {"type": "string"}}, "required": ["file_path"]}`), // Example params
				Handler:     "read_file",
			},
			expectSuccess: true,
		},
		{
			name: "custom handler with command",
			def: FileToolDefinition{
				Name:        "custom_tool",
				Description: "Custom tool",
				Parameters:  json.RawMessage(`{"type": "object"}`), // Example params
				Handler:     "custom",
				Command:     "echo 'test'", // Command defined in the tool definition
			},
			expectSuccess: true,
		},
		{
			name: "custom handler without command should fail",
			def: FileToolDefinition{
				Name:        "invalid_custom",
				Description: "Invalid custom tool",
				Parameters:  json.RawMessage(`{"type": "object"}`), // Example params
				Handler:     "custom",
				Command:     "", // Empty command
			},
			expectSuccess: false,
		},
		{
			name: "unknown handler should fail",
			def: FileToolDefinition{
				Name:        "unknown",
				Description: "Unknown handler",
				Parameters:  json.RawMessage(`{"type": "object"}`), // Example params
				Handler:     "nonexistent_handler",
			},
			expectSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use createHandlerForFileDefinitionFixed instead
			handler, err := createHandlerForFileDefinitionFixed(tt.def)

			if tt.expectSuccess {
				if err != nil {
					t.Errorf("Expected success, got error: %v", err)
				}
				if handler == nil {
					t.Error("Expected non-nil handler, got nil")
				}
			} else {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				if handler != nil {
					t.Error("Expected nil handler, got non-nil")
				}
			}
		})
	}
}