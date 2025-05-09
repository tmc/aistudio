package aistudio

import (
	"fmt"
	"log"
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

// WithModel sets the Gemini model name to use.
func WithModel(name string) Option {
	return func(m *Model) error {
		m.modelName = name
		return nil
	}
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

// WithAudioPlaybackMode sets the audio playback mode.
func WithAudioPlaybackMode(mode AudioPlaybackMode) Option {
	return func(m *Model) error {
		m.audioPlaybackMode = mode
		return nil
	}
}

// WithLogo enables or disables the logo display.
func WithLogo(showLogo bool) Option {
	return func(m *Model) error {
		m.showLogo = showLogo
		return nil
	}
}

// WithLogMessages enables or disables the log messages display.
func WithLogMessages(show bool, maxEntries ...int) Option {
	return func(m *Model) error {
		m.showLogMessages = show

		// Set default maximum number of log messages if not specified
		if len(maxEntries) > 0 && maxEntries[0] > 0 {
			m.maxLogMessages = maxEntries[0]
		} else if m.maxLogMessages == 0 {
			m.maxLogMessages = 10 // Default to 10 entries
		}

		return nil
	}
}

// WithAudioStatus enables or disables the audio playback status display.
func WithAudioStatus(show bool) Option {
	return func(m *Model) error {
		m.showAudioStatus = show
		return nil
	}
}

// WithBidiStream enables or disables using the true bidirectional stream.
func WithBidiStream(enable bool) Option {
	return func(m *Model) error {
		m.useBidi = enable
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
