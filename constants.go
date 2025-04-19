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

// DefaultModel is the model known to support Bidi streaming
const DefaultModel = "models/gemini-2.0-flash-live-001"

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
