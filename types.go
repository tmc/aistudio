package aistudio

import (
	"context"
	"time"

	"cloud.google.com/go/ai/generativelanguage/apiv1alpha/generativelanguagepb"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tmc/aistudio/api"
	"github.com/tmc/aistudio/audioplayer"
	"github.com/tmc/aistudio/settings"
)

// AudioPlaybackMode defines the method used for audio playback
type AudioPlaybackMode int

const (
	// AudioPlaybackDirect plays each audio chunk directly
	AudioPlaybackDirect AudioPlaybackMode = iota

	// AudioPlaybackOnDiskFIFO uses a temporary file as a buffer between audio chunks
	AudioPlaybackOnDiskFIFO
)

// VideoInputMode defines the type of video input simulation.
type VideoInputMode int

const (
	VideoInputNone VideoInputMode = iota
	VideoInputCamera
	VideoInputScreen
)

func (v VideoInputMode) String() string {
	switch v {
	case VideoInputCamera:
		return "Camera"
	case VideoInputScreen:
		return "Screen"
	default:
		return "None"
	}
}

// AudioChunk represents a piece of audio data with associated text
type AudioChunk struct {
	Data         []byte    // The audio data
	Text         string    // The associated text
	IsProcessing bool      // Whether this chunk is currently being processed
	IsComplete   bool      // Whether this chunk has finished processing
	StartTime    time.Time // When this chunk started processing
	Duration     float64   // Estimated duration in seconds, based on audio data size
	MessageIndex int       // Index of the message this chunk belongs to (-1 if not applicable)
}

// Message represents a chat message with optional audio data
type Message struct {
	Sender    string    // Who sent the message (You, Gemini, System)
	Content   string    // The message text
	HasAudio  bool      // Whether the message has associated audio
	AudioData []byte    // The raw audio data (if HasAudio is true) - stores the *complete* audio after consolidation
	IsPlaying bool      // Whether the audio is currently playing
	IsPlayed  bool      // Whether the audio has been played
	Timestamp time.Time // When the message was sent
}

// Component is the interface that all UI components should implement
type Component interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (tea.Model, tea.Cmd)
	View() string
}

// FocusableComponent is the interface for components that can be focused
type FocusableComponent interface {
	Component
	Focus()
	Blur()
	IsFocused() bool
}

// Model represents the state of the Bubble Tea application.
type Model struct {
	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	client   *api.Client // API client wrapper

	// Stream connections - we'll use either one depending on the mode
	stream     generativelanguagepb.GenerativeService_StreamGenerateContentClient
	bidiStream generativelanguagepb.GenerativeService_BidiGenerateContentClient

	streamReady bool
	useBidi     bool      // Whether to use BidiGenerateContent (true) or StreamGenerateContent (false)
	messages    []Message // Stores structured chat messages for display
	err         error
	width       int
	height      int
	sending     bool
	receiving   bool
	quitting    bool

	// Stream management
	streamCtx       context.Context
	streamCtxCancel context.CancelFunc

	// Configuration
	modelName   string
	apiKey      string // Store API key if provided via option
	enableAudio bool   // Config: Enable audio output?
	voiceName   string // Config: Which voice to use?
	playerCmd   string // Config: Command to play raw PCM audio
	showLogo    bool   // Whether to show a logo or not

	// Log Messages
	logMessages     []string // Stores recent log messages
	maxLogMessages  int      // Maximum number of log messages to store
	showLogMessages bool     // Whether to show log messages or not

	// Simulated Input State
	micActive      bool
	videoInputMode VideoInputMode

	// Audio processing
	audioChannel      chan AudioChunk   // Channel for audio processing queue
	currentAudio      *AudioChunk       // Currently playing audio (needs careful sync)
	audioQueue        []AudioChunk      // Queue of audio chunks waiting to be processed (UI only)
	isAudioProcessing bool              // Whether audio is currently being processed (player active)
	showAudioStatus   bool              // Whether to show audio status in UI
	audioPlaybackMode AudioPlaybackMode // How to play audio (direct or per-play files)

	// Audio consolidation
	consolidatedAudioData  []byte        // Buffer for consolidating multiple audio chunks
	bufferStartTime        time.Time     // When we started buffering audio chunks
	bufferMessageIdx       int           // Index of the message receiving consolidated audio data
	bufferTimer            *time.Timer   // Timer for flushing audio buffer after timeout
	lastFlushTime          time.Time     // When we last flushed the audio buffer
	currentBufferWindow    time.Duration // Current adaptive buffer window duration
	recentChunkSizes       []int         // Track recent chunk sizes for adaptive buffering
	recentChunkTimes       []time.Time   // Track timestamps of recent chunks
	consecutiveSmallChunks int           // Count of consecutive small chunks

	// Ticker state
	tickerRunning bool // Flag to indicate if the playback ticker is active

	// Channel for goroutines to send messages back to the UI loop
	uiUpdateChan chan tea.Msg // Channel for safe UI updates

	// New UI components
	settingsPanel     *settings.Model
	activeAudioPlayer *audioplayer.Model

	// Focus management
	focusedComponent  string // One of "input", "viewport", "settings"
	showSettingsPanel bool   // Whether to show the settings panel
}

// Option defines a functional option for configuring the Model.
type Option func(*Model) error

// --- Messages ---

// Stream-related messages are defined in stream.go

// Audio-related messages
type audioPlaybackErrorMsg struct{ err error }
type audioPlaybackStartedMsg struct{ chunk AudioChunk }   // Chunk sent to player
type audioPlaybackCompletedMsg struct{ chunk AudioChunk } // Chunk finished playing
type audioQueueUpdatedMsg struct{}                        // Chunk added to audioChannel

// logMessageMsg is for internal logging captured via interceptor
type logMessageMsg struct {
	message string
}

// flushAudioBufferMsg is sent when the audio buffer timer expires or size threshold met
type flushAudioBufferMsg struct{}

// playbackTickMsg triggers UI refresh during playback
type playbackTickMsg time.Time
