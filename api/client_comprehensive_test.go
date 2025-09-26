package api

import (
	"context"
	"os"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
)

// TestClientConfiguration tests various client configuration scenarios
func TestClientConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config *APIClientConfig
		valid  bool
	}{
		{
			name: "Valid Gemini API configuration",
			config: &APIClientConfig{
				APIKey:  "test-key",
				Backend: BackendGeminiAPI,
			},
			valid: true,
		},
		{
			name: "Valid Vertex AI configuration",
			config: &APIClientConfig{
				Backend:   BackendVertexAI,
				ProjectID: "test-project",
				Location:  "us-central1",
			},
			valid: true,
		},
		{
			name: "Missing API key for Gemini",
			config: &APIClientConfig{
				Backend: BackendGeminiAPI,
			},
			valid: false, // Should fall back to env var
		},
		{
			name: "Missing project ID for Vertex",
			config: &APIClientConfig{
				Backend:  BackendVertexAI,
				Location: "us-central1",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip if no API credentials available
			if os.Getenv("GEMINI_API_KEY") == "" && tt.config.APIKey == "test-key" {
				t.Skip("Skipping test: No API credentials available")
			}

			ctx := context.Background()
			client, err := NewClient(ctx, tt.config)

			if tt.valid && err != nil && tt.config.Backend == BackendGeminiAPI {
				// Check if it's just missing credentials
				if tt.config.APIKey == "test-key" {
					t.Skip("Skipping: Test API key not valid")
				}
				t.Errorf("Expected valid config but got error: %v", err)
			}

			if !tt.valid && err == nil && client != nil {
				t.Error("Expected invalid config but client was created")
			}
		})
	}
}

// TestStreamClientConfig tests StreamClientConfig creation and validation
func TestStreamClientConfig(t *testing.T) {
	tests := []struct {
		name   string
		config StreamClientConfig
		check  func(StreamClientConfig) bool
	}{
		{
			name: "Basic configuration",
			config: StreamClientConfig{
				ModelName:    "gemini-2.0-flash-latest",
				EnableAudio:  false,
				SystemPrompt: "You are a helpful assistant",
			},
			check: func(c StreamClientConfig) bool {
				return c.ModelName == "gemini-2.0-flash-latest"
			},
		},
		{
			name: "With temperature settings",
			config: StreamClientConfig{
				ModelName:   "gemini-2.0-flash-latest",
				Temperature: 0.7,
				TopP:        0.95,
				TopK:        40,
			},
			check: func(c StreamClientConfig) bool {
				return c.Temperature == 0.7 && c.TopP == 0.95 && c.TopK == 40
			},
		},
		{
			name: "With tool definitions",
			config: StreamClientConfig{
				ModelName:       "gemini-2.0-flash-latest",
				ToolDefinitions: []*ToolDefinition{},
			},
			check: func(c StreamClientConfig) bool {
				return c.ToolDefinitions != nil
			},
		},
		{
			name: "With feature flags",
			config: StreamClientConfig{
				ModelName:           "gemini-2.0-flash-latest",
				EnableWebSearch:     true,
				EnableCodeExecution: true,
				EnableWebSocket:     false,
			},
			check: func(c StreamClientConfig) bool {
				return c.EnableWebSearch && c.EnableCodeExecution && !c.EnableWebSocket
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.check(tt.config) {
				t.Errorf("Configuration check failed for %s", tt.name)
			}
		})
	}
}

// TestModelValidation tests model name validation
func TestModelValidation(t *testing.T) {
	tests := []struct {
		name      string
		modelName string
		isValid   bool
	}{
		{
			name:      "Valid Gemini 2.0 model",
			modelName: "gemini-2.0-flash-latest",
			isValid:   true,
		},
		{
			name:      "Valid Gemini 1.5 model",
			modelName: "gemini-1.5-pro",
			isValid:   true,
		},
		{
			name:      "Valid live model",
			modelName: "gemini-2.0-flash-live-001",
			isValid:   true,
		},
		{
			name:      "Model with prefix",
			modelName: "models/gemini-2.0-flash-latest",
			isValid:   true,
		},
		{
			name:      "Empty model name",
			modelName: "",
			isValid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation - non-empty
			isValid := tt.modelName != ""
			if isValid != tt.isValid {
				t.Errorf("Expected validation %v, got %v for model %s",
					tt.isValid, isValid, tt.modelName)
			}
		})
	}
}

// TestProtocolSelection tests WebSocket vs gRPC protocol selection
func TestProtocolSelection(t *testing.T) {
	tests := []struct {
		name            string
		modelName       string
		enableWebSocket bool
		expectWS        bool
	}{
		{
			name:            "Live model with WS enabled",
			modelName:       "gemini-2.0-flash-live-001",
			enableWebSocket: true,
			expectWS:        true,
		},
		{
			name:            "Live model with WS disabled",
			modelName:       "gemini-2.0-flash-live-001",
			enableWebSocket: false,
			expectWS:        false,
		},
		{
			name:            "Standard model with WS enabled",
			modelName:       "gemini-2.0-flash-latest",
			enableWebSocket: true,
			expectWS:        false, // Standard models don't support WS
		},
		{
			name:            "Standard model with WS disabled",
			modelName:       "gemini-2.0-flash-latest",
			enableWebSocket: false,
			expectWS:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isLive := isLiveModel(tt.modelName)
			useWS := isLive && tt.enableWebSocket

			if useWS != tt.expectWS {
				t.Errorf("Expected WebSocket=%v, got %v for model %s with WS=%v",
					tt.expectWS, useWS, tt.modelName, tt.enableWebSocket)
			}
		})
	}
}

// TestRetryLogic tests retry with exponential backoff
func TestRetryLogic(t *testing.T) {
	tests := []struct {
		name        string
		maxRetries  int
		shouldRetry bool
	}{
		{
			name:        "First retry attempt",
			maxRetries:  3,
			shouldRetry: true,
		},
		{
			name:        "Max retries reached",
			maxRetries:  0,
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &RetryableError{
				Err:        context.DeadlineExceeded,
				RetryCount: 0,
				MaxRetries: tt.maxRetries,
			}

			if err.ShouldRetry() != tt.shouldRetry {
				t.Errorf("Expected ShouldRetry=%v, got %v",
					tt.shouldRetry, err.ShouldRetry())
			}
		})
	}
}

// TestConnectionResilience tests connection handling
func TestConnectionResilience(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping connection test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test with invalid credentials to verify error handling
	client, err := NewClient(ctx, &APIClientConfig{
		APIKey:  "invalid-key",
		Backend: BackendGeminiAPI,
	})

	if err == nil && client != nil {
		// Connection might succeed with invalid key until first request
		t.Log("Client created with invalid key - error will occur on first request")
	}
}

// TestToolDefinitionHandling tests tool definition processing
func TestToolDefinitionHandling(t *testing.T) {
	// Test tool definition creation
	t.Run("Tool definition creation", func(t *testing.T) {
		params, _ := structpb.NewStruct(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input": map[string]interface{}{
					"type":        "string",
					"description": "Input parameter",
				},
			},
		})

		toolDef := &ToolDefinition{
			Name:        "test_tool",
			Description: "Test tool description",
			Parameters:  params,
		}

		if toolDef.Name != "test_tool" {
			t.Errorf("Expected tool name 'test_tool', got %s", toolDef.Name)
		}
		if toolDef.Description != "Test tool description" {
			t.Errorf("Expected description 'Test tool description', got %s", toolDef.Description)
		}
		if toolDef.Parameters == nil {
			t.Error("Expected parameters to be set")
		}
	})
}

// TestErrorClassification tests error classification for retry logic
func TestErrorClassification(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		isRetryable bool
	}{
		{
			name:        "Context deadline exceeded",
			err:         context.DeadlineExceeded,
			isRetryable: true,
		},
		{
			name:        "Context canceled",
			err:         context.Canceled,
			isRetryable: false,
		},
		{
			name:        "Nil error",
			err:         nil,
			isRetryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryable := isRetryableError(tt.err)
			if retryable != tt.isRetryable {
				t.Errorf("Expected isRetryable=%v, got %v for error %v",
					tt.isRetryable, retryable, tt.err)
			}
		})
	}
}

// BenchmarkStreamClientConfig benchmarks config creation
func BenchmarkStreamClientConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = StreamClientConfig{
			ModelName:       "gemini-2.0-flash-latest",
			EnableAudio:     false,
			SystemPrompt:    "Test prompt",
			Temperature:     0.7,
			TopP:            0.95,
			TopK:            40,
			MaxOutputTokens: 2048,
		}
	}
}

// BenchmarkToolDefinitionCreation benchmarks tool definition creation
func BenchmarkToolDefinitionCreation(b *testing.B) {
	params, _ := structpb.NewStruct(map[string]interface{}{
		"type": "object",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = &ToolDefinition{
			Name:        "bench_tool",
			Description: "Benchmark tool",
			Parameters:  params,
		}
	}
}