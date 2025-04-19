package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	language "cloud.google.com/go/ai/generativelanguage/apiv1alpha"
	"cloud.google.com/go/ai/generativelanguage/apiv1alpha/generativelanguagepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/structpb"
)

// Client wraps the Google Generative Language client.
type Client struct {
	APIKey           string // Optional API Key
	GenerativeClient *language.GenerativeClient
	httpTransport    http.RoundTripper // Custom HTTP transport for testing
}

// SetHTTPTransport sets a custom HTTP transport for testing purposes.
// This must be called before InitClient.
func (c *Client) SetHTTPTransport(transport http.RoundTripper) {
	c.httpTransport = transport
}

// ToolDefinition represents a tool definition in a JSON file
type ToolDefinition = generativelanguagepb.FunctionDeclaration

// ToolResponse
type ToolResponse = generativelanguagepb.FunctionResponse

// RegisteredTool represents a tool registered with the application that can be called
type RegisteredTool struct {
	ToolDefinition ToolDefinition                          // The tool definition
	Handler        func(args json.RawMessage) (any, error) // Function that handles the tool call
	IsAvailable    bool                                    // Whether the tool is currently available
}

// ClientConfig holds configuration passed during stream initialization.
type ClientConfig struct {
	ModelName    string // Model name
	EnableAudio  bool   // Note: v1alpha Bidi setup has limited audio config
	VoiceName    string // Informational, may not be directly usable in v1alpha setup
	SystemPrompt string // System prompt to use for the conversation

	// Generation configuration parameters
	Temperature     float32 // Controls randomness (0.0-1.0)
	TopP            float32 // Controls diversity (0.0-1.0)
	TopK            int32   // Number of highest probability tokens to consider
	MaxOutputTokens int32   // Maximum number of tokens to generate

	// Feature flags
	EnableWebSearch     bool   // Enable web search/grounding capabilities
	EnableCodeExecution bool   // Enable code execution capabilities
	ResponseMimeType    string // MIME type of the expected response (e.g., "application/json")
	ResponseSchemaFile  string // Path to JSON schema file defining response structure

	// Display options
	DisplayTokenCounts bool // Whether to display token counts in the UI

	ToolDefinitions []ToolDefinition // Tool definitions for the session
}

// StreamOutput holds the processed output from a stream response chunk.
type StreamOutput struct {
	Text  string
	Audio []byte // Raw audio data (PCM S16LE, 24kHz expected if audio is generated)

	FunctionCall        *generativelanguagepb.FunctionCall        // Function call data
	ExecutableCode      *generativelanguagepb.ExecutableCode      // Executable code data
	CodeExecutionResult *generativelanguagepb.CodeExecutionResult // Executable code result data

	SetupComplete *bool
	TurnComplete  bool

	// Additional feedback and metadata
	SafetyRatings     []*generativelanguagepb.SafetyRating    // Safety ratings for content
	GroundingMetadata *generativelanguagepb.GroundingMetadata // Grounding metadata

	// Usage information
	PromptTokenCount    int32 // Number of tokens in the prompt (only available at end of response)
	CandidateTokenCount int32 // Number of tokens in the response (only available at end of response)
	TotalTokenCount     int32 // Total tokens used (prompt + response, only available at end of response)
}

// InitClient initializes the underlying Google Cloud Generative Language client.
func (c *Client) InitClient(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	if c.GenerativeClient != nil {
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
	c.GenerativeClient = client
	log.Println("GenerativeClient initialized successfully.")
	return nil
}

// InitWithGRPCConn initializes the client using a pre-configured gRPC connection.
// This is primarily for testing with the rpcreplay package.
func (c *Client) InitWithGRPCConn(ctx context.Context, conn *grpc.ClientConn) error {
	if c.GenerativeClient != nil {
		return nil
	} // Prevent re-initialization

	// Create the client with the provided connection
	// We need to create it ourselves since NewGenerativeClientWithGRPCConn isn't available
	client, err := language.NewGenerativeClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		return fmt.Errorf("failed to create generative client with gRPC connection: %w", err)
	}

	c.GenerativeClient = client
	log.Println("GenerativeClient initialized successfully with gRPC connection.")
	return nil
}

// Close closes the underlying client connection.
func (c *Client) Close() error {
	if c.GenerativeClient != nil {
		log.Println("Closing GenerativeClient connection.")
		err := c.GenerativeClient.Close()
		c.GenerativeClient = nil
		return err
	}
	return nil
}

// InitStreamGenerateContent starts a streaming session using StreamGenerateContent.
// This is a one-way streaming method and not true bidirectional streaming.
func (c *Client) InitStreamGenerateContent(ctx context.Context, config ClientConfig) (generativelanguagepb.GenerativeService_StreamGenerateContentClient, error) {
	return nil, fmt.Errorf("only bidi streaming is supported at the moment")
	if c.GenerativeClient == nil {
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
		Model:    config.ModelName,
		Contents: contents,
	}

	// Set up audio config if enabled
	if config.EnableAudio {
		log.Printf("Enabling audio output with voice: %s", config.VoiceName)
		if request.GenerationConfig == nil {
			request.GenerationConfig = &generativelanguagepb.GenerationConfig{
				SpeechConfig: &generativelanguagepb.SpeechConfig{
					VoiceConfig: &generativelanguagepb.VoiceConfig{
						VoiceConfig: &generativelanguagepb.VoiceConfig_PrebuiltVoiceConfig{
							PrebuiltVoiceConfig: &generativelanguagepb.PrebuiltVoiceConfig{
								VoiceName: &config.VoiceName,
							},
						},
					},
				},
			}
		}
	}

	log.Printf("Sending Content Request: %s", prototext.Format(request))

	// Start streaming content
	stream, err := c.GenerativeClient.StreamGenerateContent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to start stream: %w", err)
	}
	log.Println("StreamGenerateContent started successfully.")

	return stream, nil
}

// InitBidiStream starts a bidirectional streaming session using BidiGenerateContent.
func (c *Client) InitBidiStream(ctx context.Context, config ClientConfig) (generativelanguagepb.GenerativeService_BidiGenerateContentClient, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	if c.GenerativeClient == nil {
		log.Println("InitBidiStream: Client not initialized, attempting InitClient...")
		if err := c.InitClient(ctx); err != nil {
			return nil, err
		}
	}

	log.Printf("Starting BidiGenerateContent for model: %s", config.ModelName)

	// Start bidirectional streaming content
	stream, err := c.GenerativeClient.BidiGenerateContent(ctx)
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

	// Set up GenerationConfig with conditional fields
	genConfig := &generativelanguagepb.GenerationConfig{}
	genConfig.Temperature = &config.Temperature
	if config.TopP > 0 {
		genConfig.TopP = &config.TopP
	}
	if config.TopK > 0 {
		genConfig.TopK = &config.TopK
	}
	if config.MaxOutputTokens > 0 {
		genConfig.MaxOutputTokens = &config.MaxOutputTokens
	}
	// Only set voice config if VoiceName is specified
	if config.VoiceName != "" {
		genConfig.SpeechConfig = &generativelanguagepb.SpeechConfig{
			VoiceConfig: &generativelanguagepb.VoiceConfig{
				VoiceConfig: &generativelanguagepb.VoiceConfig_PrebuiltVoiceConfig{
					PrebuiltVoiceConfig: &generativelanguagepb.PrebuiltVoiceConfig{
						VoiceName: &config.VoiceName,
					},
				},
			},
		}
	}
	// Send setup message to the server:
	setupMessage := &generativelanguagepb.BidiGenerateContentSetup{
		Model:             config.ModelName,
		SystemInstruction: textContent(config.SystemPrompt),
		Tools:             tools,
		GenerationConfig:  genConfig,
	}

	// Set response MIME type if specified
	if config.ResponseMimeType != "" {
		setupMessage.GenerationConfig.ResponseMimeType = config.ResponseMimeType
	}

	// If output schema is specified, set it in the setup message, and set the mime type to application/json:
	if config.ResponseSchemaFile != "" {
		schema, err := os.ReadFile(config.ResponseSchemaFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read response schema file: %w", err)
		}
		schemaObj := &generativelanguagepb.Schema{}
		m := protojson.UnmarshalOptions{}
		if err := m.Unmarshal(schema, schemaObj); err != nil {
			return nil, fmt.Errorf("failed to parse response schema: %w", err)
		}
		log.Printf("Parsed response schema: %s", prototext.Format(schemaObj))
		setupMessage.GenerationConfig.ResponseSchema = schemaObj
		setupMessage.GenerationConfig.ResponseMimeType = "application/json"
	}

	if setupMessage.GenerationConfig == nil {
		setupMessage.GenerationConfig = &generativelanguagepb.GenerationConfig{}
	}
	setupMessage.GenerationConfig.ResponseModalities = []generativelanguagepb.GenerationConfig_Modality{
		generativelanguagepb.GenerationConfig_TEXT,
	}
	if config.EnableAudio {
		// setupMessage.GenerationConfig.ResponseModalities = append(setupMessage.GenerationConfig.ResponseModalities, generativelanguagepb.GenerationConfig_AUDIO)
		setupMessage.GenerationConfig.ResponseModalities = []generativelanguagepb.GenerationConfig_Modality{
			generativelanguagepb.GenerationConfig_AUDIO,
		}
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

// CustomToolResponse represents a simplified tool response structure
type CustomToolResponse struct {
	Name     string          // Function name/ID
	Response *structpb.Value // JSON response content as structpb.Value
}

// SendToolResultsToBidiStream sends tool results to an existing BidiGenerateContent stream.
func (c *Client) SendToolResultsToBidiStream(stream generativelanguagepb.GenerativeService_BidiGenerateContentClient, toolResults ...*generativelanguagepb.FunctionResponse) error {
	if stream == nil {
		return fmt.Errorf("stream is nil")
	}

	// Create the client message
	tr := &generativelanguagepb.BidiGenerateContentToolResponse{
		FunctionResponses: toolResults,
	}

	request := &generativelanguagepb.BidiGenerateContentClientMessage{
		MessageType: &generativelanguagepb.BidiGenerateContentClientMessage_ToolResponse{
			ToolResponse: tr,
		},
	}

	if os.Getenv("DEBUG_AISTUDIO") != "" {
		log.Printf("Sending tool responses using content API: %s", prototext.Format(tr))
	}

	if err := stream.Send(request); err != nil {
		return fmt.Errorf("failed to send tool responses: %w", err)
	}

	log.Printf("Tool responses sent successfully using content API.")
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
		//log.Printf("part type: %T", part.GetData())
		// Extract text
		if textData := part.GetText(); textData != "" {
			output.Text += textData
			log.Printf("Extracted text from part: %q", textData)
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

	// Look for safety ratings in the response
	// In the Gemini API, safety ratings might come in different places
	// For now, we'll look for direct safety information in the parts
	if resp.GetServerContent() != nil && resp.GetServerContent().GetModelTurn() != nil {
		for _, part := range resp.GetServerContent().GetModelTurn().GetParts() {
			// Check if this part has metadata with safety ratings (not guaranteed in proto)
			// This is a more generic approach since the exact API structure might vary
			if field := reflect.ValueOf(part).Elem().FieldByName("SafetyRatings"); field.IsValid() {
				ratings, ok := field.Interface().([]*generativelanguagepb.SafetyRating)
				if ok && len(ratings) > 0 {
					log.Printf("Extracted %d safety ratings from part metadata", len(ratings))
					output.SafetyRatings = ratings
				}
			}

			// Try to extract any content blocking information (if available)
			if field := reflect.ValueOf(part).Elem().FieldByName("BlockedReasons"); field.IsValid() {
				if field.Len() > 0 {
					log.Printf("Content contains blocked reasons")
				}
			}
		}
	}

	// Extract grounding metadata if available
	if resp.GetServerContent() != nil && resp.GetServerContent().GetGroundingMetadata() != nil {
		log.Printf("Extracted grounding metadata")
		output.GroundingMetadata = resp.GetServerContent().GetGroundingMetadata()
	}
	if resp.GetSetupComplete() != nil {
		log.Printf("Setup complete message received")
		v := true
		output.SetupComplete = &v
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
	// Check if this is the final chunk in the response
	// The API doesn't directly expose token counts in the expected way,
	// so we'll use a simpler approach to detect final chunks
	if resp.GetServerContent() != nil {
		// Check if this is the final chunk based on turn completion
		if resp.GetServerContent().GetTurnComplete() {
			output.TurnComplete = true
			log.Printf("Final response chunk detected")
		}

		// We don't have direct access to token counts in this API version,
		// but we can estimate based on text length as a fallback
		if output.TurnComplete && output.Text != "" {
			// Very rough estimate: ~4 chars per token on average
			estimatedTokens := len(output.Text) / 4
			output.CandidateTokenCount = int32(estimatedTokens)
			output.TotalTokenCount = int32(estimatedTokens) // Without prompt info

			log.Printf("Estimated token usage (rough): Response≈%d", estimatedTokens)
		}
	}
	// TODO: move to helper on StreamOutput
	if output.Text == "" && len(output.Audio) == 0 && output.FunctionCall == nil && output.ExecutableCode == nil && output.CodeExecutionResult == nil && output.TurnComplete == false {
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
					// Always log when we get text to help with debugging
					if strings.TrimSpace(textData) != "" {
						log.Printf("RECEIVED TEXT: %q", textData)
					}
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

	if output.Text == "" && len(output.Audio) == 0 && output.FunctionCall == nil && output.ExecutableCode == nil && output.CodeExecutionResult == nil && output.TurnComplete == false {

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

// ClientError represents an error from the client.
type ClientError struct {
	Code    int    // HTTP status code
	Message string // Error message
}

func (e *ClientError) Error() string {
	return fmt.Sprintf("Client Error: %d - %s", e.Code, e.Message)
}

// IsRetryable returns true if the error is retryable.
func (e *ClientError) IsRetryable() bool {
	// Add your logic to determine if the error is retryable
	// For example, you could check the HTTP status code
	// and consider 5xx errors as retryable
	return e.Code >= 500 &&
		e.Code < 600 &&
		!strings.Contains(e.Message, "canceled") &&
		!strings.Contains(e.Message, "deadline")
}

// RetryableError represents an error that can be retried.
type RetryableError struct {
	Err        error         // The underlying error
	Retries    int           // Number of retries attempted
	MaxRetries int           // Maximum number of retries allowed
	Backoff    time.Duration // Backoff duration for the next retry
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("Retryable Error: %s (retries: %d/%d, backoff: %v)", e.Err.Error(), e.Retries, e.MaxRetries, e.Backoff)
}

// IsRetryable returns true if the error is retryable and the maximum number of retries has not been reached.
func (e *RetryableError) IsRetryable() bool {
	return e.Retries < e.MaxRetries
}

// Retry retries the operation with exponential backoff.
func (e *RetryableError) Retry(ctx context.Context, op func() error) error {
	if !e.IsRetryable() {
		return e.Err
	}

	// Add jitter to the backoff time (±10%)
	jitter := float64(e.Backoff) * 0.1 * (2*rand.Float64() - 1)
	backoffWithJitter := e.Backoff + time.Duration(jitter)
	if backoffWithJitter < 0 {
		backoffWithJitter = e.Backoff // Ensure backoff is never negative
	}

	// Use a timer for backoff that respects context
	timer := time.NewTimer(backoffWithJitter)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		// Context was canceled or timed out
		return ctx.Err()
	case <-timer.C:
		// Backoff time elapsed, proceed with retry
	}

	e.Retries++
	e.Backoff *= 2 // Exponential backoff

	err := op()
	if err != nil {
		if clientErr, ok := err.(*ClientError); ok && clientErr.IsRetryable() {
			return &RetryableError{
				Err:        err,
				Retries:    e.Retries,
				MaxRetries: e.MaxRetries,
				Backoff:    e.Backoff,
			}
		}
		return err
	}

	return nil
}

// RetryWithExponentialBackoff retries the given operation with exponential backoff.
func RetryWithExponentialBackoff(ctx context.Context, op func() error, maxRetries int, initialBackoff time.Duration) error {
	// Check context before first attempt
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// First attempt
	err := op()
	if err == nil {
		return nil // Success on first try
	}

	// If error is not retryable, return immediately
	clientErr, ok := err.(*ClientError)
	if !(ok && clientErr.IsRetryable()) {
		return err
	}

	// Set up for retries
	attempt := 1
	backoff := initialBackoff

	// Retry loop
	for attempt <= maxRetries {
		// Add jitter to backoff (±10%)
		jitter := float64(backoff) * 0.1 * (2*rand.Float64() - 1)
		actualBackoff := backoff + time.Duration(jitter)
		if actualBackoff < 0 {
			actualBackoff = backoff // Ensure backoff is never negative
		}

		// Wait with respect to context
		timer := time.NewTimer(actualBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			// Continue with retry
		}

		// Attempt the operation
		err = op()
		if err == nil {
			return nil // Success
		}

		// Check if error is retryable
		clientErr, ok := err.(*ClientError)
		if !(ok && clientErr.IsRetryable()) {
			return err // Non-retryable error
		}

		// Prepare for next retry
		attempt++
		backoff *= 2 // Exponential backoff
	}

	// Exhausted all retries
	return err
}
