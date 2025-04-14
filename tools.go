package aistudio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/ai/generativelanguage/apiv1alpha/generativelanguagepb"
	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/tmc/aistudio/api"
)

// Use the FunctionDeclaration type directly from the API package
type ToolDefinition = api.ToolDefinition

// The structures defined here represent the tool calling API used by Gemini models

// ToolCall represents a tool call request from the model
type ToolCall struct {
	ID        string          `json:"id"`        // Unique ID for this tool call
	Name      string          `json:"name"`      // Name of the tool to call
	Arguments json.RawMessage `json:"arguments"` // Arguments in JSON format
}

// ToolResult represents the result of executing a tool call
type ToolResult = api.ToolResult

// ToolManager manages tool registration and execution
type ToolManager struct {
	// RegisteredTools holds all available tools that can be called
	RegisteredTools    map[string]api.RegisteredTool
	RegisteredToolDefs []ToolDefinition // Store the tool definitions for reference
}

// tempParamSchema is used for intermediate unmarshalling of JSON schema parameters
// where types are represented as strings (e.g., "string", "object").
type tempParamSchema struct {
	Type        string                      `json:"type"`
	Description string                      `json:"description,omitempty"`
	Format      string                      `json:"format,omitempty"`
	Nullable    bool                        `json:"nullable,omitempty"`
	Enum        []string                    `json:"enum,omitempty"`
	Properties  map[string]*tempParamSchema `json:"properties,omitempty"` // Recursive for object
	Required    []string                    `json:"required,omitempty"`   // For object
	Items       *tempParamSchema            `json:"items,omitempty"`      // Recursive for array
}

// convertTempSchemaToProtoSchema converts the intermediate schema representation
// (with string types) into the generativelanguagepb.Schema format (with enum types).
func convertTempSchemaToProtoSchema(temp *tempParamSchema) (*generativelanguagepb.Schema, error) {
	if temp == nil {
		return nil, nil
	}

	protoSchema := &generativelanguagepb.Schema{
		Description: temp.Description,
		Nullable:    temp.Nullable,
		Format:      temp.Format,
		Enum:        temp.Enum,
	}

	// Convert the type string to the corresponding GenerationConfig_Type enum
	switch strings.ToLower(temp.Type) {
	case "string":
		protoSchema.Type = generativelanguagepb.Type_STRING
	case "integer", "int", "int32", "int64":
		protoSchema.Type = generativelanguagepb.Type_INTEGER
	case "number", "float", "double":
		protoSchema.Type = generativelanguagepb.Type_NUMBER
	case "boolean", "bool":
		protoSchema.Type = generativelanguagepb.Type_BOOLEAN
	case "array":
		protoSchema.Type = generativelanguagepb.Type_ARRAY
		if temp.Items != nil {
			var err error
			protoSchema.Items, err = convertTempSchemaToProtoSchema(temp.Items)
			if err != nil {
				return nil, fmt.Errorf("failed to convert array items: %w", err)
			}
		}
	case "object":
		protoSchema.Type = generativelanguagepb.Type_OBJECT
		protoSchema.Properties = make(map[string]*generativelanguagepb.Schema)
		for key, propTempSchema := range temp.Properties {
			var err error
			protoSchema.Properties[key], err = convertTempSchemaToProtoSchema(propTempSchema)
			if err != nil {
				return nil, fmt.Errorf("failed to convert property '%s': %w", key, err)
			}
		}
		protoSchema.Required = temp.Required
	case "":
		// If type is empty, treat as unspecified. Could potentially infer object if properties exist,
		// but explicit type is preferred.
		protoSchema.Type = generativelanguagepb.Type_TYPE_UNSPECIFIED
		if len(temp.Properties) > 0 {
			log.Printf("Warning: Schema for field description '%s' has properties but no 'type' specified. Treating as unspecified.", temp.Description)
		}
	default:
		return nil, fmt.Errorf("unsupported schema type: '%s' for field description '%s'", temp.Type, temp.Description)
	}

	return protoSchema, nil
}

// NewToolManager creates a new tool manager
func NewToolManager() *ToolManager {
	return &ToolManager{
		RegisteredTools: make(map[string]api.RegisteredTool),
	}
}

// Message to indicate that a tool call has been sent
type toolCallSentMsg struct{}

// Message for tool call results
type toolCallResultMsg struct {
	results []ToolResult
}

// ParseToolDefinitions reads tool definitions from an io.Reader.
// It attempts to parse the JSON data into a structure compatible with common tool definition formats,
// converting JSON schema parameters (with string types) into the required protobuf Schema format.
func ParseToolDefinitions(in io.Reader) ([]ToolDefinition, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return nil, fmt.Errorf("failed to read tool definitions: %w", err)
	}

	var finalToolDefs []ToolDefinition
	var parseErrs []string

	// Attempt 1: Parse as []FileToolDefinition (Handles aistudio/Claude-like format from testdata)
	var fileToolDefs []FileToolDefinition
	err = json.Unmarshal(data, &fileToolDefs)
	if err == nil && len(fileToolDefs) > 0 {
		// Convert FileToolDefinition to ToolDefinition
		finalToolDefs = make([]ToolDefinition, 0, len(fileToolDefs))
		conversionErrs := []string{} // Collect errors during conversion

		for _, fileDef := range fileToolDefs {
			var tempSchema tempParamSchema
			var protoSchema *generativelanguagepb.Schema
			var convertErr error

			// Unmarshal parameters if present and valid JSON
			if len(fileDef.Parameters) > 0 && string(fileDef.Parameters) != "null" {
				// Ensure the parameters field contains valid JSON before attempting unmarshal
				if !json.Valid(fileDef.Parameters) {
					conversionErrs = append(conversionErrs, fmt.Sprintf("tool '%s': parameters field contains invalid JSON", fileDef.Name))
					continue // Skip tool with invalid parameters JSON
				}

				if err := json.Unmarshal(fileDef.Parameters, &tempSchema); err != nil {
					conversionErrs = append(conversionErrs, fmt.Sprintf("tool '%s': failed to unmarshal parameters JSON: %v", fileDef.Name, err))
					continue // Skip tool with unmarshal error
				}

				// Convert the temporary schema to the protobuf schema
				protoSchema, convertErr = convertTempSchemaToProtoSchema(&tempSchema)
				if convertErr != nil {
					conversionErrs = append(conversionErrs, fmt.Sprintf("tool '%s': failed to convert parameters schema: %v", fileDef.Name, convertErr))
					continue // Skip tool with conversion error
				}
			}

			finalToolDefs = append(finalToolDefs, ToolDefinition{
				Name:        fileDef.Name,
				Description: fileDef.Description,
				Parameters:  protoSchema, // Assign the converted schema
			})
		}
		// If we successfully converted at least one tool, return them.
		// Also return any non-fatal parsing/conversion errors encountered.
		if len(finalToolDefs) > 0 {
			var combinedErr error
			if len(conversionErrs) > 0 {
				combinedErr = fmt.Errorf("encountered errors during parameter processing: %s", strings.Join(conversionErrs, "; "))
			}
			return finalToolDefs, combinedErr // Return successfully converted tools and any errors
		}
		// If parsing succeeded but resulted in zero tools after filtering errors
		parseErrs = append(parseErrs, fmt.Sprintf("parsing as []FileToolDefinition succeeded but yielded no valid tools after parameter processing: %s", strings.Join(conversionErrs, "; ")))

	} else if err != nil {
		parseErrs = append(parseErrs, fmt.Sprintf("parsing as []FileToolDefinition failed: %v", err))
	} else {
		// It's valid JSON, but an empty list or not the expected structure
		parseErrs = append(parseErrs, "parsing as []FileToolDefinition resulted in empty list or unexpected structure")
	}

	// Attempt 2: Parse as Gemini format (if Attempt 1 failed or yielded no tools)
	var geminiToolDefs struct {
		Tools []struct {
			FunctionDeclarations []struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"` // Keep as RawMessage
			} `json:"functionDeclarations"`
		} `json:"tools"`
	}
	// Use a new decoder for potentially different error reporting
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields() // Be stricter for the Gemini format attempt
	err = decoder.Decode(&geminiToolDefs)

	if err == nil && len(geminiToolDefs.Tools) > 0 {
		finalToolDefs = []ToolDefinition{} // Reset slice
		conversionErrs := []string{}       // Collect errors during conversion

		for _, tool := range geminiToolDefs.Tools {
			for _, funcDecl := range tool.FunctionDeclarations {
				var tempSchema tempParamSchema
				var protoSchema *generativelanguagepb.Schema
				var convertErr error

				// Unmarshal parameters if present and valid JSON
				if len(funcDecl.Parameters) > 0 && string(funcDecl.Parameters) != "null" {
					// Ensure the parameters field contains valid JSON
					if !json.Valid(funcDecl.Parameters) {
						conversionErrs = append(conversionErrs, fmt.Sprintf("tool '%s' (Gemini format): parameters field contains invalid JSON", funcDecl.Name))
						continue // Skip this function declaration
					}
					if err := json.Unmarshal(funcDecl.Parameters, &tempSchema); err != nil {
						conversionErrs = append(conversionErrs, fmt.Sprintf("tool '%s' (Gemini format): failed to unmarshal parameters JSON: %v", funcDecl.Name, err))
						continue // Skip this function declaration
					}

					// Convert the temporary schema to the protobuf schema
					protoSchema, convertErr = convertTempSchemaToProtoSchema(&tempSchema)
					if convertErr != nil {
						conversionErrs = append(conversionErrs, fmt.Sprintf("tool '%s' (Gemini format): failed to convert parameters schema: %v", funcDecl.Name, convertErr))
						continue // Skip this function declaration
					}
				}

				finalToolDefs = append(finalToolDefs, ToolDefinition{
					Name:        funcDecl.Name,
					Description: funcDecl.Description,
					Parameters:  protoSchema, // Could be nil if parameters were empty/null
				})
			}
		}
		// If we successfully converted at least one tool, return them.
		// Also return any non-fatal parsing/conversion errors encountered.
		if len(finalToolDefs) > 0 {
			var combinedErr error
			if len(conversionErrs) > 0 {
				combinedErr = fmt.Errorf("encountered errors during parameter processing: %s", strings.Join(conversionErrs, "; "))
			}
			return finalToolDefs, combinedErr // Return successfully converted tools and any errors
		}
		// If parsing succeeded but resulted in zero tools after filtering errors
		parseErrs = append(parseErrs, fmt.Sprintf("parsing as Gemini format succeeded but yielded no valid tools after parameter processing: %s", strings.Join(conversionErrs, "; ")))

	} else if err != nil {
		parseErrs = append(parseErrs, fmt.Sprintf("parsing as Gemini format failed: %v", err))
	} else {
		// It's valid JSON, but an empty list or not the expected structure
		parseErrs = append(parseErrs, "parsing as Gemini format resulted in empty list")
	}

	// If all attempts failed, return a combined error
	return nil, fmt.Errorf("failed to parse tool definitions in any recognized format: %s", strings.Join(parseErrs, "; "))
}

// RegisterTool registers a new tool with the given name, description, and handler.
func (tm *ToolManager) RegisterTool(name, description string, parameters json.RawMessage, handler func(json.RawMessage) (any, error)) error {
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}

	if description == "" {
		return fmt.Errorf("tool description cannot be empty")
	}

	// Process the parameters
	var protoSchema *generativelanguagepb.Schema
	if len(parameters) > 0 {
		// Handle JSON parameters
		// If parameters is a JSON string, it might already be a Schema
		// But more likely, it's a JSON schema in string format that needs conversion
		if json.Valid(parameters) {
			// Try to unmarshal directly into the temp schema for conversion
			var tempSchema tempParamSchema
			if err := json.Unmarshal(parameters, &tempSchema); err != nil {
				log.Printf("Warning: Failed to unmarshal parameters JSON for tool '%s': %v. Parameters might be incomplete.", name, err)
			} else {
				// Convert the temporary schema to the protobuf schema
				var err error
				protoSchema, err = convertTempSchemaToProtoSchema(&tempSchema)
				if err != nil {
					log.Printf("Warning: Failed to convert parameters schema for tool '%s': %v. Parameters might be incomplete.", name, err)
					protoSchema = nil // Proceed without parameters on conversion error
				}
			}
		} else {
			// Handle cases where parameters are not valid JSON
			log.Printf("Warning: Parameters for tool '%s' are not valid JSON. Parameters will be omitted.", name)
		}
	}

	// Ensure handler is not nil
	if handler == nil {
		return fmt.Errorf("handler cannot be nil for tool '%s'", name)
	}

	tm.RegisteredTools[name] = api.RegisteredTool{
		ToolDefinition: api.ToolDefinition{
			Name:        name,
			Description: description,
			Parameters:  protoSchema, // Assign the converted (or nil) schema
		},
		Handler:     handler,
		IsAvailable: true,
	}
	tm.RegisteredToolDefs = append(tm.RegisteredToolDefs, tm.RegisteredTools[name].ToolDefinition)
	return nil
}

// GetAvailableTools returns the definitions of all available tools
func (tm *ToolManager) GetAvailableTools() []api.ToolDefinition {
	var availableTools []api.ToolDefinition
	for _, tool := range tm.RegisteredTools {
		if tool.IsAvailable {
			availableTools = append(availableTools, tool.ToolDefinition)
		}
	}
	return availableTools
}

// processToolCalls processes tool calls from the model and returns the results
func (m *Model) processToolCalls(toolCalls []ToolCall) ([]ToolResult, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	if m.toolManager == nil {
		return nil, fmt.Errorf("tool manager not initialized")
	}

	// If tool approval is required, show the approval modal
	if m.requireApproval {
		// Store the tool calls for approval
		m.pendingToolCalls = toolCalls
		m.approvalIndex = 0
		m.showToolApproval = true

		// Return empty results for now - the tools will be processed
		// when the user approves them in the UI
		return nil, nil
	}

	// Process tools immediately if approval not required
	return m.executeToolCalls(toolCalls)
}

func mkErrorResponseStruct(err error) *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"error": err.Error(),
	})
	return s
}

// executeToolCalls actually executes the tool calls after they've been approved
func (m *Model) executeToolCalls(toolCalls []ToolCall) ([]ToolResult, error) {
	var results []ToolResult
	for _, call := range toolCalls {
		result := ToolResult{
			Id:   call.ID, // Use the correct field name 'Id'
			Name: call.Name,
		}
		registeredTool, exists := m.toolManager.RegisteredTools[call.Name]
		if !exists || !registeredTool.IsAvailable {
			result.Response = mkErrorResponseStruct(fmt.Errorf("tool '%s' not found or not available", call.Name))
			results = append(results, result)
			continue
		}

		// Add a system message to show the formatted tool call
		toolCallMessage := formatToolCall(call) // Use the new formatter
		m.messages = append(m.messages, formatMessage("System", toolCallMessage))
		m.viewport.SetContent(m.formatAllMessages())
		m.viewport.GotoBottom()

		// Process the tool call
		response, err := registeredTool.Handler(call.Arguments)
		resp := map[string]any{
			"result": response,
		}
		if err != nil {
			resp["error"] = err.Error()
		}
		result.Response, _ = structpb.NewStruct(resp)

		// Add a system message with the formatted result
		resultMessage := formatToolResult(result) // Use the new formatter
		m.messages = append(m.messages, formatMessage("System", resultMessage))
		m.viewport.SetContent(m.formatAllMessages())
		m.viewport.GotoBottom()

		results = append(results, result)
	}

	return results, nil
}

// formatToolCall creates a formatted string for displaying a tool call in the UI.
func formatToolCall(call ToolCall) string {
	var argsBuf bytes.Buffer
	// Pretty-print the JSON arguments for readability
	if err := json.Indent(&argsBuf, call.Arguments, "", "  "); err != nil {
		// Fallback to raw JSON if indentation fails
		argsBuf.Write(call.Arguments)
	}

	return fmt.Sprintf("🔧 Calling Tool: %s\nArguments:\n```json\n%s\n```",
		call.Name,
		argsBuf.String(),
	)
}

// formatToolResult creates a formatted string for displaying a tool result in the UI.
func formatToolResult(result ToolResult) string {
	var resultStr string

	if errVal, ok := result.Response.Fields["error"]; ok {
		resultStr = fmt.Sprintf("❌ Error: %s", errVal.GetStringValue())
	} else {
		// Pretty-print the JSON response
		responseBytes, err := json.MarshalIndent(result.Response, "", "  ")
		if err != nil {
			// Fallback if marshalling fails
			resultStr = fmt.Sprintf("✅ Result: %v (failed to marshal JSON: %v)", result.Response, err)
		} else {
			resultStr = fmt.Sprintf("✅ Result:\n```json\n%s\n```", string(responseBytes))
		}
	}
	return fmt.Sprintf("Tool Result (ID: %s)\n%s", result.Id, resultStr) // Use the correct field name 'Id'
}

// sendToolResultsCmd creates a command that sends tool results back to the model
func (m *Model) sendToolResultsCmd(results []ToolResult) tea.Cmd {
	return func() tea.Msg {
		if m.bidiStream == nil {
			log.Println("sendToolResultsCmd: Bidi stream is nil, cannot send tool results")
			return sendErrorMsg{err: fmt.Errorf("bidirectional stream not initialized")}
		}

		// Convert tool results to JSON for sending
		resultsJson, err := json.Marshal(results)
		if err != nil {
			return sendErrorMsg{err: fmt.Errorf("failed to marshal tool results: %w", err)}
		}

		log.Printf("Sending tool results to model: %s", string(resultsJson))

		var fnResps []*generativelanguagepb.FunctionResponse
		for _, result := range results {
			fnResps = append(fnResps, (*generativelanguagepb.FunctionResponse)(&result))
		}
		// Send to existing bidirectional stream using the API client
		err = m.client.SendToolResultsToBidiStream(m.bidiStream, fnResps...)
		if err != nil {
			return sendErrorMsg{err: fmt.Errorf("failed to send tool results: %w", err)}
		}

		return toolCallSentMsg{}
	}
}

// ExtractToolCalls extracts tool calls from the response
func ExtractToolCalls(resp *api.StreamOutput) []ToolCall {
	// Check if there's a FunctionCall in the response
	if resp.FunctionCall != nil {
		// Extract information from the API's function call
		functionCall := resp.FunctionCall

		// Generate a unique ID for the tool call if one is not provided
		id := functionCall.Id

		// Marshal the arguments to JSON for consistent handling
		argsJson, err := json.Marshal(functionCall.Args)
		if err != nil {
			log.Printf("Error marshaling function call arguments: %v", err)
			// Fallback to empty JSON object if marshaling fails
			argsJson = []byte("{}")
		}

		toolCall := ToolCall{
			ID:        id,
			Name:      functionCall.Name,
			Arguments: argsJson,
		}

		log.Printf("Extracted tool call from API: %s with arguments %s", toolCall.Name, string(toolCall.Arguments))
		return []ToolCall{toolCall}
	}

	// Fallback to looking for marker-based tool calls in the text
	// This is for backward compatibility with older implementations
	if strings.Contains(resp.Text, "[TOOL_CALL]") {
		toolCallParts := strings.Split(resp.Text, "[TOOL_CALL]")
		if len(toolCallParts) <= 1 {
			return nil
		}

		var toolCalls []ToolCall
		for i := 1; i < len(toolCallParts); i++ {
			parts := strings.SplitN(toolCallParts[i], "[/TOOL_CALL]", 2)
			if len(parts) < 1 {
				continue
			}

			toolCallJson := strings.TrimSpace(parts[0])
			var toolCall ToolCall
			if err := json.Unmarshal([]byte(toolCallJson), &toolCall); err != nil {
				log.Printf("Error unmarshaling tool call from text: %v", err)
				continue
			}
			toolCalls = append(toolCalls, toolCall)
		}

		if len(toolCalls) > 0 {
			log.Printf("Extracted %d tool calls from text markers", len(toolCalls))
			return toolCalls
		}
	}

	return nil
}

// ListAvailableTools returns a formatted string describing all available tools
func (tm *ToolManager) ListAvailableTools() string {
	var sb strings.Builder
	sb.WriteString("Available Tools:\n\n")

	for name, tool := range tm.RegisteredTools {
		if !tool.IsAvailable {
			continue
		}

		sb.WriteString(fmt.Sprintf("## %s\n", name))
		sb.WriteString(fmt.Sprintf("Description: %s\n", tool.ToolDefinition.Description))

		// Format parameters if available
		// Note: Checking Properties might be brittle if the schema structure varies.
		// A simpler check might be just `tool.ToolDefinition.Parameters != nil`.
		if tool.ToolDefinition.Parameters != nil {
			// Marshal the Schema to indented JSON using standard json
			bytes, err := json.MarshalIndent(tool.ToolDefinition.Parameters, "", "  ")
			if err != nil {
				log.Printf("Warning: Failed to marshal parameters for display (tool %s) using standard json: %v", name, err)
				sb.WriteString("Parameters: <error marshalling parameters>\n")
			} else if string(bytes) != "null" && string(bytes) != "{}" { // Avoid printing empty/null schemas
				sb.WriteString("Parameters:\n```json\n")
				sb.WriteString(string(bytes))
				sb.WriteString("\n```\n")
			} else {
				sb.WriteString("Parameters: (none)\n") // Explicitly state none if schema is empty/null
			}
		} else {
			sb.WriteString("Parameters: (none)\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ToolDefinition represents a tool definition, compatible with aistudio custom format,
// FileToolDefinition represents the structure expected in the JSON tool definition file.
// This is kept separate from api.ToolDefinition to handle the raw JSON parameters during loading.
type FileToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`        // Read as raw JSON first
	Handler     string          `json:"handler"`           // Custom field for defining handler type
	Command     string          `json:"command,omitempty"` // Custom field used by specific handlers
}

// LoadToolsFromFile loads tool definitions from a JSON file
func LoadToolsFromFile(filePath string, tm *ToolManager) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open tools file '%s': %w", filePath, err)
	}
	defer file.Close()

	// Read the raw file content
	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read tools file '%s': %w", filePath, err)
	}

	// Attempt to parse as a list of FileToolDefinition
	var fileToolDefs []FileToolDefinition
	if err := json.Unmarshal(data, &fileToolDefs); err != nil {
		// Add more sophisticated parsing logic here if needed (e.g., Gemini format)
		return fmt.Errorf("failed to parse tool definitions from '%s' as JSON array: %w", filePath, err)
	}

	for _, def := range fileToolDefs {
		if def.Handler == "" {
			def.Handler = "custom" // Default to custom handler if not specified
		}
		if def.Command == "" {
			def.Command = def.Name // Default command to the tool name if not specified
		}
		// Ensure Parameters is valid JSON before proceeding
		if len(def.Parameters) > 0 && !json.Valid(def.Parameters) {
			log.Printf("Warning: Skipping tool '%s' due to invalid JSON in parameters: %s", def.Name, string(def.Parameters))
			time.Sleep(1 * time.Second)
			continue
		}

		// Create the handler based on the FileToolDefinition
		handler, err := createHandlerForFileDefinition(def)
		if err != nil {
			log.Printf("Warning: Skipping tool '%s': %v", def.Name, err)
			continue
		}

		// Register the tool using the parsed definition and handler
		// Pass def.Parameters (json.RawMessage) directly; RegisterTool handles conversion.
		err = tm.RegisterTool(
			def.Name,
			def.Description,
			def.Parameters, // Pass as json.RawMessage
			handler,
		)
		if err != nil {
			log.Printf("Warning: Failed to register tool '%s': %v", def.Name, err)
			time.Sleep(1 * time.Second)
			continue
		}
		log.Printf("Registered tool from file: %s", def.Name)
	}

	return nil
}

// ExecuteCommandTool executes a shell command and returns the result
func ExecuteCommandTool(command string, args []string, timeout time.Duration) (string, error) {
	ctx := context.Background()
	var cancel context.CancelFunc

	// TODO: configurable prefix?
	command = fmt.Sprintf("aistudio-tool-%s", command)

	// Check if the command exists in PATH
	execPath, found := findExecutableInPath(command)
	if !found {
		return "", fmt.Errorf("command '%s' not found in PATH", command)
	}
	// Use the full path to the executable
	command = execPath

	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, command, args...)
	// set up very restricted environment:
	path := os.Getenv("PATH")
	cmd.Env = []string{
		"PATH=" + path,
		"HOME=/tmp",
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("command execution failed: %v\nStderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// findExecutableInPath checks if an executable exists in the PATH
func findExecutableInPath(executable string) (string, bool) {
	// Try to find the executable in PATH
	path, err := exec.LookPath(executable)
	if err == nil {
		return path, true
	}

	// Check if a tool-specific executable exists (aistudio-tool-{name})
	toolSpecificName := "aistudio-tool-" + executable
	path, err = exec.LookPath(toolSpecificName)
	if err == nil {
		return path, true
	}

	return "", false
}

// createHandlerForFileDefinition creates a handler function based on a FileToolDefinition
func createHandlerForFileDefinition(def FileToolDefinition) (func(json.RawMessage) (any, error), error) {
	switch def.Handler {
	case "system_info":
		// This handler is defined inline for simplicity, matching RegisterDefaultTools
		return func(args json.RawMessage) (any, error) {
			return nil, fmt.Errorf("system_info handler is not implemented yet")
			var params struct {
				IncludeTime bool `json:"include_time"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			result := map[string]any{
				"hostname": "localhost",
			}

			if params.IncludeTime {
				result["current_time"] = time.Now().Format(time.RFC3339)
			}

			return result, nil
		}, nil

	case "exec_command":
		// This handler is defined inline for simplicity, matching RegisterDefaultTools
		return func(args json.RawMessage) (any, error) {
			return nil, fmt.Errorf("exec_command handler is not implemented yet")
			var params struct {
				Command   string   `json:"command"`
				Args      []string `json:"args,omitempty"`
				TimeoutMs int      `json:"timeout_ms,omitempty"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			// If a command is specified in the definition, override the command from args
			if def.Command != "" {
				params.Command = def.Command
			}

			// Security check - restrict dangerous commands
			dangerousCmds := map[string]bool{
				"rm": true, "mv": true, "cp": true, "dd": true,
				"mkfs": true, "reboot": true, "shutdown": true,
				"wget": true, "curl": true, "chmod": true,
			}

			if dangerousCmds[params.Command] {
				return nil, fmt.Errorf("command '%s' is not allowed for security reasons", params.Command)
			}

			// Verify the command exists in PATH
			execPath, found := findExecutableInPath(params.Command)
			if !found {
				log.Printf("Warning: Command '%s' not found in PATH", params.Command)
				return map[string]any{
					"success": false,
					"error":   fmt.Sprintf("Command '%s' not found. Make sure it's installed and in your PATH.", params.Command),
				}, nil
			}

			// Use the full path to the executable
			log.Printf("Using executable at: %s", execPath)
			params.Command = execPath

			// Set a default timeout if none provided
			timeout := time.Duration(params.TimeoutMs) * time.Millisecond
			if timeout == 0 {
				timeout = 5 * time.Second // Default timeout
			}

			// Execute the command
			output, err := ExecuteCommandTool(params.Command, params.Args, timeout)
			if err != nil {
				return map[string]any{
					"success": false,
					"error":   err.Error(),
				}, nil
			}

			return map[string]any{
				"success": true,
				"output":  output,
			}, nil
		}, nil

	case "file_operations":
		// This handler provides file operations
		return func(args json.RawMessage) (any, error) {
			return nil, fmt.Errorf("file_operations handler is not implemented yet")
			var params struct {
				Operation string `json:"operation"`
				Path      string `json:"path"`
				Content   string `json:"content,omitempty"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			// Get the absolute path for security
			absPath, err := filepath.Abs(params.Path)
			if err != nil {
				return nil, fmt.Errorf("invalid path: %w", err)
			}

			switch params.Operation {
			case "read":
				data, err := os.ReadFile(absPath)
				if err != nil {
					return map[string]any{
						"success": false,
						"error":   err.Error(),
					}, nil
				}
				return map[string]any{
					"success": true,
					"content": string(data),
				}, nil

			case "write":
				err := os.WriteFile(absPath, []byte(params.Content), 0644)
				if err != nil {
					return map[string]any{
						"success": false,
						"error":   err.Error(),
					}, nil
				}
				return map[string]any{
					"success": true,
					"path":    absPath,
				}, nil

			case "exists":
				_, err := os.Stat(absPath)
				exists := !os.IsNotExist(err)
				return map[string]any{
					"exists": exists,
					"path":   absPath,
				}, nil

			default:
				return nil, fmt.Errorf("unknown file operation: %s", params.Operation)
			}
		}, nil

	case "read_file":
		// This handler is defined inline for simplicity, matching RegisterDefaultTools
		return func(args json.RawMessage) (any, error) {
			return nil, fmt.Errorf("read_file handler is not implemented yet")
			var params struct {
				FilePath string `json:"file_path"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			// Get the absolute path for security
			absPath, err := filepath.Abs(params.FilePath)
			if err != nil {
				return nil, fmt.Errorf("invalid path: %w", err)
			}

			data, err := os.ReadFile(absPath)
			if err != nil {
				return map[string]any{
					"success": false,
					"error":   err.Error(),
				}, nil
			}
			return map[string]any{
				"success": true,
				"content": string(data),
			}, nil
		}, nil

	case "custom":
		// Custom handler - delegates to a specific command
		return func(args json.RawMessage) (any, error) {
			// Set a default timeout
			timeout := 5 * time.Second

			// Execute the command with args passed as JSON
			cmdArgs := []string{string(args)}
			return ExecuteCommandTool(def.Command, cmdArgs, timeout)
		}, nil

	default:
		return nil, fmt.Errorf("unknown handler type: %s", def.Handler)
	}
}

// RegisterDefaultTools registers a set of default tools with the tool manager
func (tm *ToolManager) RegisterDefaultTools() error {
	// System Info tool
	err := tm.RegisterTool(
		"system_info",
		"Get information about the system",
		json.RawMessage(`{
			"type": "object",
			"properties": {
				"include_time": {
					"type": "boolean",
					"description": "Whether to include the current time in the response"
				}
			}
		}`),
		func(args json.RawMessage) (any, error) {
			var params struct {
				IncludeTime bool `json:"include_time"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			result := map[string]any{
				"hostname": "localhost",
			}

			if params.IncludeTime {
				result["current_time"] = time.Now().Format(time.RFC3339)
			}

			return result, nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed to register system_info tool: %w", err)
	}

	// Command Execution tool
	err = tm.RegisterTool(
		"exec_command",
		"Execute a shell command",
		json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {
					"type": "string",
					"description": "The command to execute"
				},
				"args": {
					"type": "array",
					"description": "Arguments to pass to the command",
					"items": {
						"type": "string"
					}
				},
				"timeout_ms": {
					"type": "integer",
					"description": "Timeout in milliseconds (default: 5000)"
				}
			},
			"required": ["command"]
		}`),
		func(args json.RawMessage) (any, error) {
			var params struct {
				Command   string   `json:"command"`
				Args      []string `json:"args,omitempty"`
				TimeoutMs int      `json:"timeout_ms,omitempty"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			// Security check - restrict dangerous commands
			dangerousCmds := map[string]bool{
				"rm": true, "mv": true, "cp": true, "dd": true,
				"mkfs": true, "reboot": true, "shutdown": true,
				"wget": true, "curl": true, "chmod": true,
			}

			if dangerousCmds[params.Command] {
				return nil, fmt.Errorf("command '%s' is not allowed for security reasons", params.Command)
			}

			// Verify the command exists in PATH
			execPath, found := findExecutableInPath(params.Command)
			if !found {
				log.Printf("Warning: Command '%s' not found in PATH", params.Command)
				return map[string]any{
					"success": false,
					"error":   fmt.Sprintf("Command '%s' not found. Make sure it's installed and in your PATH.", params.Command),
				}, nil
			}

			// Use the full path to the executable
			log.Printf("Using executable at: %s", execPath)
			params.Command = execPath

			// Set a default timeout if none provided
			timeout := time.Duration(params.TimeoutMs) * time.Millisecond
			if timeout == 0 {
				timeout = 5 * time.Second // Default timeout
			}

			// Execute the command
			output, err := ExecuteCommandTool(params.Command, params.Args, timeout)
			if err != nil {
				return map[string]any{
					"success": false,
					"error":   err.Error(),
				}, nil
			}

			return map[string]any{
				"success": true,
				"output":  output,
			}, nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed to register exec_command tool: %w", err)
	}

	return nil
}
