package api

import (
	"testing"

	"cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
)

func TestTextContent(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		role     string
		wantRole string
	}{
		{
			name:     "basic text",
			text:     "Hello world",
			wantRole: "",
		},
		{
			name:     "with user role",
			text:     "Hello world",
			role:     "user",
			wantRole: "user",
		},
		{
			name:     "with model role",
			text:     "Hello world",
			role:     "model",
			wantRole: "model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []textContentOption
			if tt.role != "" {
				opts = append(opts, withRole(tt.role))
			}

			content := textContent(tt.text, opts...)

			// Check content creation
			if content == nil {
				t.Fatal("Expected content to be created, got nil")
			}

			// Check role
			if content.Role != tt.wantRole {
				t.Errorf("Expected role %q, got %q", tt.wantRole, content.Role)
			}

			// Check parts
			if len(content.Parts) != 1 {
				t.Errorf("Expected 1 part, got %d", len(content.Parts))
			}

			// Check text
			textPart := content.Parts[0].GetText()
			if textPart != tt.text {
				t.Errorf("Expected text %q, got %q", tt.text, textPart)
			}
		})
	}
}

func TestAlphaTextContent(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		role     string
		wantRole string
	}{
		{
			name:     "basic text",
			text:     "Hello world",
			wantRole: "",
		},
		{
			name:     "with user role",
			text:     "Hello world",
			role:     "user",
			wantRole: "user",
		},
		{
			name:     "with model role",
			text:     "Hello world",
			role:     "model",
			wantRole: "model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []alphaTextContentOption
			if tt.role != "" {
				opts = append(opts, withAlphaRole(tt.role))
			}

			content := alphaTextContent(tt.text, opts...)

			// Check content creation
			if content == nil {
				t.Fatal("Expected content to be created, got nil")
			}

			// Check role
			if content.Role != tt.wantRole {
				t.Errorf("Expected role %q, got %q", tt.wantRole, content.Role)
			}

			// Check parts
			if len(content.Parts) != 1 {
				t.Errorf("Expected 1 part, got %d", len(content.Parts))
			}

			// Check text
			textPart := content.Parts[0].GetText()
			if textPart != tt.text {
				t.Errorf("Expected text %q, got %q", tt.text, textPart)
			}
		})
	}
}

func TestExtractOutput(t *testing.T) {
	tests := []struct {
		name           string
		response       *generativelanguagepb.GenerateContentResponse
		expectedText   string
		expectedAudio  bool
		expectedTokens int32
	}{
		{
			name:          "nil response",
			response:      nil,
			expectedText:  "",
			expectedAudio: false,
		},
		{
			name: "text only response",
			response: &generativelanguagepb.GenerateContentResponse{
				Candidates: []*generativelanguagepb.Candidate{
					{
						Content: &generativelanguagepb.Content{
							Parts: []*generativelanguagepb.Part{
								{
									Data: &generativelanguagepb.Part_Text{
										Text: "Hello world",
									},
								},
							},
						},
					},
				},
			},
			expectedText:  "Hello world",
			expectedAudio: false,
		},
		{
			name: "multiple text parts",
			response: &generativelanguagepb.GenerateContentResponse{
				Candidates: []*generativelanguagepb.Candidate{
					{
						Content: &generativelanguagepb.Content{
							Parts: []*generativelanguagepb.Part{
								{
									Data: &generativelanguagepb.Part_Text{
										Text: "Hello ",
									},
								},
								{
									Data: &generativelanguagepb.Part_Text{
										Text: "world",
									},
								},
							},
						},
					},
				},
			},
			expectedText:  "Hello world",
			expectedAudio: false,
		},
		{
			name: "with audio",
			response: &generativelanguagepb.GenerateContentResponse{
				Candidates: []*generativelanguagepb.Candidate{
					{
						Content: &generativelanguagepb.Content{
							Parts: []*generativelanguagepb.Part{
								{
									Data: &generativelanguagepb.Part_Text{
										Text: "Hello world",
									},
								},
								{
									Data: &generativelanguagepb.Part_InlineData{
										InlineData: &generativelanguagepb.Blob{
											MimeType: "audio/wav",
											Data:     []byte{1, 2, 3, 4},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedText:  "Hello world",
			expectedAudio: true,
		},
		{
			name: "with feedback",
			response: &generativelanguagepb.GenerateContentResponse{
				Candidates: []*generativelanguagepb.Candidate{
					{
						Content: &generativelanguagepb.Content{
							Parts: []*generativelanguagepb.Part{
								{
									Data: &generativelanguagepb.Part_Text{
										Text: "Hello world",
									},
								},
							},
						},
					},
				},
				PromptFeedback: &generativelanguagepb.GenerateContentResponse_PromptFeedback{
					BlockReason: generativelanguagepb.GenerateContentResponse_PromptFeedback_SAFETY,
					SafetyRatings: []*generativelanguagepb.SafetyRating{
						{
							Category:    generativelanguagepb.HarmCategory_HARM_CATEGORY_DANGEROUS_CONTENT,
							Probability: generativelanguagepb.SafetyRating_HIGH,
						},
					},
				},
			},
			expectedText:  "Hello world [Blocked: SAFETY] [Safety: HARM_CATEGORY_DANGEROUS_CONTENT - HIGH]",
			expectedAudio: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := ExtractOutput(tt.response)

			// Check text
			if output.Text != tt.expectedText {
				t.Errorf("Expected text %q, got %q", tt.expectedText, output.Text)
			}

			// Check audio
			if (len(output.Audio) > 0) != tt.expectedAudio {
				t.Errorf("Expected audio presence: %v, got: %v", tt.expectedAudio, len(output.Audio) > 0)
			}
		})
	}
}

func TestProcessFeedback(t *testing.T) {
	tests := []struct {
		name           string
		feedback       *generativelanguagepb.GenerateContentResponse_PromptFeedback
		expectedOutput string
	}{
		{
			name:           "nil feedback",
			feedback:       nil,
			expectedOutput: "",
		},
		{
			name: "blocked for safety",
			feedback: &generativelanguagepb.GenerateContentResponse_PromptFeedback{
				BlockReason: generativelanguagepb.GenerateContentResponse_PromptFeedback_SAFETY,
			},
			expectedOutput: "[Blocked: SAFETY]",
		},
		{
			name: "safety ratings",
			feedback: &generativelanguagepb.GenerateContentResponse_PromptFeedback{
				SafetyRatings: []*generativelanguagepb.SafetyRating{
					{
						Category:    generativelanguagepb.HarmCategory_HARM_CATEGORY_HATE_SPEECH,
						Probability: generativelanguagepb.SafetyRating_MEDIUM,
					},
				},
			},
			expectedOutput: "[Safety: HARM_CATEGORY_HATE_SPEECH - MEDIUM]",
		},
		{
			name: "negligible ratings not included",
			feedback: &generativelanguagepb.GenerateContentResponse_PromptFeedback{
				SafetyRatings: []*generativelanguagepb.SafetyRating{
					{
						Category:    generativelanguagepb.HarmCategory_HARM_CATEGORY_HATE_SPEECH,
						Probability: generativelanguagepb.SafetyRating_NEGLIGIBLE,
					},
				},
			},
			expectedOutput: "",
		},
		{
			name: "low ratings not included",
			feedback: &generativelanguagepb.GenerateContentResponse_PromptFeedback{
				SafetyRatings: []*generativelanguagepb.SafetyRating{
					{
						Category:    generativelanguagepb.HarmCategory_HARM_CATEGORY_HATE_SPEECH,
						Probability: generativelanguagepb.SafetyRating_LOW,
					},
				},
			},
			expectedOutput: "",
		},
		{
			name: "multiple ratings",
			feedback: &generativelanguagepb.GenerateContentResponse_PromptFeedback{
				SafetyRatings: []*generativelanguagepb.SafetyRating{
					{
						Category:    generativelanguagepb.HarmCategory_HARM_CATEGORY_HATE_SPEECH,
						Probability: generativelanguagepb.SafetyRating_MEDIUM,
					},
					{
						Category:    generativelanguagepb.HarmCategory_HARM_CATEGORY_DANGEROUS_CONTENT,
						Probability: generativelanguagepb.SafetyRating_HIGH,
					},
				},
			},
			expectedOutput: "[Safety: HARM_CATEGORY_HATE_SPEECH - MEDIUM] [Safety: HARM_CATEGORY_DANGEROUS_CONTENT - HIGH]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := processFeedback(tt.feedback)
			if output != tt.expectedOutput {
				t.Errorf("Expected output %q, got %q", tt.expectedOutput, output)
			}
		})
	}
}

func TestConvertToAlphaFunctionDeclaration(t *testing.T) {
	// Create a test beta function declaration
	betaDecl := generativelanguagepb.FunctionDeclaration{
		Name:        "test_function",
		Description: "Test function for conversion",
	}

	// Convert to alpha
	alphaDecl := convertToAlphaFunctionDeclaration(&betaDecl)

	// Check conversion
	if alphaDecl.Name != betaDecl.Name {
		t.Errorf("Expected name %q, got %q", betaDecl.Name, alphaDecl.Name)
	}
	if alphaDecl.Description != betaDecl.Description {
		t.Errorf("Expected description %q, got %q", betaDecl.Description, alphaDecl.Description)
	}
}
