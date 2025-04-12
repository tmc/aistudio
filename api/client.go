package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
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

// ClientConfig holds configuration passed during stream initialization.
type ClientConfig struct {
	ModelName   string
	EnableAudio bool   // Note: v1alpha Bidi setup has limited audio config
	VoiceName   string // Informational, may not be directly usable in v1alpha setup
}

// StreamOutput holds the processed output from a stream response chunk.
type StreamOutput struct {
	Text  string
	Audio []byte // Raw audio data (PCM S16LE, 24kHz expected if audio is generated)

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

	// Create the content request with a welcoming message
	request := &generativelanguagepb.GenerateContentRequest{
		Model: config.ModelName,
		Contents: []*generativelanguagepb.Content{
			{
				Parts: []*generativelanguagepb.Part{
					{
						Data: &generativelanguagepb.Part_Text{
							Text: "Hello, I'm ready to chat. How can I help you today?",
						},
					},
				},
				Role: "user",
			},
		},
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

	// Send setup message to the server:
	request := &generativelanguagepb.BidiGenerateContentClientMessage{
		MessageType: &generativelanguagepb.BidiGenerateContentClientMessage_Setup{
			Setup: &generativelanguagepb.BidiGenerateContentSetup{
				Model: config.ModelName,
				// Note: Audio config is not fully supported in v1alpha
				// AudioConfig: &generativelanguagepb.AudioConfig{
				//	Voice: config.VoiceName,
				// },
			},
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

	request := &generativelanguagepb.BidiGenerateContentClientMessage{
		MessageType: &generativelanguagepb.BidiGenerateContentClientMessage_ClientContent{
			ClientContent: &generativelanguagepb.BidiGenerateContentClientContent{
				Turns: []*generativelanguagepb.Content{
					{
						Parts: []*generativelanguagepb.Part{
							{
								Data: &generativelanguagepb.Part_Text{
									Text: text,
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
		log.Printf("Sending Raw Bidi Request: %s", prototext.Format(request))
	}
	if err := stream.Send(request); err != nil {
		return fmt.Errorf("failed to send message to stream: %w", err)
	}
	log.Printf("Message sent to bidirectional stream successfully.")
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
		// // todo feedback message handling
		// if feedback := turn.GetPromptFeedback(); feedback != nil {
		// todo: turncomplete handling
		// todo: handle feedback
		// todo: handle groundingfeedback
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
	if output.Text == "" && len(output.Audio) == 0 {
		log.Println("Received response chunk contained no processable text or audio.")
	}
	output.Text = strings.TrimSpace(output.Text)
	if output.Audio != nil {
		log.Printf("Extracted audio data, %d bytes", len(output.Audio))
	} else {
		log.Println("No audio data extracted.")
	}
	if output.Text != "" {
		log.Printf("Extracted text data: %q", output.Text)
	}
	if output.Audio != nil {
		log.Printf("Extracted audio data, %d bytes", len(output.Audio))
	}
	if os.Getenv("DEBUG_AISTUDIO") != "" {
		log.Printf("Final Extracted Output: %q", output.Text)
		log.Printf("Final Extracted Audio Data: %d bytes", len(output.Audio))
	}
	// Trim whitespace from the text output
	output.Text = strings.TrimSpace(output.Text)
	if output.Text == "" && len(output.Audio) == 0 {
		log.Println("Received response chunk contained no processable text or audio.")
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

	if output.Text == "" && len(output.Audio) == 0 {
		log.Println("Received response chunk contained no processable text or audio.")
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
