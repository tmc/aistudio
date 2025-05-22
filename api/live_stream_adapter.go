package api

import (
	"context"
	"fmt"
	"io"
	"log"

	"cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// LiveClientInterface defines the interface for WebSocket clients
type LiveClientInterface interface {
	SendMessage(message string) error
	ReceiveMessage() (*StreamOutput, error)
	Close() error
	Initialize() error
}

// LiveStreamAdapter adapts the LiveClient to implement the
// StreamGenerateContentClient interface for compatibility with existing code.
type LiveStreamAdapter struct {
	client       LiveClientInterface
	ctx          context.Context
	responsesCh  chan *StreamOutput
	errorCh      chan error
	responses    []*generativelanguagepb.GenerateContentResponse
	currentIndex int
	closed       bool
}

// NewLiveStreamAdapter creates a new adapter for the LiveClient.
func NewLiveStreamAdapter(ctx context.Context, client LiveClientInterface) *LiveStreamAdapter {
	adapter := &LiveStreamAdapter{
		client:      client,
		ctx:         ctx,
		responsesCh: make(chan *StreamOutput, 10),
		errorCh:     make(chan error, 1),
		responses:   make([]*generativelanguagepb.GenerateContentResponse, 0),
	}

	// Initialize the client if needed
	if err := client.Initialize(); err != nil {
		log.Printf("Failed to initialize LiveClient: %v", err)
		adapter.errorCh <- err
	}

	return adapter
}

// Recv implements the StreamGenerateContentClient interface.
func (a *LiveStreamAdapter) Recv() (*generativelanguagepb.GenerateContentResponse, error) {
	// Check if we've already closed
	if a.closed {
		return nil, io.EOF
	}

	// Check if we have an error
	select {
	case err := <-a.errorCh:
		return nil, err
	default:
		// Continue
	}

	// Check if we have a cached response
	if a.currentIndex < len(a.responses) {
		resp := a.responses[a.currentIndex]
		a.currentIndex++
		return resp, nil
	}

	// Get a new response from the LiveClient
	output, err := a.client.ReceiveMessage()
	if err != nil {
		log.Printf("Error receiving message from LiveClient: %v", err)
		return nil, err
	}

	// Convert the output to a GenerateContentResponse
	resp := convertOutputToResponse(output)
	a.responses = append(a.responses, resp)
	a.currentIndex++

	log.Printf("LiveStreamAdapter.Recv: got response: %v", resp)
	return resp, nil
}

// Header returns the header metadata for this stream.
func (a *LiveStreamAdapter) Header() (metadata.MD, error) {
	return metadata.MD{}, nil
}

// Trailer returns the trailer metadata for this stream.
func (a *LiveStreamAdapter) Trailer() metadata.MD {
	return metadata.MD{}
}

// Context returns the context for this stream.
func (a *LiveStreamAdapter) Context() context.Context {
	return a.ctx
}

// CloseSend closes the sending side of the stream.
func (a *LiveStreamAdapter) CloseSend() error {
	if a.closed {
		return nil
	}
	a.closed = true
	return a.client.Close()
}

// SendMessage sends a message through the LiveClient.
func (a *LiveStreamAdapter) SendMessage(message string) error {
	if a.closed {
		return fmt.Errorf("stream is closed")
	}
	return a.client.SendMessage(message)
}

// RecvMsg receives a message and stores it into m.
func (a *LiveStreamAdapter) RecvMsg(m interface{}) error {
	resp, err := a.Recv()
	if err != nil {
		return err
	}

	// Try to set the message
	protoMsg, ok := m.(proto.Message)
	if !ok {
		return fmt.Errorf("cannot convert message to proto.Message")
	}

	// Copy from resp to protoMsg
	respMsg, ok := protoMsg.(*generativelanguagepb.GenerateContentResponse)
	if !ok {
		return fmt.Errorf("message is not a GenerateContentResponse")
	}

	// Copy fields from resp to respMsg
	proto.Reset(respMsg)
	respBytes, err := proto.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %v", err)
	}

	if err := proto.Unmarshal(respBytes, respMsg); err != nil {
		return fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return nil
}

// SendMsg sends a message through the LiveClient.
func (a *LiveStreamAdapter) SendMsg(m interface{}) error {
	// This is not used in the current implementation
	return fmt.Errorf("SendMsg not implemented")
}

// convertOutputToResponse converts a StreamOutput to a GenerateContentResponse.
func convertOutputToResponse(output *StreamOutput) *generativelanguagepb.GenerateContentResponse {
	if output == nil {
		return &generativelanguagepb.GenerateContentResponse{}
	}

	// Create a simple response with a part containing the text
	part := &generativelanguagepb.Part{}
	part.Data = &generativelanguagepb.Part_Text{
		Text: output.Text,
	}

	content := &generativelanguagepb.Content{
		Role:  "model",
		Parts: []*generativelanguagepb.Part{part},
	}

	// Determine finish reason
	var finishReason generativelanguagepb.Candidate_FinishReason
	if output.TurnComplete {
		finishReason = generativelanguagepb.Candidate_STOP
	} else {
		finishReason = generativelanguagepb.Candidate_FINISH_REASON_UNSPECIFIED
	}

	// Set index
	var index int32 = 0

	// Create candidate
	candidate := &generativelanguagepb.Candidate{
		Content:      content,
		FinishReason: finishReason,
		Index:        &index,
	}

	// Create response
	resp := &generativelanguagepb.GenerateContentResponse{
		Candidates: []*generativelanguagepb.Candidate{candidate},
	}

	// We'll skip usage metadata for now as it causes type issues
	// The actual response content will still work correctly

	return resp
}
