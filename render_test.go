package aistudio

import (
	"strings"
	"testing"
	"time"
)

func TestNewMessageRenderer(t *testing.T) {
	model := &Model{}
	renderer := NewMessageRenderer(model)

	if renderer == nil {
		t.Error("NewMessageRenderer() returned nil")
	}

	if renderer.model != model {
		t.Error("NewMessageRenderer() didn't set model correctly")
	}
}

func TestRenderMessages(t *testing.T) {
	// Create a test model with some messages
	model := &Model{
		width: 80,
		messages: []Message{
			{
				Sender:    "You",
				Content:   "Hello",
				Timestamp: time.Now(),
			},
			{
				Sender:    "Gemini",
				Content:   "Hi there!",
				Timestamp: time.Now().Add(1 * time.Second),
			},
		},
	}

	renderer := NewMessageRenderer(model)
	output := renderer.RenderMessages()

	// Basic checks
	if output == "" {
		t.Error("RenderMessages() returned empty string")
	}

	if !strings.Contains(output, "Hello") {
		t.Error("RenderMessages() doesn't contain user message content")
	}

	if !strings.Contains(output, "Hi there!") {
		t.Error("RenderMessages() doesn't contain model message content")
	}
}

func TestFormatMessageHeader(t *testing.T) {
	model := &Model{}
	renderer := NewMessageRenderer(model)

	// Test different sender types
	testCases := []struct {
		msg           Message
		expectedParts []string
	}{
		{
			msg: Message{
				Sender: senderNameUser,
				ID:     "user123456789",
			},
			expectedParts: []string{"You", "Text"},
		},
		{
			msg: Message{
				Sender: senderNameModel,
				ID:     "model123",
			},
			expectedParts: []string{"Gemini", "Text"},
		},
		{
			msg: Message{
				Sender:   "System",
				ID:       "sys123",
				HasAudio: true,
			},
			expectedParts: []string{"System", "Audio"},
		},
		{
			msg: Message{
				Sender:           "System",
				IsExecutableCode: true,
			},
			expectedParts: []string{"System", "Code"},
		},
	}

	for i, tc := range testCases {
		header := renderer.formatMessageHeader(tc.msg, i)

		for _, part := range tc.expectedParts {
			if !strings.Contains(stripStyles(header), part) {
				t.Errorf("formatMessageHeader(%v, %d) doesn't contain %q; got: %q",
					tc.msg.Sender, i, part, stripStyles(header))
			}
		}

		// ID should be included if provided (truncated to 8 chars for long IDs)
		if tc.msg.ID != "" {
			idToCheck := tc.msg.ID
			if len(idToCheck) > 8 {
				idToCheck = idToCheck[:8]
			}
			if !strings.Contains(stripStyles(header), idToCheck) {
				t.Errorf("formatMessageHeader(%v, %d) doesn't contain ID %q; got: %q",
					tc.msg.Sender, i, idToCheck, stripStyles(header))
			}
		}
	}
}

func TestFormatMessageText(t *testing.T) {
	model := &Model{}
	renderer := NewMessageRenderer(model)

	// Test different message types
	testCases := []struct {
		name          string
		msg           Message
		index         int
		expectedParts []string
	}{
		{
			name: "Empty message",
			msg: Message{
				Sender:  "System",
				Content: "",
			},
			index:         0,
			expectedParts: []string{}, // Should be empty
		},
		{
			name: "Basic text message",
			msg: Message{
				Sender:  "You",
				Content: "Hello world",
			},
			index:         1,
			expectedParts: []string{"You", "Text", "Hello world"},
		},
		{
			name: "Audio message",
			msg: Message{
				Sender:    "Gemini",
				Content:   "Message with audio",
				HasAudio:  true,
				AudioData: []byte("audio data"),
			},
			index:         2,
			expectedParts: []string{"Gemini", "Audio", "Message with audio", "🔊"},
		},
		{
			name: "Message with token counts",
			msg: Message{
				Sender:  "Gemini",
				Content: "Response with tokens",
				TokenCounts: &TokenCounts{
					PromptTokenCount:   10,
					ResponseTokenCount: 20,
				},
			},
			index:         3,
			expectedParts: []string{"Gemini", "Response with tokens", "10", "20"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			formatted := renderer.formatMessageText(tc.msg, tc.index)
			plainText := stripStyles(formatted)

			// Skip empty message test
			if len(tc.expectedParts) == 0 {
				if formatted != "" {
					t.Errorf("Expected empty result for empty message, got: %q", formatted)
				}
				return
			}

			// Check that each expected part is in the output
			for _, part := range tc.expectedParts {
				if !strings.Contains(plainText, part) {
					t.Errorf("formatMessageText() output doesn't contain %q; got: %q",
						part, plainText)
				}
			}
		})
	}
}

func TestGroupMessages(t *testing.T) {
	// Create a model with messages including tool calls and responses
	model := &Model{
		messages: []Message{
			{
				Sender:  "You",
				Content: "Hello",
			},
			{
				Sender:  "Gemini",
				Content: "Hi there!",
			},
			{
				Sender: "Gemini",
				ToolCall: &ToolCall{
					ID:   "tool1",
					Name: "TestTool",
				},
			},
			{
				Sender: "System",
				ToolResponse: &ToolResponse{
					Id:   "tool1",
					Name: "TestTool",
				},
			},
			{
				Sender:  "You",
				Content: "Another message",
			},
		},
	}

	renderer := NewMessageRenderer(model)
	groups := renderer.groupMessages()

	// We should have 4 groups: user, gemini, tool call+response as one group, user
	if len(groups) != 4 {
		t.Errorf("groupMessages() returned %d groups, expected 4", len(groups))
	}

	// Check that the tool call and response are grouped together
	foundToolGroup := false
	for _, group := range groups {
		if len(group) == 2 && group[0].IsToolCall() && group[1].IsToolResponse() {
			foundToolGroup = true
			if group[0].ToolCall.ID != group[1].ToolResponse.Id {
				t.Errorf("Tool call ID %q doesn't match response ID %q",
					group[0].ToolCall.ID, group[1].ToolResponse.Id)
			}
		}
	}

	if !foundToolGroup {
		t.Error("groupMessages() didn't group tool call with its response")
	}
}

func TestFindToolResponse(t *testing.T) {
	// Create a model with a tool call and matching response
	model := &Model{
		messages: []Message{
			{
				Sender:  "You",
				Content: "Hello",
			},
			{
				Sender: "Gemini",
				ToolCall: &ToolCall{
					ID:   "tool1",
					Name: "TestTool",
				},
			},
			{
				Sender:  "System",
				Content: "Some other message",
			},
			{
				Sender: "System",
				ToolResponse: &ToolResponse{
					Id:   "tool1",
					Name: "TestTool",
				},
			},
		},
	}

	renderer := NewMessageRenderer(model)

	// Test finding the response
	toolCallMsg := model.messages[1]
	response := renderer.findToolResponse(toolCallMsg, 2)

	if response == nil {
		t.Error("findToolResponse() returned nil, expected to find a response")
	} else if response.ToolResponse.Id != toolCallMsg.ToolCall.ID {
		t.Errorf("Found response ID %q doesn't match tool call ID %q",
			response.ToolResponse.Id, toolCallMsg.ToolCall.ID)
	}

	// Test with non-tool call message
	nonToolMsg := model.messages[0]
	response = renderer.findToolResponse(nonToolMsg, 1)
	if response != nil {
		t.Error("findToolResponse() for non-tool call message should return nil")
	}

	// Test with tool call that has no response
	noResponseToolMsg := Message{
		Sender: "Gemini",
		ToolCall: &ToolCall{
			ID:   "tool2",
			Name: "NoResponseTool",
		},
	}
	response = renderer.findToolResponse(noResponseToolMsg, 0)
	if response != nil {
		t.Error("findToolResponse() for tool call with no response should return nil")
	}
}

// Helper function to strip ANSI styles for testing content
func stripStyles(s string) string {
	return StripANSI(s)
}
