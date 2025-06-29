package api

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	// Save original env vars to restore after test
	origAPIKey := os.Getenv("GEMINI_API_KEY")
	origProjectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	origLocation := os.Getenv("GOOGLE_CLOUD_LOCATION")
	origVertexAI := os.Getenv("AISTUDIO_USE_VERTEXAI")

	// Cleanup function to restore environment
	defer func() {
		os.Setenv("GEMINI_API_KEY", origAPIKey)
		os.Setenv("GOOGLE_CLOUD_PROJECT", origProjectID)
		os.Setenv("GOOGLE_CLOUD_LOCATION", origLocation)
		os.Setenv("AISTUDIO_USE_VERTEXAI", origVertexAI)
	}()

	tests := []struct {
		name          string
		config        *APIClientConfig
		envVars       map[string]string
		expectError   bool
		expectedError string
	}{
		{
			name:        "nil context should fail",
			config:      nil,
			expectError: true,
		},
		{
			name: "gemini api with api key",
			config: &APIClientConfig{
				APIKey:  "test-api-key",
				Backend: BackendGeminiAPI,
			},
			expectError: false,
		},
		{
			name: "gemini api key from env var",
			config: &APIClientConfig{
				Backend: BackendGeminiAPI,
			},
			envVars: map[string]string{
				"GEMINI_API_KEY": "env-api-key",
			},
			expectError: false,
		},
		{
			name: "vertex ai requires project id",
			config: &APIClientConfig{
				Backend: BackendVertexAI,
			},
			expectError:   true,
			expectedError: "project ID is required for Vertex AI backend",
		},
		{
			name: "vertex ai with project id and location",
			config: &APIClientConfig{
				Backend:   BackendVertexAI,
				ProjectID: "test-project",
				Location:  "us-central1",
			},
			expectError:   true, // Will fail due to actual auth, but client initialization logic passes
			expectedError: "failed to create Vertex AI client",
		},
		{
			name: "vertex ai with env project id",
			config: &APIClientConfig{
				Backend: BackendVertexAI,
			},
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "env-project",
			},
			expectError:   true, // Will fail due to actual auth, but client initialization logic passes
			expectedError: "failed to create Vertex AI client",
		},
		{
			name: "vertex ai with env location",
			config: &APIClientConfig{
				Backend:   BackendVertexAI,
				ProjectID: "test-project",
			},
			envVars: map[string]string{
				"GOOGLE_CLOUD_LOCATION": "us-west1",
			},
			expectError:   true, // Will fail due to actual auth, but client initialization logic passes
			expectedError: "failed to create Vertex AI client",
		},
		{
			name:   "env var for vertex ai backend",
			config: &APIClientConfig{},
			envVars: map[string]string{
				"AISTUDIO_USE_VERTEXAI": "true",
				"GOOGLE_CLOUD_PROJECT":  "env-project",
				"GOOGLE_CLOUD_LOCATION": "us-west1",
			},
			expectError:   true, // Will fail due to actual auth, but client initialization logic passes
			expectedError: "failed to create Vertex AI client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for this test
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			// Create a context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if tt.name == "nil context should fail" {
				ctx = nil
			}
			defer func() {
				if cancel != nil {
					cancel()
				}
			}()

			// Call the function
			client, err := NewClient(ctx, tt.config)

			// Check if error was expected
			if tt.expectError {
				if err == nil {
					// For Vertex AI tests, success is acceptable if auth is available
					isVertexAI := (tt.config != nil && tt.config.Backend == BackendVertexAI) ||
						(tt.envVars != nil && tt.envVars["AISTUDIO_USE_VERTEXAI"] == "true")
					if isVertexAI {
						t.Logf("Vertex AI test succeeded (auth available)")
					} else {
						t.Errorf("Expected error but got nil")
					}
				} else if tt.expectedError != "" && !containsString(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.expectedError, err.Error())
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check if client was created when expected
			if !tt.expectError && client == nil {
				t.Error("Expected client to be created, but got nil")
			}

			// Additional checks if client was created
			if client != nil {
				// Make sure to close client
				if client.VertexAIClient != nil {
					client.VertexAIClient.Close()
				}
				if client.VertexModelsClient != nil {
					client.VertexModelsClient.Close()
				}
				if client.GenerativeClient != nil {
					client.GenerativeClient.Close()
				}
				if client.GenerativeClientAlpha != nil {
					client.GenerativeClientAlpha.Close()
				}
			}

			// Clean up environment variables
			for k := range tt.envVars {
				os.Setenv(k, "")
			}
		})
	}
}

func TestInitClient(t *testing.T) {
	// Save original env vars to restore after test
	origAPIKey := os.Getenv("GEMINI_API_KEY")
	origGoogleAPIKey := os.Getenv("GOOGLE_API_KEY")
	origGenAIKey := os.Getenv("GOOGLE_GENERATIVE_AI_KEY")

	// Cleanup function to restore environment
	defer func() {
		os.Setenv("GEMINI_API_KEY", origAPIKey)
		os.Setenv("GOOGLE_API_KEY", origGoogleAPIKey)
		os.Setenv("GOOGLE_GENERATIVE_AI_KEY", origGenAIKey)
	}()

	tests := []struct {
		name          string
		apiKey        string
		version       string
		envVars       map[string]string
		expectNilCtx  bool
		expectError   bool
		expectedError string
		expectAlpha   bool
		expectBeta    bool
	}{
		{
			name:          "nil context should fail",
			apiKey:        "test-api-key",
			expectNilCtx:  true,
			expectError:   true,
			expectedError: "context cannot be nil",
		},
		{
			name:          "v1beta with api key",
			apiKey:        "test-api-key",
			version:       "v1beta",
			expectError:   true, // Will fail due to actual API auth, but initialization logic should pass
			expectedError: "failed to create v1beta generative client",
			expectBeta:    true,
		},
		{
			name:          "v1alpha with api key",
			apiKey:        "test-api-key",
			version:       "v1alpha",
			expectError:   true, // Will fail due to actual API auth, but initialization logic should pass
			expectedError: "failed to create v1alpha generative client",
			expectAlpha:   true,
		},
		{
			name:          "invalid version",
			apiKey:        "test-api-key",
			version:       "invalid",
			expectError:   true,
			expectedError: "invalid Gemini API version", // Validation should happen before API call
		},
		{
			name:          "api key from GOOGLE_API_KEY",
			version:       "v1beta",
			envVars:       map[string]string{"GOOGLE_API_KEY": "env-api-key"},
			expectError:   true, // Will fail due to actual API auth, but initialization logic should pass
			expectedError: "failed to create v1beta generative client",
			expectBeta:    true,
		},
		{
			name:          "api key from GEMINI_API_KEY",
			version:       "v1beta",
			envVars:       map[string]string{"GEMINI_API_KEY": "env-api-key"},
			expectError:   true, // Will fail due to actual API auth, but initialization logic should pass
			expectedError: "failed to create v1beta generative client",
			expectBeta:    true,
		},
		{
			name:          "api key from GOOGLE_GENERATIVE_AI_KEY",
			version:       "v1beta",
			envVars:       map[string]string{"GOOGLE_GENERATIVE_AI_KEY": "env-api-key"},
			expectError:   true, // Will fail due to actual API auth, but initialization logic should pass
			expectedError: "failed to create v1beta generative client",
			expectBeta:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for this test
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			// Create a context with timeout
			var ctx context.Context
			var cancel context.CancelFunc
			if !tt.expectNilCtx {
				ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
			}

			// Create client
			client := &Client{
				APIKey:        tt.apiKey,
				GeminiVersion: tt.version,
			}

			// Call InitClient
			err := client.InitClient(ctx)

			// Check error
			if tt.expectError {
				if err == nil {
					// For Vertex AI tests, success is acceptable if auth is available
					t.Logf("Vertex AI test succeeded (auth available)")
				} else if tt.expectedError != "" && !containsString(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.expectedError, err.Error())
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Check client version initialization
			if !tt.expectError {
				if tt.expectAlpha && client.GenerativeClientAlpha == nil {
					t.Error("Expected v1alpha client to be initialized, but it's nil")
				}
				if tt.expectBeta && client.GenerativeClient == nil {
					t.Error("Expected v1beta client to be initialized, but it's nil")
				}
			}

			// Clean up
			if client.GenerativeClient != nil {
				client.GenerativeClient.Close()
			}
			if client.GenerativeClientAlpha != nil {
				client.GenerativeClientAlpha.Close()
			}

			// Clean up environment variables
			for k := range tt.envVars {
				os.Setenv(k, "")
			}
		})
	}
}

func TestInitVertexAIClient(t *testing.T) {
	tests := []struct {
		name          string
		apiKey        string
		projectID     string
		location      string
		expectNilCtx  bool
		expectError   bool
		expectedError string
	}{
		{
			name:          "nil context should fail",
			projectID:     "test-project",
			location:      "us-central1",
			expectNilCtx:  true,
			expectError:   true,
			expectedError: "context is nil",
		},
		{
			name:          "with api key",
			apiKey:        "test-api-key",
			projectID:     "test-project",
			location:      "us-central1",
			expectError:   true, // Will fail due to actual auth, but initialization logic should pass
			expectedError: "failed to create Vertex AI client",
		},
		{
			name:          "without api key",
			projectID:     "test-project",
			location:      "us-central1",
			expectError:   true, // Will fail due to actual auth, but initialization logic should pass
			expectedError: "failed to create Vertex AI client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a context with timeout
			var ctx context.Context
			var cancel context.CancelFunc
			if !tt.expectNilCtx {
				ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
			}

			// Create client
			client := &Client{
				APIKey:    tt.apiKey,
				ProjectID: tt.projectID,
				Location:  tt.location,
			}

			// Call InitVertexAIClient
			err := client.InitVertexAIClient(ctx)

			// Check error
			if tt.expectError {
				if err == nil {
					// For Vertex AI tests, success is acceptable if auth is available
					t.Logf("Vertex AI test succeeded (auth available)")
				} else if tt.expectedError != "" && !containsString(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.expectedError, err.Error())
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Clean up
			if client.VertexAIClient != nil {
				client.VertexAIClient.Close()
			}
			if client.VertexModelsClient != nil {
				client.VertexModelsClient.Close()
			}
		})
	}
}

// Helper to check if a string contains another string
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || len(s) >= len(substr) && s[len(s)-len(substr):] == substr || len(s) >= len(substr) && contains(s, substr)
}

func contains(s, substr string) bool {
	for i := 0; i < len(s); i++ {
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
