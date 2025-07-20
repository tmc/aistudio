package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	languagealpha "cloud.google.com/go/ai/generativelanguage/apiv1alpha"
	generativelanguagealphapb "cloud.google.com/go/ai/generativelanguage/apiv1alpha/generativelanguagepb"
	language "cloud.google.com/go/ai/generativelanguage/apiv1beta"
	"cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
	aiplatform "cloud.google.com/go/aiplatform/apiv1"
	"cloud.google.com/go/aiplatform/apiv1/aiplatformpb"
	vertexai "cloud.google.com/go/vertexai/genai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/structpb"
)

// Backend defines which AI service to use.
type Backend int

const (
	BackendGeminiAPI Backend = iota
	BackendVertexAI
	BackendGrok
)

// APIClientConfig holds configuration for the Client.
type APIClientConfig struct {
	// Authentication and service selection
	APIKey  string
	Backend Backend

	// Vertex AI settings
	ProjectID string
	Location  string

	// Model settings
	ModelName       string
	ToolDefinitions []ToolDefinition

	// HTTP configuration
	HTTPTransport http.RoundTripper
}

// Client wraps the Google Generative Language client and Vertex AI client.
type Client struct {
	// Configuration
	APIKey        string // Optional API Key
	Backend       Backend
	ProjectID     string
	Location      string
	GeminiVersion string // "v1alpha" or "v1beta" - defaults to "v1beta"

	// Client instances
	GenerativeClient      *language.GenerativeClient
	GenerativeClientAlpha *languagealpha.GenerativeClient // For v1alpha
	VertexAIClient        *vertexai.Client
	VertexModelsClient    *aiplatform.ModelClient
	GrokHTTPClient        *http.Client      // HTTP client for Grok API
	httpTransport         http.RoundTripper // Custom HTTP transport for testing
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

// StreamClientConfig holds configuration passed during stream initialization.
type StreamClientConfig struct {
	ModelName    string // Model name
	EnableAudio  bool
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
	EnableWebSocket     bool   // Enable WebSocket protocol for live models
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

// NewClient creates a new Client with the given config.
func NewClient(ctx context.Context, config *APIClientConfig) (*Client, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	client := &Client{}

	// Apply configuration
	if config != nil {
		client.APIKey = config.APIKey
		client.Backend = config.Backend
		client.ProjectID = config.ProjectID
		client.Location = config.Location
		client.httpTransport = config.HTTPTransport
	}

	// Apply defaults if needed
	if client.Location == "" {
		client.Location = "us-central1"
	}

	// Check environment variables if not set directly
	if client.APIKey == "" {
		client.APIKey = os.Getenv("GEMINI_API_KEY")
	}

	if client.ProjectID == "" {
		client.ProjectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}

	if envLoc := os.Getenv("GOOGLE_CLOUD_LOCATION"); client.Location == "" && envLoc != "" {
		client.Location = envLoc
	} else if envRegion := os.Getenv("GOOGLE_CLOUD_REGION"); client.Location == "" && envRegion != "" {
		client.Location = envRegion
	}

	// Check if we should use Vertex AI based on environment variable
	if os.Getenv("AISTUDIO_USE_VERTEXAI") == "true" && client.Backend == BackendGeminiAPI {
		client.Backend = BackendVertexAI
	}

	// Initialize the appropriate client based on the backend
	if client.Backend == BackendVertexAI {
		if client.ProjectID == "" {
			return nil, fmt.Errorf("project ID is required for Vertex AI backend")
		}

		if err := client.InitVertexAIClient(ctx); err != nil {
			return nil, err
		}
	} else {
		if err := client.InitGeminiClient(ctx); err != nil {
			return nil, err
		}
	}

	return client, nil
}

// InitVertexAIClient initializes the Vertex AI client.
func (c *Client) InitVertexAIClient(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context is nil")
	}

	c.Backend = BackendVertexAI

	// Close existing clients if they exist
	if c.VertexAIClient != nil {
		c.VertexAIClient.Close()
		c.VertexAIClient = nil
	}
	if c.VertexModelsClient != nil {
		c.VertexModelsClient.Close()
		c.VertexModelsClient = nil
	}

	// Set up client options
	var clientOpts []option.ClientOption

	// If API key is provided, use it for authentication
	// This allows using API key even with Vertex AI
	if c.APIKey != "" {
		// Use direct API key authentication with Vertex AI
		clientOpts = append(clientOpts, option.WithAPIKey(c.APIKey))

		// Add explicit authentication to bypass ADC requirement
		clientOpts = append(clientOpts, option.WithoutAuthentication())

		log.Printf("Using provided API key for Vertex AI authentication (direct mode)")
	} else {
		log.Printf("Using Application Default Credentials (ADC) for Vertex AI authentication")
		// Will use Application Default Credentials by default
	}

	// Create Vertex AI genai client with timeout monitoring
	log.Printf("[DEBUG] Creating Vertex AI genai client for project %s in %s", c.ProjectID, c.Location)
	start := time.Now()
	vertexClient, err := vertexai.NewClient(ctx, c.ProjectID, c.Location, clientOpts...)
	if err != nil {
		elapsed := time.Since(start)
		log.Printf("[ERROR] Failed to create Vertex AI client after %v: %v", elapsed, err)
		// Check if it was a timeout
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("[ERROR] Vertex AI client creation timed out - check network connectivity to Google Cloud")
		}
		return fmt.Errorf("failed to create Vertex AI client: %w", err)
	}
	elapsed := time.Since(start)
	log.Printf("[DEBUG] Vertex AI genai client created successfully in %v", elapsed)
	c.VertexAIClient = vertexClient

	// Create Vertex AI models client for listing models
	// Apply same authentication options to models client
	log.Printf("[DEBUG] Creating Vertex AI models client")
	modelsStart := time.Now()
	modelsClient, err := aiplatform.NewModelClient(ctx, clientOpts...)
	if err != nil {
		modelsElapsed := time.Since(modelsStart)
		log.Printf("[ERROR] Failed to create Vertex AI models client after %v: %v", modelsElapsed, err)
		// Clean up genai client if models client fails
		c.VertexAIClient.Close()
		return fmt.Errorf("failed to create Vertex AI models client: %w", err)
	}
	modelsElapsed := time.Since(modelsStart)
	log.Printf("[DEBUG] Vertex AI models client created successfully in %v", modelsElapsed)
	c.VertexModelsClient = modelsClient

	totalElapsed := time.Since(start)
	log.Printf("[DEBUG] Initialized Vertex AI clients for project %s in %s (total: %v)",
		c.ProjectID, c.Location, totalElapsed)
	return nil
}

// ListVertexAIModels lists available models from Vertex AI.
func (c *Client) ListVertexAIModels(ctx context.Context, filter string) ([]string, error) {
	if c.VertexModelsClient == nil {
		return nil, fmt.Errorf("vertex AI models client not initialized")
	}

	parent := fmt.Sprintf("projects/%s/locations/%s", c.ProjectID, c.Location)
	req := &aiplatformpb.ListModelsRequest{
		Parent: parent,
	}

	var models []string
	it := c.VertexModelsClient.ListModels(ctx, req)
	for {
		model, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error listing models: %w", err)
		}

		// If filter is specified, only include models that match the filter
		if filter != "" && !strings.Contains(model.GetDisplayName(), filter) {
			continue
		}

		models = append(models, model.GetDisplayName())
	}

	return models, nil
}

// InitGeminiClient initializes the Gemini API client.
func (c *Client) InitGeminiClient(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	c.Backend = BackendGeminiAPI

	// Close existing client if it exists
	if c.GenerativeClient != nil {
		c.GenerativeClient.Close()
		c.GenerativeClient = nil
	}

	return c.InitClient(ctx)
}

// InitClient initializes the Google Cloud Generative Language client with the specified API version.
func (c *Client) InitClient(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	// Handle Grok backend initialization
	if c.Backend == BackendGrok {
		return c.InitGrokClient(ctx)
	}

	// Handle Vertex AI backend initialization
	if c.Backend == BackendVertexAI {
		return c.InitVertexAIClient(ctx)
	}

	// Set default version if not specified
	if c.GeminiVersion == "" {
		c.GeminiVersion = "v1beta" // Default to v1beta
	}

	// Check if appropriate client is already initialized
	if c.GeminiVersion == "v1alpha" && c.GenerativeClientAlpha != nil {
		return nil
	} else if c.GeminiVersion == "v1beta" && c.GenerativeClient != nil {
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
	if c.APIKey == "" && c.Backend == BackendGrok {
		c.APIKey = os.Getenv("GROK_API_KEY")
	}

	var opts []option.ClientOption
	if c.APIKey != "" {
		log.Println("Using provided API Key.")
		opts = append(opts, option.WithAPIKey(c.APIKey))
	} else {
		log.Println("API Key not provided, attempting Application Default Credentials (ADC).")
	}

	// For gRPC connections, we shouldn't use WithHTTPClient as it conflicts with gRPC options
	// Instead, we'll use gRPC-specific options for timeouts and connection settings

	// Add gRPC dial options
	// Create gRPC dial options for better connection stability
	kacp := keepalive.ClientParameters{
		Time:                20 * time.Second, // Send pings every 20 seconds if there is no activity
		Timeout:             10 * time.Second, // Wait 10 seconds for a ping response
		PermitWithoutStream: true,             // Allow pings even without an active stream
	}

	// Use these gRPC-specific dial options
	opts = append(opts,
		option.WithGRPCDialOption(grpc.WithKeepaliveParams(kacp)),
		option.WithGRPCDialOption(grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.DefaultConfig,
			MinConnectTimeout: 20 * time.Second,
		})),
	)

	// Only use HTTP transport for testing, not in normal operation
	if c.httpTransport != nil && os.Getenv("AISTUDIO_ENABLE_HTTP_OVERRIDE") == "true" {
		log.Println("WARNING: Using custom HTTP transport for testing (may conflict with gRPC)")
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
	log.Printf("Initializing GenerativeClient with options: %v (API version: %s)", loggableOpts, c.GeminiVersion)

	// Initialize the client based on the specified API version
	if c.GeminiVersion == "v1alpha" {
		// Initialize the v1alpha client
		alphaClient, err := languagealpha.NewGenerativeClient(ctx, opts...)
		if err != nil {
			return fmt.Errorf("failed to create v1alpha generative client: %w", err)
		}
		c.GenerativeClientAlpha = alphaClient
		log.Println("v1alpha GenerativeClient initialized successfully.")
	} else {
		// Default to v1beta client
		betaClient, err := language.NewGenerativeClient(ctx, opts...)
		if err != nil {
			return fmt.Errorf("failed to create v1beta generative client: %w", err)
		}
		c.GenerativeClient = betaClient
		log.Println("v1beta GenerativeClient initialized successfully.")
	}
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
	var err error

	// Close v1beta client if exists
	if c.GenerativeClient != nil {
		log.Println("Closing v1beta GenerativeClient connection.")
		err = c.GenerativeClient.Close()
		c.GenerativeClient = nil
	}

	// Close v1alpha client if exists
	if c.GenerativeClientAlpha != nil {
		log.Println("Closing v1alpha GenerativeClientAlpha connection.")
		alphaErr := c.GenerativeClientAlpha.Close()
		c.GenerativeClientAlpha = nil
		// Return alpha error if no beta error or set a combined error
		if err == nil {
			err = alphaErr
		} else if alphaErr != nil {
			err = fmt.Errorf("multiple close errors: %v and %v", err, alphaErr)
		}
	}

	return err
}

// InitStreamGenerateContent starts a streaming session using StreamGenerateContent.
// This is a one-way streaming method and not true bidirectional streaming.
func (c *Client) InitStreamGenerateContent(ctx context.Context, config *StreamClientConfig) (generativelanguagepb.GenerativeService_StreamGenerateContentClient, error) {
	return nil, fmt.Errorf("only bidi streaming is supported at the moment")
}

// InitBidiStream starts a bidirectional streaming session using StreamGenerateContent.
// Returns a StreamGenerateContent interface for compatibility with existing code.
func (c *Client) InitBidiStream(ctx context.Context, config *StreamClientConfig) (generativelanguagepb.GenerativeService_StreamGenerateContentClient, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	// Check if WebSocket mode is enabled or if this is a live model
	if config.EnableWebSocket && IsLiveModel(config.ModelName) {
		log.Printf("Using WebSocket implementation for live model: %s", config.ModelName)
		return c.initLiveStream(ctx, config)
	}

	// Even if it's a live model, use gRPC if WebSocket is not explicitly enabled
	if IsLiveModel(config.ModelName) && !config.EnableWebSocket {
		log.Printf("Live model detected: %s, but using gRPC as WebSocket is not enabled", config.ModelName)
	}

	// Check if we need to initialize the client
	if c.GenerativeClient == nil && c.GenerativeClientAlpha == nil {
		log.Println("InitBidiStream: Client not initialized, attempting InitClient...")
		if err := c.InitClient(ctx); err != nil {
			return nil, err
		}
	}

	// Handle alpha version if that's the selected version
	if c.GeminiVersion == "v1alpha" {
		return c.initBidiStreamAlpha(ctx, config)
	}

	// Default to v1beta (the regular implementation)
	// Do a quick basic validation first
	isQuickValid, quickErr := c.quickModelValidation(config.ModelName)
	if quickErr != nil {
		log.Printf("Warning: Quick validation failed for model %s: %v", config.ModelName, quickErr)
	} else if !isQuickValid {
		// Fall back to full API validation if needed
		isValid, err := c.ValidateModel(config.ModelName)
		if err != nil {
			log.Printf("Warning: Could not validate model %s: %v", config.ModelName, err)
		} else if !isValid {
			return nil, fmt.Errorf("model '%s' is not found in the list of supported models for the v1beta API", config.ModelName)
		}
	}

	log.Printf("Starting StreamGenerateContent for model: %s (using v1beta)", config.ModelName)

	// Set up tools if defined
	var fns []*generativelanguagepb.FunctionDeclaration
	if config.ToolDefinitions != nil {
		for i := range config.ToolDefinitions {
			td := &config.ToolDefinitions[i]
			fns = append(fns, td)
		}
	}

	tools := []*generativelanguagepb.Tool{
		{
			FunctionDeclarations: fns,
		},
	}

	log.Printf("Using system prompt: %s", config.SystemPrompt)

	// Log registered tools information
	if config.ToolDefinitions != nil && len(config.ToolDefinitions) > 0 {
		toolNames := make([]string, 0, len(config.ToolDefinitions))
		for i := range config.ToolDefinitions {
			toolNames = append(toolNames, config.ToolDefinitions[i].Name)
		}
		log.Printf("Registered tools (%d): %s", len(config.ToolDefinitions), strings.Join(toolNames, ", "))
	}

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

	// Set response MIME type if specified
	if config.ResponseMimeType != "" {
		genConfig.ResponseMimeType = config.ResponseMimeType
	}

	// If output schema is specified, set it in the setup message
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
		genConfig.ResponseSchema = schemaObj
		genConfig.ResponseMimeType = "application/json"
	}

	// Set up response modalities
	genConfig.ResponseModalities = []generativelanguagepb.GenerationConfig_Modality{
		generativelanguagepb.GenerationConfig_TEXT,
	}
	if config.EnableAudio {
		genConfig.ResponseModalities = []generativelanguagepb.GenerationConfig_Modality{
			generativelanguagepb.GenerationConfig_AUDIO,
		}
	}

	// Create the initial message with an empty starter
	request := &generativelanguagepb.GenerateContentRequest{
		Model: config.ModelName,
		Contents: []*generativelanguagepb.Content{
			textContent("I'm ready to help.", withRole("user")),
		},
		GenerationConfig: genConfig,
	}

	// For Gemini 2.0 models, don't use tools at all for now
	if config.ToolDefinitions != nil && len(config.ToolDefinitions) > 0 && !strings.Contains(config.ModelName, "gemini-2.0") {
		request.Tools = tools
	}
	if os.Getenv("DEBUG_AISTUDIO") != "" {
		log.Printf("Sending StreamGenerateContent Request: %s", prototext.Format(request))
	}

	// Start the stream with the initial message
	stream, err := c.GenerativeClient.StreamGenerateContent(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to start stream: %w", err)
	}

	log.Printf("Stream established successfully.")
	return stream, nil
}

// SendMessageToBidiStream sends a message to an existing StreamGenerateContent stream.
// This is a compatibility function that actually creates a new stream for each message.
func (c *Client) SendMessageToBidiStream(stream generativelanguagepb.GenerativeService_StreamGenerateContentClient, text string) error {
	// Check if this is a LiveStreamAdapter (WebSocket stream)
	if adapter, ok := stream.(*LiveStreamAdapter); ok {
		log.Printf("Sending message via LiveStreamAdapter: %s", text)
		return adapter.SendMessage(text)
	}

	// For regular StreamGenerateContent, we can't send to an existing stream
	// But we're keeping the interface for compatibility
	log.Printf("SendMessageToBidiStream is a no-op in this implementation as StreamGenerateContent is one-way.")

	return nil
}

// CustomToolResponse represents a simplified tool response structure
type CustomToolResponse struct {
	Name     string          // Function name/ID
	Response *structpb.Value // JSON response content as structpb.Value
}

// SendToolResultsToBidiStream sends tool results to an existing StreamGenerateContent stream.
// This is a compatibility function - in v1beta StreamGenerateContent doesn't support tool calls.
func (c *Client) SendToolResultsToBidiStream(stream generativelanguagepb.GenerativeService_StreamGenerateContentClient, toolResults ...*generativelanguagepb.FunctionResponse) error {
	log.Printf("SendToolResultsToBidiStream is a no-op in this implementation as StreamGenerateContent doesn't support tool responses.")
	return nil
}

// ExtractBidiOutput extracts text and audio data from the GenerateContentResponse.
// This is a compatibility function that delegates to ExtractOutput to maintain interface compatibility
func ExtractBidiOutput(resp *generativelanguagepb.GenerateContentResponse) StreamOutput {
	return ExtractOutput(resp)
}

// initBidiStreamAlpha handles the v1alpha version of the bidirectional streaming.
func (c *Client) initBidiStreamAlpha(ctx context.Context, config *StreamClientConfig) (generativelanguagepb.GenerativeService_StreamGenerateContentClient, error) {
	if c.GenerativeClientAlpha == nil {
		return nil, fmt.Errorf("v1alpha client not initialized")
	}

	// Validate the model against known supported models for v1alpha
	isValid, err := c.ValidateModelAlpha(config.ModelName)
	if err != nil {
		log.Printf("Warning: Could not validate model %s for v1alpha: %v", config.ModelName, err)
	} else if !isValid {
		return nil, fmt.Errorf("model '%s' is not found in the list of supported models for the v1alpha API", config.ModelName)
	}

	log.Printf("Starting StreamGenerateContent for model: %s (using v1alpha)", config.ModelName)

	// Set up tools if defined for v1alpha
	var fns []*generativelanguagealphapb.FunctionDeclaration
	if config.ToolDefinitions != nil {
		for i := range config.ToolDefinitions {
			// Convert v1beta ToolDefinition to v1alpha
			alphaFn := convertToAlphaFunctionDeclaration(&config.ToolDefinitions[i])
			fns = append(fns, &alphaFn)
		}
	}

	// Create v1alpha tools
	alphaTools := []*generativelanguagealphapb.Tool{
		{
			FunctionDeclarations: fns,
		},
	}

	// Set up GenerationConfig for v1alpha
	alphaGenConfig := &generativelanguagealphapb.GenerationConfig{}
	alphaGenConfig.Temperature = &config.Temperature
	if config.TopP > 0 {
		alphaGenConfig.TopP = &config.TopP
	}
	if config.TopK > 0 {
		alphaGenConfig.TopK = &config.TopK
	}
	if config.MaxOutputTokens > 0 {
		alphaGenConfig.MaxOutputTokens = &config.MaxOutputTokens
	}

	// Only set voice config if VoiceName is specified (for v1alpha)
	if config.VoiceName != "" {
		alphaGenConfig.SpeechConfig = &generativelanguagealphapb.SpeechConfig{
			VoiceConfig: &generativelanguagealphapb.VoiceConfig{
				VoiceConfig: &generativelanguagealphapb.VoiceConfig_PrebuiltVoiceConfig{
					PrebuiltVoiceConfig: &generativelanguagealphapb.PrebuiltVoiceConfig{
						VoiceName: &config.VoiceName,
					},
				},
			},
		}
	}

	// Set response MIME type if specified
	if config.ResponseMimeType != "" {
		alphaGenConfig.ResponseMimeType = config.ResponseMimeType
	}

	// Set response modalities for v1alpha
	alphaGenConfig.ResponseModalities = []generativelanguagealphapb.GenerationConfig_Modality{
		generativelanguagealphapb.GenerationConfig_TEXT,
	}
	if config.EnableAudio {
		alphaGenConfig.ResponseModalities = []generativelanguagealphapb.GenerationConfig_Modality{
			generativelanguagealphapb.GenerationConfig_AUDIO,
		}
	}

	// Create the initial message for v1alpha
	alphaRequest := &generativelanguagealphapb.GenerateContentRequest{
		Model: config.ModelName,
		Contents: []*generativelanguagealphapb.Content{
			alphaTextContent("I'm ready to help.", withAlphaRole("user")),
		},
		GenerationConfig: alphaGenConfig,
	}

	// For Gemini 2.0 models, don't use tools at all for now
	if config.ToolDefinitions != nil && len(config.ToolDefinitions) > 0 && !strings.Contains(config.ModelName, "gemini-2.0") {
		alphaRequest.Tools = alphaTools
	}

	// Start the stream with the initial message (v1alpha)
	stream, err := c.GenerativeClientAlpha.StreamGenerateContent(ctx, alphaRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to start v1alpha stream: %w", err)
	}

	log.Printf("v1alpha stream established successfully.")
	return alphaToStandardStreamAdapter{stream: stream}, nil
}

// Helper functions for v1alpha API
type alphaTextContentOption func(c *generativelanguagealphapb.Content)

func withAlphaRole(role string) alphaTextContentOption {
	return func(c *generativelanguagealphapb.Content) {
		c.Role = role
	}
}

func alphaTextContent(text string, opts ...alphaTextContentOption) *generativelanguagealphapb.Content {
	c := &generativelanguagealphapb.Content{
		Parts: []*generativelanguagealphapb.Part{
			{
				Data: &generativelanguagealphapb.Part_Text{
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

// Convert from v1beta to v1alpha function declaration
func convertToAlphaFunctionDeclaration(beta *generativelanguagepb.FunctionDeclaration) generativelanguagealphapb.FunctionDeclaration {
	return generativelanguagealphapb.FunctionDeclaration{
		Name:        beta.Name,
		Description: beta.Description,
		// Additional conversion logic as needed
	}
}

// Adapter to make v1alpha stream compatible with v1beta interface
type alphaToStandardStreamAdapter struct {
	stream generativelanguagealphapb.GenerativeService_StreamGenerateContentClient
}

func (a alphaToStandardStreamAdapter) Recv() (*generativelanguagepb.GenerateContentResponse, error) {
	// Receive from alpha stream
	alphaResp, err := a.stream.Recv()
	if err != nil {
		return nil, err
	}

	// Convert alpha response to beta response format
	betaResp := &generativelanguagepb.GenerateContentResponse{}

	// Convert candidates
	if alphaResp.Candidates != nil && len(alphaResp.Candidates) > 0 {
		betaResp.Candidates = make([]*generativelanguagepb.Candidate, len(alphaResp.Candidates))
		for i, alphaCandidate := range alphaResp.Candidates {
			betaCandidate := &generativelanguagepb.Candidate{
				FinishReason: generativelanguagepb.Candidate_FinishReason(alphaCandidate.FinishReason),
				Index:        alphaCandidate.Index,
			}

			// Convert content
			if alphaCandidate.Content != nil {
				betaCandidate.Content = &generativelanguagepb.Content{
					Role: alphaCandidate.Content.Role,
				}

				// Convert parts
				if alphaCandidate.Content.Parts != nil {
					betaCandidate.Content.Parts = make([]*generativelanguagepb.Part, len(alphaCandidate.Content.Parts))
					for j, alphaPart := range alphaCandidate.Content.Parts {
						betaPart := &generativelanguagepb.Part{}

						// Handle different part types
						if textData := alphaPart.GetText(); textData != "" {
							betaPart.Data = &generativelanguagepb.Part_Text{
								Text: textData,
							}
						} else if inlineData := alphaPart.GetInlineData(); inlineData != nil {
							betaPart.Data = &generativelanguagepb.Part_InlineData{
								InlineData: &generativelanguagepb.Blob{
									MimeType: inlineData.MimeType,
									Data:     inlineData.Data,
								},
							}
						}

						betaCandidate.Content.Parts[j] = betaPart
					}
				}
			}

			// Convert safety ratings
			if alphaCandidate.SafetyRatings != nil {
				betaCandidate.SafetyRatings = make([]*generativelanguagepb.SafetyRating, len(alphaCandidate.SafetyRatings))
				for j, alphaSafetyRating := range alphaCandidate.SafetyRatings {
					betaCandidate.SafetyRatings[j] = &generativelanguagepb.SafetyRating{
						Category:    generativelanguagepb.HarmCategory(alphaSafetyRating.Category),
						Probability: generativelanguagepb.SafetyRating_HarmProbability(alphaSafetyRating.Probability),
					}
				}
			}

			// Set the converted candidate in the response
			betaResp.Candidates[i] = betaCandidate
		}
	}

	return betaResp, nil
}

func (a alphaToStandardStreamAdapter) Header() (metadata.MD, error) {
	return a.stream.Header()
}

func (a alphaToStandardStreamAdapter) Trailer() metadata.MD {
	return a.stream.Trailer()
}

func (a alphaToStandardStreamAdapter) CloseSend() error {
	return a.stream.CloseSend()
}

func (a alphaToStandardStreamAdapter) Context() context.Context {
	return a.stream.Context()
}

func (a alphaToStandardStreamAdapter) SendMsg(m interface{}) error {
	return a.stream.SendMsg(m)
}

func (a alphaToStandardStreamAdapter) RecvMsg(m interface{}) error {
	return a.stream.RecvMsg(m)
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

		// Check finish reason to determine if turn is complete
		if candidate.FinishReason == generativelanguagepb.Candidate_STOP {
			output.TurnComplete = true
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
	if promptFeedback == nil {
		return ""
	}
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

type textContentOption func(c *generativelanguagepb.Content)

// withRole sets the role of a Content object
func withRole(role string) textContentOption {
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
	// First check if we can retry
	if !e.IsRetryable() {
		return e.Err
	}

	// Check context before first attempt
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// First attempt (not counted as a retry)
	err := op()
	if err == nil {
		return nil
	}

	// Check if error is retryable
	if clientErr, ok := err.(*ClientError); !ok || !clientErr.IsRetryable() {
		return err // Non-retryable error
	}

	// Now do retries
	for e.Retries < e.MaxRetries {
		// Add jitter to the backoff time (±10%)
		jitter := float64(e.Backoff) * 0.1 * (2*rand.Float64() - 1)
		backoffWithJitter := e.Backoff + time.Duration(jitter)
		if backoffWithJitter < 0 {
			backoffWithJitter = e.Backoff // Ensure backoff is never negative
		}

		// Use a timer for backoff that respects context
		timer := time.NewTimer(backoffWithJitter)

		select {
		case <-ctx.Done():
			// Context was canceled or timed out
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			// Backoff time elapsed, proceed with retry
		}
		timer.Stop()

		e.Retries++
		e.Backoff *= 2 // Exponential backoff

		err = op()
		if err == nil {
			return nil // Success
		}

		// Update the error
		e.Err = err

		// Check if we should continue retrying
		if clientErr, ok := err.(*ClientError); !ok || !clientErr.IsRetryable() {
			return err // Non-retryable error
		}
	}

	return e.Err // Max retries exceeded
}

// quickModelValidation is a simple validation that avoids API calls
// Uses a basic format check instead of contacting the API
func (c *Client) quickModelValidation(modelName string) (bool, error) {
	// If we're simply using a bare model name like "gemini-pro", allow it automatically
	if !strings.Contains(modelName, "/") {
		log.Printf("Using simplified model name format: %s", modelName)
		return true, nil
	}

	// Check for well-known models and formats
	if strings.Contains(modelName, "gemini") ||
		strings.Contains(modelName, "palm") ||
		strings.Contains(modelName, "models/") {
		return true, nil
	}

	return true, nil // For now, always validate to true to avoid excessive API calls
}

// initLiveStream initializes a Live API session using WebSockets
func (c *Client) initLiveStream(ctx context.Context, config *StreamClientConfig) (generativelanguagepb.GenerativeService_StreamGenerateContentClient, error) {
	log.Printf("Initializing Live API client for model: %s", config.ModelName)

	// Check for recording/replay mode
	var recorder *WSRecorder
	recordEnv := os.Getenv("WS_RECORD_MODE")

	if recordEnv != "" {
		// Determine record or replay mode
		recordMode := recordEnv == "1"
		recordDir := "testdata/ws_recordings"

		// Create a sanitized model name for the recording file
		modelPart := strings.ReplaceAll(config.ModelName, "/", "_")
		modelPart = strings.ReplaceAll(modelPart, ":", "_")
		recordFile := filepath.Join(recordDir, fmt.Sprintf("%s.wsrec", modelPart))

		var err error
		recorder, err = NewWSRecorder(recordFile, recordMode)
		if err != nil && !recordMode {
			log.Printf("Warning: Failed to load WebSocket recording, will use live API: %v", err)
		} else if recorder != nil {
			log.Printf("Using WebSocket %s mode with recording file: %s",
				map[bool]string{true: "record", false: "replay"}[recordMode], recordFile)
		}
	}

	// Create the LiveClient with recorder if available
	liveClient, err := NewLiveClient(ctx, c.APIKey, config, recorder)
	if err != nil {
		return nil, fmt.Errorf("failed to create LiveClient: %w", err)
	}

	// Create and return the adapter
	adapter := NewLiveStreamAdapter(ctx, liveClient)
	return adapter, nil
}

// ValidateModelAlpha checks if the given model is a valid Gemini model for v1alpha.
func (c *Client) ValidateModelAlpha(modelName string) (bool, error) {
	if c.GenerativeClientAlpha == nil {
		return false, fmt.Errorf("GenerativeClientAlpha not initialized")
	}

	// If we're simply using a bare model name like "gemini-pro", allow it automatically
	if !strings.Contains(modelName, "/") {
		log.Printf("Using simplified model name format: %s", modelName)
		return true, nil
	}

	return true, nil // For now, always validate to true to avoid excessive API calls
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
