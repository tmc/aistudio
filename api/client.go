package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"

	language "cloud.google.com/go/ai/generativelanguage/apiv1alpha"
	"cloud.google.com/go/ai/generativelanguage/apiv1alpha/generativelanguagepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/prototext"
)

// Client wraps the Google Generative Language client.
type Client struct {
	APIKey        string // Optional API Key
	GenAI         *language.GenerativeClient
	httpTransport http.RoundTripper // Custom HTTP transport for testing
}

// SetHTTPTransport sets a custom HTTP transport for testing purposes.
// This must be called before InitClient.
func (c *Client) SetHTTPTransport(transport http.RoundTripper) {
	c.httpTransport = transport
}

// ToolDefinition represents a tool definition in a JSON file
type ToolDefinition = generativelanguagepb.FunctionDeclaration

// RegisteredTool represents a tool registered with the application that can be called
type RegisteredTool struct {
	ToolDefinition ToolDefinition                                  // The tool definition
	Handler        func(args json.RawMessage) (interface{}, error) // Function that handles the tool call
	IsAvailable    bool                                            // Whether the tool is currently available
}

// ClientConfig holds configuration passed during stream initialization.
type ClientConfig struct {
	ModelName    string // Model name
	EnableAudio  bool   // Note: v1alpha Bidi setup has limited audio config
	VoiceName    string // Informational, may not be directly usable in v1alpha setup
	SystemPrompt string // System prompt to use for the conversation

	// todo: enable web search
	// todo: enable code execution
	// todo: enable structured output

	ToolDefinitions []ToolDefinition // Tool definitions for the session
}

// StreamOutput holds the processed output from a stream response chunk.
type StreamOutput struct {
	Text  string
	Audio []byte // Raw audio data (PCM S16LE, 24kHz expected if audio is generated)

	FunctionCall        *generativelanguagepb.FunctionCall        // Function call data
	ExecutableCode      *generativelanguagepb.ExecutableCode      // Executable code data
	CodeExecutionResult *generativelanguagepb.CodeExecutionResult // Executable code result data

	// TODO: add other fields
}

// InitClient initializes the underlying Google Cloud Generative Language client.
func (c *Client) InitClient(ctx context.Context) error {
	if c.GenAI != nil {
		return nil
	} // Prevent re-initialization

	// Try additional environment variables if no API key was explicitly provided
	if c.APIKey == "" {
		c.APIKey = os.Getenv("GOOGLE_API_KEY")
	}
	if c.APIKey == "" {
		c.APIKey = os.Getenv("GEMINI_API_KEY")
	}
	if c.APIKey == "" {
		c.APIKey = os.Getenv("GOOGLE_GENERATIVE_AI_KEY")
	}

	var opts []option.ClientOption
	if c.APIKey != "" {
		log.Println("Using provided API Key.")
		opts = append(opts, option.WithAPIKey(c.APIKey))
	} else {
		log.Println("API Key not provided, attempting Application Default Credentials (ADC).")
	}

	// If we have a custom HTTP transport, use it
	if c.httpTransport != nil {
		log.Println("Using custom HTTP transport for testing")
		opts = append(opts, option.WithHTTPClient(&http.Client{
			Transport: c.httpTransport,
		}))
	}

	loggableOpts := []string{}
	for _, o := range opts {
		optStr := fmt.Sprintf("%T", o)
		if !strings.Contains(optStr, "WithAPIKey") {
			loggableOpts = append(loggableOpts, optStr)
		} else {
			loggableOpts = append(loggableOpts, "WithAPIKey(****)")
		}
	}
	log.Printf("Initializing GenerativeClient with options: %v", loggableOpts)

	client, err := language.NewGenerativeClient(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create generative client: %w", err)
	}
	c.GenAI = client
	log.Println("GenerativeClient initialized successfully.")
	return nil
}

// InitWithGRPCConn initializes the client using a pre-configured gRPC connection.
// This is primarily for testing with the rpcreplay package.
func (c *Client) InitWithGRPCConn(ctx context.Context, conn *grpc.ClientConn) error {
	if c.GenAI != nil {
		return nil
	} // Prevent re-initialization

	// Create the client with the provided connection
	// We need to create it ourselves since NewGenerativeClientWithGRPCConn isn't available
	client, err := language.NewGenerativeClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		return fmt.Errorf("failed to create generative client with gRPC connection: %w", err)
	}

	c.GenAI = client
	log.Println("GenerativeClient initialized successfully with gRPC connection.")
	return nil
}

// Close closes the underlying client connection.
func (c *Client) Close() error {
	if c.GenAI != nil {
		log.Println("Closing GenerativeClient connection.")
		err := c.GenAI.Close()
		c.GenAI = nil
		return err
	}
	return nil
}

// InitStreamGenerateContent starts a streaming session using StreamGenerateContent.
// This is a one-way streaming method and not true bidirectional streaming.
func (c *Client) InitStreamGenerateContent(ctx context.Context, config ClientConfig) (generativelanguagepb.GenerativeService_StreamGenerateContentClient, error) {
	if c.GenAI == nil {
		log.Println("InitStreamGenerateContent: Client not initialized, attempting InitClient...")
		if err := c.InitClient(ctx); err != nil {
			return nil, err
		}
	}

	log.Printf("Starting StreamGenerateContent for model: %s", config.ModelName)

	// Create content parts
	contents := []*generativelanguagepb.Content{}

	// Add system prompt if defined
	// Create the content request
	request := &generativelanguagepb.GenerateContentRequest{
		Model:            config.ModelName,
		Contents:         contents,
		GenerationConfig: &generativelanguagepb.GenerationConfig{},
	}

	// Set up audio config if enabled
	if config.EnableAudio {
		log.Printf("Enabling audio output with voice: %s", config.VoiceName)
		if request.GenerationConfig == nil {
			request.GenerationConfig = &generativelanguagepb.GenerationConfig{}
		}
	}

	log.Printf("Sending Content Request: %s", prototext.Format(request))

	// Start streaming content
	stream, err := c.GenAI.StreamGenerateContent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to start stream: %w", err)
	}
	log.Println("StreamGenerateContent started successfully.")

	return stream, nil
}

// InitBidiStream starts a bidirectional streaming session using BidiGenerateContent.
func (c *Client) InitBidiStream(ctx context.Context, config ClientConfig) (generativelanguagepb.GenerativeService_BidiGenerateContentClient, error) {
	if c.GenAI == nil {
		log.Println("InitBidiStream: Client not initialized, attempting InitClient...")
		if err := c.InitClient(ctx); err != nil {
			return nil, err
		}
	}

	log.Printf("Starting BidiGenerateContent for model: %s", config.ModelName)

	// Start bidirectional streaming content
	stream, err := c.GenAI.BidiGenerateContent(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start bidirectional stream: %w", err)
	}
	log.Printf("Bidirectional stream established")

	var fns []*generativelanguagepb.FunctionDeclaration
	for _, td := range config.ToolDefinitions {
		fd := generativelanguagepb.FunctionDeclaration(td)
		fns = append(fns, &fd)
	}

	tools, err := []*generativelanguagepb.Tool{
		{
			FunctionDeclarations: fns,
		},
	}, nil
	if err != nil {
		return nil, fmt.Errorf("failed to parse tool definitions: %w", err)
	}
	log.Printf("Using system prompt: %s", config.SystemPrompt)

	// Log registered tools information
	if len(config.ToolDefinitions) > 0 {
		toolNames := make([]string, 0, len(config.ToolDefinitions))
		for _, td := range config.ToolDefinitions {
			toolNames = append(toolNames, td.Name)
		}
		log.Printf("Registered tools (%d): %s", len(config.ToolDefinitions), strings.Join(toolNames, ", "))
	}

	log.Printf("Using API tools: %v", tools)

	// Send setup message to the server:
	setupMessage := &generativelanguagepb.BidiGenerateContentSetup{
		Model: config.ModelName,
		// Note: Audio config is not fully supported in v1alpha
		// AudioConfig: &generativelanguagepb.AudioConfig{
		//	Voice: config.VoiceName,
		// },
		SystemInstruction: textContent(config.SystemPrompt),
		//SystemInstruction: textContent("You are a pirate captain. You are very rude and aggressive. You are not a friendly assistant."),
		Tools: tools,

		GenerationConfig: &generativelanguagepb.GenerationConfig{
			// todo: response schema
			SpeechConfig: &generativelanguagepb.SpeechConfig{
				VoiceConfig: &generativelanguagepb.VoiceConfig{
					VoiceConfig: &generativelanguagepb.VoiceConfig_PrebuiltVoiceConfig{
						PrebuiltVoiceConfig: &generativelanguagepb.PrebuiltVoiceConfig{
							VoiceName: &config.VoiceName,
						},
					},
				},
			},
		},
	}

	if config.EnableAudio {
		log.Printf("Enabling audio output with voice: %s", config.VoiceName)
		// setupMessage.GenerationConfig.ResponseModalities = []generativelanguagepb.GenerationConfig_Modality{
		// 	generativelanguagepb.GenerationConfig_TEXT,
		// 	generativelanguagepb.GenerationConfig_AUDIO,
		// }
	}

	request := &generativelanguagepb.BidiGenerateContentClientMessage{
		MessageType: &generativelanguagepb.BidiGenerateContentClientMessage_Setup{
			Setup: setupMessage,
		},
	}
	if os.Getenv("DEBUG_AISTUDIO") != "" {
		log.Printf("Sending Raw Bidi Setup Request: %s", prototext.Format(request))
	}

	if err := stream.Send(request); err != nil {
		return nil, fmt.Errorf("failed to send setup message to stream: %w", err)
	}
	log.Printf("Setup message sent to bidirectional stream successfully.")
	return stream, nil
}

// SendMessageToBidiStream sends a message to an existing BidiGenerateContent stream.
func (c *Client) SendMessageToBidiStream(stream generativelanguagepb.GenerativeService_BidiGenerateContentClient, text string) error {
	if stream == nil {
		return fmt.Errorf("stream is nil")
	}
	log.Printf("Sending message to bidirectional stream: %s", text)

	// Create turns array
	turns := []*generativelanguagepb.Content{}
	// Add user message
	turns = append(turns, &generativelanguagepb.Content{
		Parts: []*generativelanguagepb.Part{
			{
				Data: &generativelanguagepb.Part_Text{
					Text: text,
				},
			},
		},
	})

	request := &generativelanguagepb.BidiGenerateContentClientMessage{
		MessageType: &generativelanguagepb.BidiGenerateContentClientMessage_ClientContent{
			ClientContent: &generativelanguagepb.BidiGenerateContentClientContent{
				Turns:        turns,
				TurnComplete: true,
			},
		},
	}

	if os.Getenv("DEBUG_AISTUDIO") != "" {
		log.Printf("Sending Raw Bidi Request: %s", prototext.Format(request))
	}
	if err := stream.Send(request); err != nil {
		return fmt.Errorf("failed to send message to stream: %w", err)
	}
	log.Printf("Message sent to bidirectional stream successfully.")
	return nil
}

// SendToolResultsToBidiStream sends tool results to an existing BidiGenerateContent stream.
func (c *Client) SendToolResultsToBidiStream(stream generativelanguagepb.GenerativeService_BidiGenerateContentClient, toolResults interface{}) error {
	if stream == nil {
		return fmt.Errorf("stream is nil")
	}

	// Serialize the tool results for logging
	resultData, _ := json.Marshal(toolResults)
	log.Printf("Sending tool results to bidirectional stream: %s", string(resultData))

	// Type assertion for the expected structure
	typedResults, ok := toolResults.([]interface{})
	if !ok {
		// Try to handle a slice directly using reflection
		resultValue := reflect.ValueOf(toolResults)
		if resultValue.Kind() == reflect.Slice {
			// Convert reflection slice to interface slice
			slice := make([]interface{}, resultValue.Len())
			for i := 0; i < resultValue.Len(); i++ {
				slice[i] = resultValue.Index(i).Interface()
			}
			typedResults = slice
		} else {
			// If it's not already a slice, wrap it in a slice
			typedResults = []interface{}{toolResults}
		}
	}

	// Create function response content
	var functionResponsesContent []*generativelanguagepb.Content
	
	for _, result := range typedResults {
		var id string
		var response interface{}

		if resultMap, ok := result.(map[string]interface{}); ok {
			// Extract fields from the map
			if idVal, ok := resultMap["id"]; ok {
				id = fmt.Sprintf("%v", idVal)
			}
			
			// Extract the response field
			if responseVal, ok := resultMap["response"]; ok {
				response = responseVal
			}
		}

		// If we have a valid response, add it to the content
		if id != "" {
			// Marshal the response to JSON
			responseJson, err := json.Marshal(response)
			if err != nil {
				log.Printf("Warning: Failed to marshal function response: %v", err)
				responseJson = []byte("{}")
			}

			// Create a content for this function response
			functionResponseContent := &generativelanguagepb.Content{
				Parts: []*generativelanguagepb.Part{
					{
						Data: &generativelanguagepb.Part_Text{
							Text: fmt.Sprintf("Function call response for '%s': %s", id, string(responseJson)),
						},
					},
				},
				Role: "function",
			}
			functionResponsesContent = append(functionResponsesContent, functionResponseContent)
		}
	}

	// Send function responses as regular content for now
	if len(functionResponsesContent) > 0 {
		for _, content := range functionResponsesContent {
			request := &generativelanguagepb.BidiGenerateContentClientMessage{
				MessageType: &generativelanguagepb.BidiGenerateContentClientMessage_ClientContent{
					ClientContent: &generativelanguagepb.BidiGenerateContentClientContent{
						Turns:        []*generativelanguagepb.Content{content},
						TurnComplete: true,
					},
				},
			}

			if os.Getenv("DEBUG_AISTUDIO") != "" {
				log.Printf("Sending tool result as function content: %s", prototext.Format(request))
			}

			if err := stream.Send(request); err != nil {
				return fmt.Errorf("failed to send function response: %w", err)
			}
		}

		log.Printf("Tool results sent as function content successfully.")
		return nil
	}

	// Fallback to text-based approach if the proper API doesn't work
	resultsJson, err := json.MarshalIndent(toolResults, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize tool results: %w", err)
	}

	message := fmt.Sprintf("[TOOL_RESULTS]\n%s\n[/TOOL_RESULTS]", string(resultsJson))

	request := &generativelanguagepb.BidiGenerateContentClientMessage{
		MessageType: &generativelanguagepb.BidiGenerateContentClientMessage_ClientContent{
			ClientContent: &generativelanguagepb.BidiGenerateContentClientContent{
				Turns: []*generativelanguagepb.Content{
					{
						Parts: []*generativelanguagepb.Part{
							{
								Data: &generativelanguagepb.Part_Text{
									Text: message,
								},
							},
						},
					},
				},
				TurnComplete: true,
			},
		},
	}

	if os.Getenv("DEBUG_AISTUDIO") != "" {
		log.Printf("Falling back to text-based tool results: %s", prototext.Format(request))
	}

	if err := stream.Send(request); err != nil {
		return fmt.Errorf("failed to send text-based tool results: %w", err)
	}

	log.Printf("Tool results sent using text fallback successfully.")
	return nil
}

// ExtractBidiOutput extracts text and audio data from the BidiGenerateContentServerMessage.
func ExtractBidiOutput(resp *generativelanguagepb.BidiGenerateContentServerMessage) StreamOutput {
	output := StreamOutput{}
	if resp == nil {
		return output
	}

	if os.Getenv("DEBUG_AISTUDIO") != "" {
		log.Printf("Received Raw Bidi Response: %s", prototext.Format(resp))
	}

	// Extract data from the response
	for _, part := range resp.GetServerContent().GetModelTurn().GetParts() {
		// Extract text
		if textData := part.GetText(); textData != "" {
			output.Text += textData
		}
		if part.GetFunctionCall() != nil {
			log.Printf("Extracted function call: %q", part.GetFunctionCall())
			// print raw proto:
			log.Printf("Extracted function call: %s", prototext.Format(part.GetFunctionCall()))
			output.FunctionCall = part.GetFunctionCall()
		}
		if part.GetExecutableCode() != nil {
			log.Printf("Extracted executable code: %q", part.GetExecutableCode())
			// print raw proto:
			log.Printf("Extracted executable code: %s", prototext.Format(part.GetExecutableCode()))
			output.ExecutableCode = part.GetExecutableCode()
		}
		if part.GetCodeExecutionResult() != nil {
			log.Printf("Extracted executable code result: %q", part.GetCodeExecutionResult())
			// print raw proto:
			log.Printf("Extracted executable code result: %s", prototext.Format(part.GetCodeExecutionResult()))
			output.CodeExecutionResult = part.GetCodeExecutionResult()
		}
		// Extract audio if available - checking any inline data
		if inlineData := part.GetInlineData(); inlineData != nil {
			//log.Printf("Part contains inline data (%s, %d bytes)", inlineData.MimeType, len(inlineData.Data))
			if strings.HasPrefix(inlineData.MimeType, "audio/") {
				//log.Printf("Extracted audio data (%s, %d bytes)", inlineData.MimeType, len(inlineData.Data))
				if len(output.Audio) == 0 {
					output.Audio = inlineData.Data
					//log.Printf("Audio data set in response, %d bytes", len(output.Audio))
				} else {
					log.Println("Warning: Multiple audio parts received, using first.")
				}
			}
		}
		// // todo feedback message handling
		// if feedback := turn.GetPromptFeedback(); feedback != nil {
		// todo: turncomplete handling
		// todo: handle feedback
		// todo: handle groundingfeedback
	}

	// Handle tool calls:
	if toolCall := resp.GetToolCall(); toolCall != nil {
		functionCalls := toolCall.GetFunctionCalls()
		if len(functionCalls) > 0 {
			// For now, just use the first function call. In the future, we might want to handle multiple.
			fn := functionCalls[0]
			log.Printf("Extracted function call: %q", fn.Name)
			// print raw proto:
			log.Printf("Extracted function call: %s", prototext.Format(fn))
			output.FunctionCall = fn
		}
	}

	/*
		case *generativelanguagepb.BidiGenerateContentServerMessage_ServerFeedback:
			if msg.ServerFeedback != nil {
				feedbackText := processFeedback(msg.ServerFeedback)
				if feedbackText != "" {
					output.Text += " " + feedbackText
				}
			}
		default:
			log.Printf("Unknown message type: %T", msg)
		}
	*/
	output.Text = strings.TrimSpace(output.Text)
	if os.Getenv("DEBUG_AISTUDIO") != "" {
		if output.Audio != nil {
			log.Printf("Extracted audio data, %d bytes", len(output.Audio))
		}
		if output.Text != "" {
			log.Printf("Extracted text data: %q", output.Text)
		}
		if output.Audio != nil {
			log.Printf("Extracted audio data, %d bytes", len(output.Audio))
		}
		log.Printf("Final Extracted Output: %q", output.Text)
		log.Printf("Final Extracted Audio Data: %d bytes", len(output.Audio))
	}
	// Trim whitespace from the text output
	output.Text = strings.TrimSpace(output.Text)
	if output.Text == "" && len(output.Audio) == 0 && output.FunctionCall == nil && output.ExecutableCode == nil && output.CodeExecutionResult == nil {
		log.Printf("Received response chunk contained no processable ouput: %s", prototext.Format(resp))
	}
	return output
}

// ExtractOutput extracts text and audio data from the GenerateContentResponse.
func ExtractOutput(resp *generativelanguagepb.GenerateContentResponse) StreamOutput {
	output := StreamOutput{}
	if resp == nil {
		return output
	}

	if os.Getenv("DEBUG_AISTUDIO") != "" {
		log.Printf("Received Raw Response: %s", prototext.Format(resp))
	}
	log.Printf("got response: %T", resp)

	// Extract data from the response
	if resp.Candidates != nil && len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]

		if candidate.Content != nil && candidate.Content.Parts != nil {
			for _, part := range candidate.Content.Parts {
				// Extract text
				if textData := part.GetText(); textData != "" {
					log.Printf("Extracted text from part: %q", textData)
					output.Text += textData
				}

				// Extract audio if available - checking any inline data
				if inlineData := part.GetInlineData(); inlineData != nil {
					log.Printf("Part contains inline data (%s, %d bytes)",
						inlineData.MimeType, len(inlineData.Data))
					if strings.HasPrefix(inlineData.MimeType, "audio/") {
						log.Printf("Extracted audio data (%s, %d bytes)",
							inlineData.MimeType, len(inlineData.Data))
						if len(output.Audio) == 0 {
							output.Audio = inlineData.Data
							log.Printf("Audio data set in response, %d bytes", len(output.Audio))
						} else {
							log.Println("Warning: Multiple audio parts received, using first.")
						}
					}
				}
			}
		}
	}

	// Handle feedback
	if promptFeedback := resp.GetPromptFeedback(); promptFeedback != nil {
		feedbackText := processFeedback(promptFeedback)
		if feedbackText != "" {
			output.Text += " " + feedbackText
		}
	}

	if output.Text == "" && len(output.Audio) == 0 && output.FunctionCall == nil && output.ExecutableCode == nil && output.CodeExecutionResult == nil {

		log.Printf("Received response chunk contained no processable output: %s", prototext.Format(resp))
	}

	output.Text = strings.TrimSpace(output.Text)
	return output
}

// Note: extractTextFromContent function was removed as the implementation
// depends on specific API types that may not be available in all versions

// processFeedback formats feedback into a string.
func processFeedback(promptFeedback *generativelanguagepb.GenerateContentResponse_PromptFeedback) string {
	var feedbackParts []string
	if promptFeedback.BlockReason != generativelanguagepb.GenerateContentResponse_PromptFeedback_BLOCK_REASON_UNSPECIFIED {
		reasonStr := promptFeedback.BlockReason.String()
		log.Printf("Received prompt feedback: Blocked - %s", reasonStr)
		feedbackParts = append(feedbackParts, fmt.Sprintf("[Blocked: %s]", reasonStr))
	}
	for _, rating := range promptFeedback.SafetyRatings {
		if rating.Probability != generativelanguagepb.SafetyRating_NEGLIGIBLE && rating.Probability != generativelanguagepb.SafetyRating_LOW {
			categoryStr := rating.Category.String()
			probStr := rating.Probability.String()
			log.Printf("Received safety rating: %s - %s", categoryStr, probStr)
			feedbackParts = append(feedbackParts, fmt.Sprintf("[Safety: %s - %s]", categoryStr, probStr))
		}
	}
	return strings.Join(feedbackParts, " ")
}

// parseToolDefinitions parses the tool definitions from a JSON string.
func parseToolDefinitions(jsonStr string) ([]*generativelanguagepb.Tool, error) {
	var tools []*generativelanguagepb.Tool
	if jsonStr == "" {
		return tools, nil
	}

	// Unmarshal the JSON string into a slice of ToolDefinition
	err := json.Unmarshal([]byte(jsonStr), &tools)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tool definitions: %w", err)
	}

	log.Printf("Parsed Tool Definitions: %v", tools)
	return tools, nil
}

type textContentOption func(c *generativelanguagepb.Content)

// WithRole sets the role of the content.
func WithRole(role string) textContentOption {
	return func(c *generativelanguagepb.Content) {
		c.Role = role
	}
}

// textContent creates a Content object with the given text.
func textContent(text string, opts ...textContentOption) *generativelanguagepb.Content {
	c := &generativelanguagepb.Content{
		Parts: []*generativelanguagepb.Part{
			{
				Data: &generativelanguagepb.Part_Text{
					Text: text,
				},
			},
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
