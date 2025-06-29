package aistudio

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/tmc/aistudio/api"
)

// WithAPIKey sets the Google API Key for the client.
func WithAPIKey(key string) Option {
	return func(m *Model) error {
		m.apiKey = key
		if m.client == nil {
			m.client = &api.Client{}
		}
		m.client.APIKey = key
		return nil
	}
}

// WithBackend explicitly sets the backend type (Gemini API or Vertex AI).
func WithBackend(backend BackendType) Option {
	return func(m *Model) error {
		m.backend = backend
		return nil
	}
}

// WithVertexAI enables the use of Vertex AI instead of Gemini API.
func WithVertexAI(enabled bool, projectID string, location string) Option {
	return func(m *Model) error {
		if enabled {
			m.backend = BackendVertexAI
		} else {
			m.backend = BackendGeminiAPI
		}
		m.projectID = projectID
		m.location = location
		return nil
	}
}

// WithGrok enables the use of xAI Grok API.
func WithGrok(enabled bool) Option {
	return func(m *Model) error {
		if enabled {
			m.backend = BackendGrok
		} else {
			m.backend = BackendGeminiAPI
		}
		return nil
	}
}

// WithVertexAIProject sets the Google Cloud project ID for Vertex AI.
func WithVertexAIProject(projectID string) Option {
	return func(m *Model) error {
		m.projectID = projectID
		return nil
	}
}

// WithVertexAILocation sets the Google Cloud location for Vertex AI.
func WithVertexAILocation(location string) Option {
	return func(m *Model) error {
		m.location = location
		return nil
	}
}

// WithModel sets the Gemini model name to use.
func WithModel(name string) Option {
	return func(m *Model) error {
		m.modelName = name
		return nil
	}
}

// WithGeminiAPIVersion sets the Gemini API version to use (v1alpha or v1beta).
func WithGeminiAPIVersion(version string) Option {
	return func(m *Model) error {
		// Validate the version
		if version != "v1alpha" && version != "v1beta" {
			return fmt.Errorf("invalid Gemini API version: %s (must be 'v1alpha' or 'v1beta')", version)
		}

		// Set the version in the client if it exists
		if m.client != nil {
			m.client.GeminiVersion = version
		}

		return nil
	}
}

// WithListModels specifies that we should list available models and exit.
func WithListModels(filter string, apiVersions ...string) Option {
	return func(m *Model) error {
		// Initialize the client if needed
		if m.client == nil {
			m.client = &api.Client{}
			m.client.APIKey = m.apiKey
		}

		// Initialize the client
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var models []string
		var err error

		// List models based on whether we're using Vertex AI or Gemini API
		if m.backend == BackendVertexAI {
			if m.projectID == "" {
				return fmt.Errorf("project ID is required for Vertex AI")
			}

			fmt.Printf("Fetching available models from Vertex AI (project: %s, location: %s)...\n",
				m.projectID, m.location)

			// Initialize client
			// Set project ID and location on client
			m.client.ProjectID = m.projectID
			m.client.Location = m.location

			// Try to initialize Vertex AI client
			if err := m.client.InitVertexAIClient(ctx); err != nil {
				// If we encounter auth errors, print helpful message and fall back to Gemini API
				if strings.Contains(err.Error(), "credentials") || strings.Contains(err.Error(), "auth") {
					fmt.Fprintf(os.Stderr, "Warning: Failed to authenticate with Vertex AI: %v\n", err)
					fmt.Fprintf(os.Stderr, "Falling back to Gemini API...\n")

					// Set backend back to Gemini API
					m.backend = BackendGeminiAPI

					// Initialize Gemini API client instead
					if err := m.client.InitClient(ctx); err != nil {
						return fmt.Errorf("failed to initialize client after Vertex AI fallback: %w", err)
					}

					// Continue with Gemini API list models
					return m.listGeminiModels(ctx, filter, apiVersions...)
				}
				return fmt.Errorf("failed to initialize Vertex AI client: %w", err)
			}

			// List models from Vertex AI
			models, err = m.client.ListVertexAIModels(ctx, filter)
			if err != nil {
				return fmt.Errorf("failed to list Vertex AI models: %w", err)
			}
		} else {
			return m.listGeminiModels(ctx, filter, apiVersions...)
		}

		// Remove duplicates from the model list
		uniqueModels := removeDuplicateModels(models)

		// Print the list of models
		fmt.Printf("Found %d models\n", len(uniqueModels))
		fmt.Println("==========")
		for _, model := range uniqueModels {
			fmt.Println(model)
		}
		fmt.Println("==========")

		// Indicate that the operation is complete and program should exit
		log.Println("Listing models complete")
		m.exitCode = 0

		// Exit immediately without continuing program execution
		os.Exit(0)
		return nil
	}
}

// removeDuplicateModels removes duplicate model names from a slice
func removeDuplicateModels(models []string) []string {
	uniqueMap := make(map[string]struct{})
	var result []string

	for _, model := range models {
		if _, exists := uniqueMap[model]; !exists {
			uniqueMap[model] = struct{}{}
			result = append(result, model)
		}
	}

	return result
}

// listGeminiModels handles listing models from the Gemini API
func (m *Model) listGeminiModels(ctx context.Context, filter string, apiVersions ...string) error {
	fmt.Println("Fetching available models from Gemini API...")

	// Initialize client for Gemini API
	if err := m.client.InitClient(ctx); err != nil {
		return fmt.Errorf("failed to initialize client: %w", err)
	}

	// Set up the options for listing models
	options := api.DefaultListModelsOptions()
	options.Filter = filter

	// If API versions are specified, use them
	if len(apiVersions) > 0 {
		// Convert string version names to APIVersion enum
		var versionsEnum []api.APIVersion
		for _, v := range apiVersions {
			switch v {
			case "alpha":
				versionsEnum = append(versionsEnum, api.APIVersionAlpha)
			case "beta":
				versionsEnum = append(versionsEnum, api.APIVersionBeta)
			case "v1":
				versionsEnum = append(versionsEnum, api.APIVersionV1)
			default:
				log.Printf("Warning: Ignoring unknown API version '%s'", v)
			}
		}

		if len(versionsEnum) > 0 {
			options.APIVersions = versionsEnum
		}
	}

	// Get the list of models with the specified options
	models, err := m.client.ListModelsWithOptions(options)
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	// Remove duplicates from the model list
	uniqueModels := removeDuplicateModels(models)

	// Print the list of models
	fmt.Printf("Found %d models\n", len(uniqueModels))
	fmt.Println("==========")
	for _, model := range uniqueModels {
		fmt.Println(model)
	}
	fmt.Println("==========")

	// Exit
	log.Println("Listing models complete")
	m.exitCode = 0

	// Exit immediately
	os.Exit(0)
	return nil
}

// WithAudioOutput enables/disables audio output and optionally sets the voice.
// Note: Voice configuration in BidiGenerateContent v1alpha setup is less defined than v1beta.
// This option primarily signals the intent to play audio if received.
func WithAudioOutput(enabled bool, voice ...string) Option {
	return func(m *Model) error {
		m.enableAudio = enabled
		if len(voice) > 0 && voice[0] != "" {
			m.voiceName = voice[0]
		} else if enabled {
			m.voiceName = DefaultVoice
		} else {
			m.voiceName = ""
		}
		// We don't set API config here, but rather in InitBidiStream based on m.enableAudio
		return nil
	}
}

// WithAudioPlayerCommand sets the external command used for audio playback.
func WithAudioPlayerCommand(cmd string) Option {
	return func(m *Model) error {
		m.playerCmd = cmd
		return nil
	}
}

// WithHistory enables chat history and specifies the directory for storing history files.
// If the directory doesn't exist, it will be created.
func WithHistory(enabled bool, historyDir string) Option {
	return func(m *Model) error {
		m.historyEnabled = enabled
		if !enabled {
			return nil
		}

		// Initialize history manager
		historyManager, err := NewHistoryManager(historyDir)
		if err != nil {
			return fmt.Errorf("failed to initialize history manager: %w", err)
		}

		// Set manager and load available sessions
		m.historyManager = historyManager
		if err := m.historyManager.LoadSessions(); err != nil {
			log.Printf("Warning: Failed to load history sessions: %v", err)
		}

		// Create initial session
		m.historyManager.NewSession("New Chat", m.modelName)
		return nil
	}
}

// WithTools enables or disables tool calling support.
func WithTools(enabled bool) Option {
	return func(m *Model) error {
		m.enableTools = enabled
		if enabled {
			m.toolManager = NewToolManager()
			// Register advanced tools by default when tools are enabled
			NewAdvancedToolsRegistry(m.toolManager)
		}
		return nil
	}
}

// WithToolApproval enables or disables approval requirement for tool calls.
func WithToolApproval(requireApproval bool) Option {
	return func(m *Model) error {
		m.requireApproval = requireApproval
		return nil
	}
}

// WithToolsFile loads tools from a JSON file.
func WithToolsFile(filePath string) Option {
	return func(m *Model) error {
		if !m.enableTools {
			log.Println("Warning: Tools are disabled, but a tools file was specified. Enable tools with -tools flag.")
			return nil
		}

		if filePath == "" {
			return nil
		}

		if m.toolManager == nil {
			// Initialize tool manager if not already done
			m.toolManager = NewToolManager()
			// Register advanced tools by default when tools are enabled
			NewAdvancedToolsRegistry(m.toolManager)
		}

		err := LoadToolsFromFile(filePath, m.toolManager)
		if err != nil {
			return fmt.Errorf("failed to load tools from file: %w", err)
		}

		log.Printf("Loaded tools from file: %s", filePath)
		return nil
	}
}

// WithSystemPrompt sets a system prompt for the conversation.
func WithSystemPrompt(prompt string) Option {
	return func(m *Model) error {
		m.systemPrompt = prompt
		return nil
	}
}

// WithTemperature sets the temperature for text generation (controls randomness).
func WithTemperature(temp float32) Option {
	return func(m *Model) error {
		m.temperature = temp
		return nil
	}
}

// WithTopP sets the top-p value for text generation (controls diversity).
func WithTopP(topP float32) Option {
	return func(m *Model) error {
		m.topP = topP
		return nil
	}
}

// WithTopK sets the top-k value for text generation.
func WithTopK(topK int32) Option {
	return func(m *Model) error {
		m.topK = topK
		return nil
	}
}

// WithMaxOutputTokens sets the maximum number of tokens to generate.
func WithMaxOutputTokens(maxTokens int32) Option {
	return func(m *Model) error {
		m.maxOutputTokens = maxTokens
		return nil
	}
}

// WithWebSearch enables or disables web search capabilities.
func WithWebSearch(enabled bool) Option {
	return func(m *Model) error {
		m.enableWebSearch = enabled
		return nil
	}
}

// WithCodeExecution enables or disables code execution capabilities.
func WithCodeExecution(enabled bool) Option {
	return func(m *Model) error {
		m.enableCodeExecution = enabled
		return nil
	}
}

// WithResponseMimeType sets the expected response MIME type (e.g., application/json).
func WithResponseMimeType(mimeType string) Option {
	return func(m *Model) error {
		m.responseMimeType = mimeType
		return nil
	}
}

// WithResponseSchema sets a schema for the response structure from a file.
func WithResponseSchema(schemaFile string) Option {
	return func(m *Model) error {
		m.responseSchemaFile = schemaFile
		return nil
	}
}

// WithDisplayTokenCounts enables or disables displaying token counts in the UI.
func WithDisplayTokenCounts(enabled bool) Option {
	return func(m *Model) error {
		m.displayTokenCounts = enabled
		return nil
	}
}

// WithGlobalTimeout sets a global timeout for the entire program.
func WithGlobalTimeout(timeout time.Duration) Option {
	return func(m *Model) error {
		m.globalTimeout = timeout
		return nil
	}
}

// WithAutoSend sets up automatic sending of a test message after a specified delay.
// This is useful for testing connection stability and debugging hanging connections.
func WithAutoSend(delay string) Option {
	return func(m *Model) error {
		if delay == "" {
			return nil // No auto-send if delay is empty
		}

		duration, err := time.ParseDuration(delay)
		if err != nil {
			return fmt.Errorf("invalid auto-send delay format: %w", err)
		}

		m.autoSendDelay = duration
		m.autoSendEnabled = true
		return nil
	}
}
