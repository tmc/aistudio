package api

import (
	"context"
	"errors"
	"testing"
	"time"

	generativelanguagealphapb "cloud.google.com/go/ai/generativelanguage/apiv1alpha/generativelanguagepb"
	"cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
	"google.golang.org/grpc/metadata"
)

// Define a mock alpha stream for testing
type mockAlphaStream struct {
	responses []*generativelanguagealphapb.GenerateContentResponse
	index     int
	closed    bool
	err       error
}

func (m *mockAlphaStream) Recv() (*generativelanguagealphapb.GenerateContentResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.index >= len(m.responses) {
		return nil, errors.New("end of stream")
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

func (m *mockAlphaStream) Header() (metadata.MD, error) {
	return metadata.MD{}, nil
}

func (m *mockAlphaStream) Trailer() metadata.MD {
	return metadata.MD{}
}

func (m *mockAlphaStream) CloseSend() error {
	m.closed = true
	return nil
}

func (m *mockAlphaStream) Context() context.Context {
	return context.Background()
}

func (m *mockAlphaStream) SendMsg(interface{}) error {
	return nil
}

func (m *mockAlphaStream) RecvMsg(interface{}) error {
	return nil
}

func TestAlphaToStandardStreamAdapter(t *testing.T) {
	// Create a mock alpha stream with test responses
	mockStream := &mockAlphaStream{
		responses: []*generativelanguagealphapb.GenerateContentResponse{
			{
				Candidates: []*generativelanguagealphapb.Candidate{
					{
						Content: &generativelanguagealphapb.Content{
							Parts: []*generativelanguagealphapb.Part{
								{
									Data: &generativelanguagealphapb.Part_Text{
										Text: "Hello from alpha",
									},
								},
							},
						},
						SafetyRatings: []*generativelanguagealphapb.SafetyRating{
							{
								Category:    generativelanguagealphapb.HarmCategory_HARM_CATEGORY_DANGEROUS_CONTENT,
								Probability: generativelanguagealphapb.SafetyRating_HIGH,
							},
						},
					},
				},
			},
		},
	}

	// Create the adapter
	adapter := alphaToStandardStreamAdapter{stream: mockStream}

	// Test Recv method
	resp, err := adapter.Recv()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check conversion
	if len(resp.Candidates) != 1 {
		t.Fatalf("Expected 1 candidate, got %d", len(resp.Candidates))
	}

	// Check text content
	candidate := resp.Candidates[0]
	if len(candidate.Content.Parts) != 1 {
		t.Fatalf("Expected 1 part, got %d", len(candidate.Content.Parts))
	}
	text := candidate.Content.Parts[0].GetText()
	if text != "Hello from alpha" {
		t.Errorf("Expected text 'Hello from alpha', got '%s'", text)
	}

	// Check safety ratings
	if len(candidate.SafetyRatings) != 1 {
		t.Fatalf("Expected 1 safety rating, got %d", len(candidate.SafetyRatings))
	}
	rating := candidate.SafetyRatings[0]
	if rating.Category != generativelanguagepb.HarmCategory_HARM_CATEGORY_DANGEROUS_CONTENT {
		t.Errorf("Expected category HARM_CATEGORY_DANGEROUS_CONTENT, got %v", rating.Category)
	}
	if rating.Probability != generativelanguagepb.SafetyRating_HIGH {
		t.Errorf("Expected probability HIGH, got %v", rating.Probability)
	}

	// Test error propagation
	mockStream.err = errors.New("test error")
	_, err = adapter.Recv()
	if err == nil || err.Error() != "test error" {
		t.Errorf("Expected 'test error', got %v", err)
	}

	// Test other methods
	adapter.CloseSend()
	if !mockStream.closed {
		t.Error("CloseSend didn't close the underlying stream")
	}

	// Test context
	ctx := adapter.Context()
	if ctx == nil {
		t.Error("Context returned nil")
	}
}

func TestRetryableErrorRetry(t *testing.T) {
	originalErr := &ClientError{
		Code:    500,
		Message: "internal server error",
	}

	retryErr := &RetryableError{
		Err:        originalErr,
		Retries:    0,
		MaxRetries: 3,
		Backoff:    10 * time.Millisecond,
	}

	// Test successful retry
	attempts := 0
	maxAttempts := 2
	op := func() error {
		attempts++
		if attempts < maxAttempts {
			return &ClientError{
				Code:    500,
				Message: "still failing",
			}
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := retryErr.Retry(ctx, op)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if attempts != maxAttempts {
		t.Errorf("Expected %d attempts, got %d", maxAttempts, attempts)
	}

	// Check updated retry count and backoff
	if retryErr.Retries != maxAttempts-1 {
		t.Errorf("Expected retries to be %d, got %d", maxAttempts-1, retryErr.Retries)
	}
	if retryErr.Backoff < 10*time.Millisecond {
		t.Errorf("Expected backoff to be increased, got %v", retryErr.Backoff)
	}

	// Test with non-retryable error
	retryErr = &RetryableError{
		Err:        originalErr,
		Retries:    0,
		MaxRetries: 3,
		Backoff:    10 * time.Millisecond,
	}

	nonRetryOp := func() error {
		return &ClientError{
			Code:    400,
			Message: "bad request",
		}
	}

	err = retryErr.Retry(ctx, nonRetryOp)
	if err == nil || err.Error() != "Client Error: 400 - bad request" {
		t.Errorf("Expected 'bad request' error, got %v", err)
	}

	// Test with context cancellation
	ctxCanceled, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = retryErr.Retry(ctxCanceled, op)
	if err == nil || err.Error() != "context canceled" {
		t.Errorf("Expected 'context canceled' error, got %v", err)
	}

	// Test with max retries exceeded
	retryErr.Retries = retryErr.MaxRetries
	err = retryErr.Retry(ctx, op)
	if err != originalErr {
		t.Errorf("Expected original error, got %v", err)
	}
}
