package aistudio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
)

// MessageFormatter contains functions for formatting messages
type MessageFormatter struct{}

// NewMessageFormatter creates a new message formatter
func NewMessageFormatter() *MessageFormatter {
	return &MessageFormatter{}
}

// FormatMessage creates a Message from sender and content
func (f *MessageFormatter) FormatMessage(sender senderName, content string) Message {
	return Message{
		ID:        uuid.New().String(), // Generate a unique UUID for each message
		Sender:    sender,
		Content:   content,
		HasAudio:  false, // Default, can be updated later
		Timestamp: time.Now(),
	}
}

// FormatError creates an error Message
func (f *MessageFormatter) FormatError(err error) Message {
	return Message{
		ID:        uuid.New().String(), // Generate a unique UUID for each message
		Sender:    "System",
		Content:   fmt.Sprintf("Error: %v", err),
		HasAudio:  false,
		Timestamp: time.Now(),
	}
}

// ToolFormatter handles formatting of tool calls and responses
type ToolFormatter struct{}

// NewToolFormatter creates a new tool formatter
func NewToolFormatter() *ToolFormatter {
	return &ToolFormatter{}
}

// FormatToolCallHeader creates a header for a tool call with icon, name, ID and status
func (f *ToolFormatter) FormatToolCallHeader(vm ToolCallViewModel) string {
	var header strings.Builder

	header.WriteString(toolCallHeaderStyle.Render(fmt.Sprintf("🔧 Tool Call: %s", vm.Name)))
	header.WriteString(" ")
	header.WriteString(toolIdStyle.Render(fmt.Sprintf("(ID: %s)", vm.ID)))

	// Add status indicator
	header.WriteString(" ")
	statusGlyph := toolStatusGlyph(vm.Status)

	switch vm.Status {
	case ToolCallStatusRunning:
		header.WriteString(toolRunningStyle.Render(fmt.Sprintf("%s Running...", statusGlyph)))
	case ToolCallStatusPending:
		header.WriteString(toolRunningStyle.Render(fmt.Sprintf("%s Pending...", statusGlyph)))
	case ToolCallStatusCompleted:
		if vm.Error != nil {
			header.WriteString(toolErrorStyle.Render(fmt.Sprintf("%s Error", statusGlyph)))
		} else {
			header.WriteString(toolStatusStyle.Render(fmt.Sprintf("%s Completed", statusGlyph)))
		}
	case ToolCallStatusRejected:
		header.WriteString(toolErrorStyle.Render(fmt.Sprintf("%s Rejected", statusGlyph)))
	}

	return header.String()
}

// FormatToolArgs formats tool arguments as pretty JSON with width adaptation
func (f *ToolFormatter) FormatToolArgs(vm ToolCallViewModel, availWidth ...int) string {
	if len(vm.Arguments) == 0 {
		return "Arguments: (none)"
	}

	var args bytes.Buffer
	if err := json.Indent(&args, vm.Arguments, "", "  "); err != nil {
		// Fallback if indent fails
		return fmt.Sprintf("Arguments: %s", string(vm.Arguments))
	}

	// Format as JSON codeblock
	jsonContent := args.String()

	// If width is provided, wrap long lines
	if len(availWidth) > 0 && availWidth[0] > 20 {
		// Leave some room for the code block markers and padding
		effectiveWidth := availWidth[0] - 6
		// Wrap the content if it's too wide
		jsonContent = lipgloss.NewStyle().MaxWidth(effectiveWidth).Render(jsonContent)
	}

	return fmt.Sprintf("```json\n%s\n```", jsonContent)
}

// FormatToolResult formats tool results as pretty JSON with width adaptation
func (f *ToolFormatter) FormatToolResult(vm ToolCallViewModel, availWidth ...int) string {
	if vm.Error != nil {
		return fmt.Sprintf("Error: %s", vm.Error.Error())
	}

	if vm.Result == nil || len(vm.Result.Fields) == 0 {
		return "Result: (empty)"
	}

	jsonBytes, err := protojson.Marshal(vm.Result)
	if err != nil {
		return fmt.Sprintf("Error marshaling result: %v", err)
	}

	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, jsonBytes, "", "  "); err != nil {
		return string(jsonBytes)
	}

	// Always strip ANSI codes from JSON output
	cleanJSON := StripANSI(prettyJSON.String())

	// If width is provided, wrap long lines
	if len(availWidth) > 0 && availWidth[0] > 20 {
		// Leave some room for the code block markers and padding
		effectiveWidth := availWidth[0] - 6
		// Wrap the content if it's too wide
		cleanJSON = lipgloss.NewStyle().MaxWidth(effectiveWidth).Render(cleanJSON)
	}

	return fmt.Sprintf("```json\n%s\n```", cleanJSON)
}

// CreateToolCallMessage creates a Message for a tool call with enhanced formatting
func (f *ToolFormatter) CreateToolCallMessage(vm ToolCallViewModel, availWidth ...int) Message {
	var content strings.Builder

	// Add the header
	content.WriteString(f.FormatToolCallHeader(vm))
	content.WriteString("\n")

	// Add the arguments, passing available width if provided
	content.WriteString(f.FormatToolArgs(vm, availWidth...))
	content.WriteString("\n")

	toolCall := ToolCall{
		ID:        vm.ID,
		Name:      vm.Name,
		Arguments: vm.Arguments,
	}

	return Message{
		ID:        uuid.New().String(), // Generate a unique UUID for each message
		Sender:    "System",
		Content:   content.String(),
		ToolCall:  &toolCall,
		Timestamp: time.Now(),
	}
}

// CreateToolResultMessage creates a Message for a tool result with enhanced formatting
func (f *ToolFormatter) CreateToolResultMessage(vm ToolCallViewModel, availWidth ...int) Message {
	var content strings.Builder
	toolResultStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)

	// Only render results for completed tools
	if vm.Status == ToolCallStatusCompleted {
		content.WriteString(toolResultStyle.Render(fmt.Sprintf("✅ Tool Result: %s", vm.Name)))
		content.WriteString(" ")
		content.WriteString(toolIdStyle.Render(fmt.Sprintf("(ID: %s)", vm.ID)))
		content.WriteString("\n")

		// Add the formatted result, passing available width if provided
		content.WriteString(f.FormatToolResult(vm, availWidth...))
		content.WriteString("\n")
	}

	toolResponse := &ToolResponse{
		Id:       vm.ID,
		Name:     vm.Name,
		Response: vm.Result,
	}

	return Message{
		ID:           uuid.New().String(), // Generate a unique UUID for each message
		Sender:       "System",
		Content:      content.String(),
		ToolResponse: toolResponse,
		ToolCall:     &ToolCall{ID: vm.ID, Name: vm.Name},
		Timestamp:    time.Now(),
	}
}

// APIFormatter handles API response conversion to display formats
type APIFormatter struct{}

// NewAPIFormatter creates a new API formatter
func NewAPIFormatter() *APIFormatter {
	return &APIFormatter{}
}

// ConvertSafetyRating converts API safety rating to our display format
func (f *APIFormatter) ConvertSafetyRating(apiRating *generativelanguagepb.SafetyRating) *SafetyRating {
	if apiRating == nil {
		return nil
	}

	// Convert category to a simplified string representation
	category := fmt.Sprintf("%s", apiRating.Category)
	// Remove the prefix if present
	category = strings.TrimPrefix(category, "HARM_CATEGORY_")

	// Convert probability to string representation
	probability := fmt.Sprintf("%s", apiRating.Probability)

	// Check if this rating is blocking content - high and medium are generally considered blocking
	blocked := apiRating.Probability == generativelanguagepb.SafetyRating_HIGH ||
		apiRating.Probability == generativelanguagepb.SafetyRating_MEDIUM

	return &SafetyRating{
		Category:    category,
		Probability: probability,
		Score:       0, // API doesn't provide probability score directly
		Blocked:     blocked,
	}
}

// ConvertGroundingMetadata converts API grounding metadata to our display format
func (f *APIFormatter) ConvertGroundingMetadata(apiMetadata *generativelanguagepb.GroundingMetadata) *GroundingMetadata {
	if apiMetadata == nil {
		return nil
	}

	result := &GroundingMetadata{
		Chunks:              make([]*GroundingChunk, 0),
		Supports:            make([]*GroundingSupport, 0),
		WebSearchQueries:    make([]string, 0),
		HasSearchEntryPoint: false,
	}

	// Convert grounding chunks
	for i, chunk := range apiMetadata.GetGroundingChunks() {
		displayChunk := &GroundingChunk{
			ID:       fmt.Sprintf("chunk-%d", i),
			Selected: false,
		}

		// Handle web chunk
		if web := chunk.GetWeb(); web != nil {
			displayChunk.IsWeb = true
			displayChunk.URI = web.GetUri()
			displayChunk.Title = web.GetTitle()
		}

		result.Chunks = append(result.Chunks, displayChunk)
	}

	// Convert web search queries
	result.WebSearchQueries = apiMetadata.GetWebSearchQueries()

	// Handle search entry point
	if entryPoint := apiMetadata.GetSearchEntryPoint(); entryPoint != nil {
		result.HasSearchEntryPoint = true
		result.SearchEntryPoint = &SearchEntryPoint{
			RenderedContent: entryPoint.GetRenderedContent(),
			HasSDKBlob:      len(entryPoint.GetSdkBlob()) > 0,
		}
	}

	// Handle support information
	for _, support := range apiMetadata.GetGroundingSupports() {
		// Convert []int32 to []int for ChunkIndices
		int32Indices := support.GetGroundingChunkIndices()
		intIndices := make([]int, len(int32Indices))
		for i, idx := range int32Indices {
			intIndices[i] = int(idx)
		}

		displaySupport := &GroundingSupport{
			Text:         "", // Will be extracted from segment information
			ChunkIndices: intIndices,
			Confidence:   support.GetConfidenceScores(),
		}

		// Extract text from segment if available
		if seg := support.GetSegment(); seg != nil {
			displaySupport.Text = seg.GetText()
		}

		// Associate chunks with this support
		displaySupport.ChunksSelected = make([]*GroundingChunk, 0)
		for _, idx := range displaySupport.ChunkIndices {
			if idx >= 0 && int(idx) < len(result.Chunks) {
				// Mark the chunk as selected
				result.Chunks[idx].Selected = true
				displaySupport.ChunksSelected = append(displaySupport.ChunksSelected, result.Chunks[idx])
			}
		}

		result.Supports = append(result.Supports, displaySupport)
	}

	return result
}
