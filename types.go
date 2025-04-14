// Edited with Aider on April 14, 2025
package aistudio

import (
	"context"
	"log"
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

// ExecutableCode represents code that can be executed
type ExecutableCode struct {
	Language string
	Code     string
}

// ExecutableCodeResult represents the result of executing code
type ExecutableCodeResult = generativelanguagepb.CodeExecutionResult

// Message represents a chat message with optional audio data
type Message struct {
	Sender    string // Who sent the message (You, Gemini, System)
	Content   string // The message text
	HasAudio  bool   // Whether the message has associated audio
	AudioData []byte // The raw audio data (if HasAudio is true) - stores the *complete* audio after consolidation
	IsPlaying bool   // Whether the audio is currently playing
	IsPlayed  bool   // Whether the audio has been played

	IsToolCall bool      // Whether this message is a tool call
	ToolCall   *ToolCall // The tool call associated with this message (if any)

	IsExecutableCode       bool                  // Whether this message contains executable code
	ExecutableCode         *ExecutableCode       // The executable code associated with this message (if any)
	IsExecutableCodeResult bool                  // Whether this message contains executable code result
	ExecutableCodeResult   *ExecutableCodeResult // The executable code result associated with this message (if any)

	// Safety ratings for this message's content
	SafetyRatings []*SafetyRating // Safety ratings associated with this message

	// Grounding information
	HasGroundingMetadata bool               // Whether this message has grounding metadata
	GroundingMetadata    *GroundingMetadata // Grounding metadata for the message

	// Function call/response tracking
	FunctionCall *generativelanguagepb.FunctionCall // Function call associated with this message

	// Token usage tracking
	PromptTokenCount   int32 // Number of tokens in the prompt
	ResponseTokenCount int32 // Number of tokens in the response
	TotalTokenCount    int32 // Total tokens used (prompt + response)
	HasTokenInfo       bool  // Whether token count information is available for this message

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
	streamCtx            context.Context
	streamCtxCancel      context.CancelFunc
	rootCtx              context.Context
	rootCtxCancel        context.CancelFunc
	streamRetryAttempt   int           // Tracks the current retry attempt number
	currentStreamBackoff time.Duration // Tracks the current backoff duration
	globalTimeout        time.Duration // Global timeout for the entire program

	// Basic configuration
	modelName   string
	apiKey      string // Store API key if provided via option
	enableAudio bool   // Config: Enable audio output?
	voiceName   string // Config: Which voice to use?
	playerCmd   string // Config: Command to play raw PCM audio
	showLogo    bool   // Whether to show a logo or not

	// Generation parameters
	temperature     float32 // Controls randomness (0.0-1.0)
	topP            float32 // Controls diversity (0.0-1.0)
	topK            int32   // Number of highest probability tokens to consider
	maxOutputTokens int32   // Maximum number of tokens to generate

	// Feature flags
	enableWebSearch     bool   // Enable web search/grounding capabilities
	enableCodeExecution bool   // Enable code execution capabilities
	responseMimeType    string // MIME type of the expected response (e.g., "application/json")
	responseSchemaFile  string // Path to JSON schema file defining response structure
	displayTokenCounts  bool   // Whether to display token counts in the UI

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

	// History management
	historyManager *HistoryManager // Manages chat history
	historyEnabled bool            // Whether history is enabled

	// Tool calling support
	enableTools      bool         // Whether tool calling is enabled
	toolManager      *ToolManager // Tool manager for handling tools
	activeToolCall   *ToolCall    // Currently active tool call, if any
	processingTool   bool         // Whether a tool call is being processed
	pendingToolCalls []ToolCall   // Tool calls waiting for approval
	showToolApproval bool         // Whether to show the tool approval modal
	approvalIndex    int          // Current tool call being approved
	requireApproval  bool         // Whether tool calls require approval

	// System prompt
	systemPrompt string // System prompt to use for the conversation
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

// ProcessGenerativeLanguageResponse processes a response from the GenerativeLanguage API
// and produces display-ready data structures
func (m *Model) ProcessGenerativeLanguageResponse(output api.StreamOutput) {
	// If there's function call data, create a special message for it
	if output.FunctionCall != nil {
		log.Printf("Processing function call: %s", output.FunctionCall.Name)

		// Create a new message for the function call
		funcCallMsg := Message{
			Sender:       "System",
			Content:      "Function Call",
			FunctionCall: output.FunctionCall,
			Timestamp:    time.Now(),
		}

		// Add it to the messages
		m.messages = append(m.messages, funcCallMsg)

		// Update the viewport content
		m.viewport.SetContent(m.formatAllMessages())
		m.viewport.GotoBottom()
	}

	// If there's grounding metadata, associate it with the last message
	if output.GroundingMetadata != nil && len(m.messages) > 0 {
		idx := len(m.messages) - 1

		log.Printf("Adding grounding metadata to message %d", idx)

		// Initialize the grounding data in our internal format
		m.messages[idx].HasGroundingMetadata = true
		m.messages[idx].GroundingMetadata = m.convertGroundingMetadata(output.GroundingMetadata)

		// Update the viewport content
		m.viewport.SetContent(m.formatAllMessages())
		m.viewport.GotoBottom()
	}

	// If there are safety ratings, associate them with the last message
	if output.SafetyRatings != nil && len(output.SafetyRatings) > 0 && len(m.messages) > 0 {
		idx := len(m.messages) - 1

		log.Printf("Adding %d safety ratings to message %d", len(output.SafetyRatings), idx)

		// Convert each safety rating to our internal format
		for _, apiRating := range output.SafetyRatings {
			safetyRating := convertSafetyRating(apiRating)
			if safetyRating != nil {
				m.messages[idx].SafetyRatings = append(m.messages[idx].SafetyRatings, safetyRating)
			}
		}

		// Update the viewport content
		m.viewport.SetContent(m.formatAllMessages())
		m.viewport.GotoBottom()
	}

	// If token counts are available and this is the final chunk, add them to the last message
	if output.IsFinalChunk && (output.PromptTokenCount > 0 || output.CandidateTokenCount > 0 || output.TotalTokenCount > 0) && len(m.messages) > 0 {
		idx := len(m.messages) - 1

		log.Printf("Adding token counts to message %d: Prompt=%d, Response=%d, Total=%d",
			idx, output.PromptTokenCount, output.CandidateTokenCount, output.TotalTokenCount)

		m.messages[idx].PromptTokenCount = output.PromptTokenCount
		m.messages[idx].ResponseTokenCount = output.CandidateTokenCount
		m.messages[idx].TotalTokenCount = output.TotalTokenCount
		m.messages[idx].HasTokenInfo = true

		// Update the viewport content if token display is enabled
		if m.displayTokenCounts {
			m.viewport.SetContent(m.formatAllMessages())
			m.viewport.GotoBottom()
		}
	}
}

// DisplaySafetyRating represents a safety rating in a displayable format
type SafetyRating struct {
	Category    string  // Category of the safety rating (e.g., HARM_CATEGORY_SEXUALLY_EXPLICIT)
	Probability string  // Probability level (e.g., NEGLIGIBLE, LOW, MEDIUM, HIGH)
	Score       float32 // Raw probability score if available
	Blocked     bool    // Whether content was blocked by this safety rating
}

// GroundingChunk represents a chunk of information used for grounding
type GroundingChunk struct {
	ID       string // Identifier for the chunk
	Title    string // Title of the chunk
	URI      string // Source URI of the chunk
	IsWeb    bool   // Whether this chunk is from the web
	Selected bool   // Whether this chunk was selected as a reference
}

// GroundingSupport represents support for a specific claim in the response
type GroundingSupport struct {
	Text           string            // The text segment this support applies to
	ChunkIndices   []int             // Indices to grounding chunks supporting this segment
	ChunksSelected []*GroundingChunk // The actual supporting chunks
	Confidence     []float32         // Confidence scores for each supporting chunk
}

// GroundingMetadata contains metadata related to grounding
type GroundingMetadata struct {
	Chunks              []*GroundingChunk   // The chunks used for grounding
	Supports            []*GroundingSupport // Support information for parts of the response
	WebSearchQueries    []string            // Web search queries for follow-up
	HasSearchEntryPoint bool                // Whether there's a search entry point
	SearchEntryPoint    *SearchEntryPoint   // Entry point for search functionality
}

// SearchEntryPoint represents a Google search entry point
type SearchEntryPoint struct {
	RenderedContent string // Web content that can be embedded
	HasSDKBlob      bool   // Whether SDK blob data is available
}

// FormattedFunctionCall represents a function call with formatted arguments
type FormattedFunctionCall struct {
	Name         string                             // Name of the function
	Arguments    string                             // Arguments as formatted JSON string
	ArgumentsMap interface{}                        // Arguments as structured data
	Raw          *generativelanguagepb.FunctionCall // The raw function call
}
