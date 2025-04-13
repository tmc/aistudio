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

// Styles
var (
	senderYouStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")) // Cyan
	senderGeminiStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")) // Magenta
	senderSystemStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8")) // Gray
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true) // Red
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
)
