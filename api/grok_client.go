package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Grok API constants
const (
	GrokAPIBaseURL = "https://api.x.ai/v1"
)

// GrokMessage represents a message in the Grok API format
type GrokMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GrokChatRequest represents a request to the Grok API
type GrokChatRequest struct {
	Model       string        `json:"model"`
	Messages    []GrokMessage `json:"messages"`
	Stream      bool          `json:"stream,omitempty"`
	Temperature *float32      `json:"temperature,omitempty"`
	MaxTokens   *int32        `json:"max_tokens,omitempty"`
}

// GrokChatResponse represents a response from the Grok API
type GrokChatResponse struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []GrokResponseChoice `json:"choices"`
	Usage   *GrokUsage           `json:"usage,omitempty"`
}

// GrokResponseChoice represents a choice in the response
type GrokResponseChoice struct {
	Index        int                `json:"index"`
	Message      *GrokMessage       `json:"message,omitempty"`
	Delta        *GrokMessage       `json:"delta,omitempty"`
	FinishReason *string            `json:"finish_reason,omitempty"`
}

// GrokUsage represents token usage information
type GrokUsage struct {
	PromptTokens     int32 `json:"prompt_tokens"`
	CompletionTokens int32 `json:"completion_tokens"`
	TotalTokens      int32 `json:"total_tokens"`
}

// GrokStreamChunk represents a single chunk in a streaming response
type GrokStreamChunk struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []GrokResponseChoice `json:"choices"`
}

// InitGrokClient initializes the HTTP client for Grok API
func (c *Client) InitGrokClient(ctx context.Context) error {
	if c.APIKey == "" {
		return fmt.Errorf("API key is required for Grok API")
	}

	// Create HTTP client with custom transport if provided
	transport := c.httpTransport
	if transport == nil {
		transport = http.DefaultTransport
	}

	c.GrokHTTPClient = &http.Client{
		Transport: transport,
	}

	return nil
}

// GrokChatCompletion sends a chat completion request to the Grok API
func (c *Client) GrokChatCompletion(ctx context.Context, req *GrokChatRequest) (*GrokChatResponse, error) {
	if c.GrokHTTPClient == nil {
		return nil, fmt.Errorf("Grok client not initialized")
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", GrokAPIBaseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.GrokHTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var grokResp GrokChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&grokResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &grokResp, nil
}

// GrokChatStream sends a streaming chat completion request to the Grok API
func (c *Client) GrokChatStream(ctx context.Context, req *GrokChatRequest) (<-chan *GrokStreamChunk, <-chan error) {
	chunkChan := make(chan *GrokStreamChunk)
	errChan := make(chan error, 1)

	go func() {
		defer close(chunkChan)
		defer close(errChan)

		if c.GrokHTTPClient == nil {
			errChan <- fmt.Errorf("Grok client not initialized")
			return
		}

		// Enable streaming
		req.Stream = true

		jsonData, err := json.Marshal(req)
		if err != nil {
			errChan <- fmt.Errorf("failed to marshal request: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", GrokAPIBaseURL+"/chat/completions", bytes.NewBuffer(jsonData))
		if err != nil {
			errChan <- fmt.Errorf("failed to create request: %w", err)
			return
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
		httpReq.Header.Set("Accept", "text/event-stream")

		resp, err := c.GrokHTTPClient.Do(httpReq)
		if err != nil {
			errChan <- fmt.Errorf("failed to send request: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			errChan <- fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
			return
		}

		// Read streaming response
		reader := resp.Body
		buf := make([]byte, 4096)
		var remaining string

		for {
			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			default:
			}

			n, err := reader.Read(buf)
			if n > 0 {
				data := remaining + string(buf[:n])
				lines := strings.Split(data, "\n")
				
				// Keep the last incomplete line for next iteration
				remaining = lines[len(lines)-1]
				lines = lines[:len(lines)-1]

				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line == "" || line == "data: [DONE]" {
						continue
					}
					
					if strings.HasPrefix(line, "data: ") {
						jsonData := strings.TrimPrefix(line, "data: ")
						
						var chunk GrokStreamChunk
						if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
							// Log error but continue processing
							continue
						}
						
						select {
						case chunkChan <- &chunk:
						case <-ctx.Done():
							errChan <- ctx.Err()
							return
						}
					}
				}
			}

			if err != nil {
				if err == io.EOF {
					break
				}
				errChan <- fmt.Errorf("failed to read response: %w", err)
				return
			}
		}
	}()

	return chunkChan, errChan
}

// ConvertMessagesToGrok converts internal messages to Grok API format
// Note: This would typically import the Message type from the main package
func ConvertMessagesToGrok(messages interface{}) []GrokMessage {
	// This is a placeholder - in practice, this would be implemented
	// when integrated with the main package's Message type
	return []GrokMessage{}
}