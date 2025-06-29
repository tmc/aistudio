// Edited with Aider on April 14, 2025
package aistudio

import (
	"time"

	"github.com/charmbracelet/lipgloss"
)

// --- Config ---
// DefaultVoice is the default voice if audio enabled
const DefaultVoice = "Puck"

// Default audio sample rate - Gemini likely outputs 24kHz
const audioSampleRate = 24000

// Default audio format - Signed 16-bit Little Endian (Linear PCM)
const audioFormat = "s16le"

// DefaultModel is the model known to support streaming with best capabilities
const DefaultModel = "models/gemini-1.5-flash-latest"

// DefaultGrokModel is the default Grok model
const DefaultGrokModel = "grok-beta"

// BackendType defines the AI backend to use.
type BackendType int

const (
	// BackendGeminiAPI uses the Google Gemini API.
	BackendGeminiAPI BackendType = iota
	// BackendVertexAI uses the Google Cloud Vertex AI.
	BackendVertexAI
	// BackendGrok uses the xAI Grok API.
	BackendGrok
)

// String returns a string representation of the backend type.
func (b BackendType) String() string {
	switch b {
	case BackendGeminiAPI:
		return "Gemini API"
	case BackendVertexAI:
		return "Vertex AI"
	case BackendGrok:
		return "Grok API"
	default:
		return "Unknown Backend"
	}
}

// Environment variable names for configuration.
const (
	EnvGeminiAPIKey     = "GEMINI_API_KEY"
	EnvGrokAPIKey       = "GROK_API_KEY"
	EnvUseVertexAI      = "AISTUDIO_USE_VERTEXAI"
	EnvUseGrok          = "AISTUDIO_USE_GROK"
	EnvVertexAIProject  = "AISTUDIO_VERTEXAI_PROJECT"
	EnvVertexAILocation = "AISTUDIO_VERTEXAI_LOCATION"
	EnvDefaultModel     = "AISTUDIO_DEFAULT_MODEL"
	EnvDefaultVoice     = "AISTUDIO_DEFAULT_VOICE"
	// Debug environment variables
	EnvDebugConnection   = "AISTUDIO_DEBUG_CONNECTION"
	EnvDebugStream       = "AISTUDIO_DEBUG_STREAM"
	EnvConnectionTimeout = "AISTUDIO_CONNECTION_TIMEOUT"
)

// Initial audio buffering window - will adjust dynamically based on chunk patterns
const initialAudioBufferingWindow = 300 * time.Millisecond

// Maximum buffering window we'll allow
const maxAudioBufferingWindow = 100 * time.Millisecond

// Minimum buffer size to trigger playback regardless of time (32KB)
const minBufferSizeForPlayback = 64 * 1024

// Minimum buffer size for continuous playback segments (16KB)
// Smaller than regular minBufferSizeForPlayback to allow quicker playback of continuous audio
const continuousPlaybackBufferSize = 32 * 1024

// Minimum time between audio flushes to prevent excessive small file creation
// Reduced to 150ms to allow more frequent playback of successive chunks
const minTimeBetweenFlushes = 150 * time.Millisecond

// Audio chunk size threshold for adaptive buffering (10KB)
const adaptiveBufferThreshold = 10 * 1024

// These buffer settings are optimized for the lookahead buffering system
// that eliminates gaps between audio segments during continuous playback

// Connection timeout settings
const (
	// DefaultConnectionTimeout is the default timeout for establishing connections
	DefaultConnectionTimeout = 60 * time.Second
	// StreamOperationTimeout is the timeout for individual stream operations
	StreamOperationTimeout = 30 * time.Second
	// HealthCheckInterval is how often to send keepalive/health checks
	HealthCheckInterval = 5 * time.Minute
	// ConnectionRetryDelay is the base delay between connection retries
	ConnectionRetryDelay = 2 * time.Second
	// MaxRetryDelay is the maximum delay between connection retries
	MaxRetryDelay = 60 * time.Second
	// DebuggingLogInterval is how often to log debugging information
	DebuggingLogInterval = 30 * time.Second
)

// progressBarWidth defines the width of the playback progress bar.
const progressBarWidth = 25 // characters

type senderName string

const (
	senderNameUser   senderName = "You"
	senderNameModel  senderName = "Gemini"
	senderNameSystem senderName = "System"
)

// DefaultModelType is the default model type (e.g., Chat, Audio, etc.)
const DefaultModelType = "Chat"

// modelType describes the type of model used
const modelType = "Chat"

// modelName describes the default model name
const modelName = "Model"

// Styles
var (
	senderUserStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))    // Cyan
	senderModelStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))    // Magenta
	senderSystemStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))    // Gray
	toolCallStyle     = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("36")) // Cyan italic for tool calls
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)    // Red
	statusStyle       = lipgloss.NewStyle().Faint(true)
	inputModeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true) // Bright Green
	logoStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)  // Magenta
	logMessageStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true) // Gray
	// Audio UI Styles
	audioIconStyle  = lipgloss.NewStyle().Bold(true)
	audioTimeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // Gray
	audioProgStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // Magenta
	audioHelpStyle  = statusStyle                                         // Reuse faint style
	audioPlayIcon   = audioIconStyle.Foreground(lipgloss.Color("10")).Render("▶")
	audioPlayedIcon = audioIconStyle.Foreground(lipgloss.Color("8")).Render("✓")
	audioReadyIcon  = audioIconStyle.Foreground(lipgloss.Color("5")).Render("🔊")
	// Executable Code Styles
	executableCodeLangStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))     // Yellow for language
	executableCodeBlockStyle  = lipgloss.NewStyle().Background(lipgloss.Color("235")).Padding(0, 1) // Dark gray background for code block
	executableCodeResultStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("40"))                // Green for successful output
	// Tool Call/Result Styles
	toolIcon              = "⏺"                                                                // Icon for tool messages
	toolCallHeaderStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)    // Magenta, Bold for Tool Call header
	toolResultHeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)    // Teal, Bold for Tool Result header
	toolArgsStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))              // Gray for arguments/results JSON
	toolStatusStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Italic(true) // Orange, Italic for status (Waiting, Executing, etc.)
	// toolExpandHintStyle = lipgloss.NewStyle().Faint(true) // Style for expand/collapse hint (if implemented)
)

// styleDefinitions contains all the styles used in the UI.
// Centralizing styles helps with consistency and easier updates.
var (
	// Text colors for views - these override constants.go styles within this file
	viewTitleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))                                                    // Magenta
	viewToolNameStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))                                                    // Teal
	viewButtonStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("63")).Padding(0, 2).MarginRight(2)  // Black on Magenta
	viewButtonDangerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("160")).Padding(0, 2).MarginRight(2) // Black on Red
	viewSpinnerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))                                                              // Pink for spinner
	viewToolStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))                                                               // Teal for tools
	// Dialog option styles
	dialogOptionSelected   = lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Bold(true)  // Blue, Bold
	dialogOptionNum        = lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Faint(true) // Blue, Dim
	dialogOptionUnselected = lipgloss.NewStyle().Faint(true)                                  // Dimmed
	dialogActionHighlight  = lipgloss.NewStyle().Bold(true)                                   // Bold
	dialogHintStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Bold(true) // Gray, Bold

	// Borders and containers
	viewportStyle      = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	settingsPanelStyle = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	viewModalStyle     = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("177:185:249")).Padding(1, 3)
)

// Styles for tool call UI components
var (
	toolIdStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	toolRunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	toolErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	toolBorderStyle  = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("205")).
				Padding(0, 1)
	systemMessageStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
)
