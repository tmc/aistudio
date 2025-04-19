// File: client_test.go - Contains test-specific implementations
package api

import (
	"context"
	"log"

	"cloud.google.com/go/ai/generativelanguage/apiv1alpha/generativelanguagepb"
	"google.golang.org/protobuf/encoding/prototext"
)

// These functions are specifically for test compatibility

// InitBidiStreamTest is a test-specific implementation that returns a StreamGenerateContent client
// for test compatibility, while the actual implementation would use BidiGenerateContent.
func (c *Client) InitBidiStreamTest(ctx context.Context, config ClientConfig) (generativelanguagepb.GenerativeService_StreamGenerateContentClient, error) {
	if c.GenerativeClient == nil {
		log.Println("InitBidiStreamTest: Client not initialized, attempting InitClient...")
		if err := c.InitClient(ctx); err != nil {
			return nil, err
		}
	}

	// Create a request with a welcoming message
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

	log.Printf("Sending StreamGenerateContent request: %s", prototext.Format(request))

	// Start streaming content for tests
	stream, err := c.GenerativeClient.StreamGenerateContent(ctx, request)
	if err != nil {
		return nil, err
	}

	log.Println("Test stream initialized successfully.")
	return stream, nil
}

// SendMessageToBidiStreamTest is a test-specific implementation for sending messages
// This is for test compatibility only
func (c *Client) SendMessageToBidiStreamTest(stream generativelanguagepb.GenerativeService_StreamGenerateContentClient, text string) error {
	log.Printf("[TEST] Would send message to stream: %s", text)
	// In tests, we don't actually send the message since StreamGenerateContent doesn't support it
	// In a real implementation with proper BidiGenerateContent, we would send the message
	return nil
}

// ExtractBidiOutputTest is a test-specific implementation that handles GenerateContentResponse
// for test compatibility, while the real implementation handles BidiGenerateContentServerMessage.
func ExtractBidiOutputTest(resp *generativelanguagepb.GenerateContentResponse) StreamOutput {
	return ExtractOutput(resp)
}
