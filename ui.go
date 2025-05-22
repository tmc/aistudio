package aistudio

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"runtime"
	"strings"

	"cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
)

// ----------------------------------------
// Message Formatting Helpers
// ----------------------------------------

// ----------------------------------------
// Message Rendering
// ----------------------------------------
// Message rendering has been refactored to render.go

// renderAllMessages formats all messages as a single string for display
// Groups tool calls with their results and adds borders around messages
func (m *Model) renderAllMessages() string {
	renderer := NewMessageRenderer(m)
	return renderer.RenderMessages()
}

// ----------------------------------------
// UI Component Views
// ----------------------------------------

func (m Model) headerView() string {
	renderer := NewUIRenderer(&m)
	return renderer.RenderHeader()
}

func (m Model) logMessagesView() string {
	renderer := NewUIRenderer(&m)
	return renderer.RenderLogMessages()
}

func (m Model) audioStatusView() string {
	// Always return empty string - we no longer show the audio status box
	return ""
}

func (m Model) footerView() string {
	renderer := NewUIRenderer(&m)
	return renderer.RenderFooter()
}

// ----------------------------------------
// Helper Functions
// ----------------------------------------

// formatMessage creates a Message from sender and content
// This function is kept for backward compatibility, but new code should use MessageFormatter
func formatMessage(sender senderName, content string) Message {
	formatter := NewMessageFormatter()
	return formatter.FormatMessage(sender, content)
}

// formatError creates an error Message
// This function is kept for backward compatibility, but new code should use MessageFormatter
func formatError(err error) Message {
	formatter := NewMessageFormatter()
	return formatter.FormatError(err)
}

// formatErrorString has been moved to MessageFormatter

// ----------------------------------------
// Audio Player Detection
// ----------------------------------------

// detectAudioPlayer attempts to find a suitable audio player command
func detectAudioPlayer() string {
	var cmd string
	var playerPath string
	var err error

	// Try ffplay (FFmpeg) first - handles stdin well
	if playerPath, err = exec.LookPath("ffplay"); err == nil {
		// Note: `-i -` reads from stdin. Assumes raw PCM with format flags.
		// WAV header *might* also work with ffplay but raw is cleaner if flags are right.
		cmd = fmt.Sprintf("%s -autoexit -nodisp -loglevel error -f %s -ar %d -ac 1 -i -", playerPath, audioFormat, audioSampleRate)
		log.Printf("Auto-detected audio player: %s (using ffplay)", cmd)
		return cmd
	}

	// Try Linux-specific players
	if runtime.GOOS == "linux" {
		if playerPath, err = exec.LookPath("aplay"); err == nil {
			// aplay needs specific format flags for raw PCM from stdin
			cmd = fmt.Sprintf("%s -q -c 1 -r %d -f %s -", playerPath, audioSampleRate, "S16_LE") // S16_LE matches s16le
			log.Printf("Auto-detected audio player: %s (using aplay)", cmd)
			return cmd
		}
		if playerPath, err = exec.LookPath("paplay"); err == nil {
			// PulseAudio player, also needs format flags for raw PCM
			cmd = fmt.Sprintf("%s --raw --channels=1 --rate=%d --format=%s", playerPath, audioSampleRate, audioFormat)
			log.Printf("Auto-detected audio player: %s (using paplay)", cmd)
			return cmd
		}
	}

	// Try macOS player (afplay) - requires temp files
	if runtime.GOOS == "darwin" {
		if playerPath, err = exec.LookPath("afplay"); err == nil {
			log.Println("Detected 'afplay'. Will use temp files for playback.")
			// Return just "afplay" as the command name. Playback logic handles the rest.
			return "afplay"
		} else {
			log.Println("Info: 'ffplay' not found. For best audio on macOS, install FFmpeg (`brew install ffmpeg`).")
		}
	}

	// Try ffmpeg as a player (less common, might depend on output device setup)
	if playerPath, err = exec.LookPath("ffmpeg"); err == nil {
		audioOutput := "alsa" // Default for Linux
		if runtime.GOOS == "darwin" {
			audioOutput = "coreaudio"
		} else if runtime.GOOS == "windows" {
			audioOutput = "dsound" // Example for Windows DirectSound
		}
		// Reads raw PCM from stdin, outputs to default audio device
		cmd = fmt.Sprintf("%s -f %s -ar %d -ac 1 -i - -f %s -", playerPath, audioFormat, audioSampleRate, audioOutput)
		log.Printf("Auto-detected audio player: %s (using ffmpeg)", cmd)
		return cmd
	}

	// Fallback if nothing found
	if runtime.GOOS == "windows" {
		log.Println("Warning: Audio playback auto-detect failed for Windows. Install FFmpeg or use WithAudioPlayerCommand.")
	} else {
		log.Println("Warning: Could not auto-detect a suitable audio player. Please install ffplay, aplay, paplay, or use WithAudioPlayerCommand.")
	}
	return "" // Return empty string if no player found
}

// ----------------------------------------
// Log Interceptor
// ----------------------------------------

// logInterceptor implements io.Writer to capture log output for display in UI
type logInterceptor struct {
	model    *Model
	original io.Writer // The original log output
}

func (li *logInterceptor) Write(p []byte) (n int, err error) {
	message := string(p)

	// Add message to the model's log messages (if enabled in model)
	if li.model != nil && li.model.maxLogMessages > 0 {
		// Trim whitespace for cleaner display
		trimmedMessage := strings.TrimSpace(message)
		if trimmedMessage != "" { // Avoid adding empty lines
			// Add to the model's log messages
			li.model.logMessages = append(li.model.logMessages, trimmedMessage)
			// Trim to max length
			if len(li.model.logMessages) > li.model.maxLogMessages {
				li.model.logMessages = li.model.logMessages[len(li.model.logMessages)-li.model.maxLogMessages:]
			}
		}
	}

	// Write to the original log output (e.g., file)
	if li.original != nil {
		// Write original bytes to preserve formatting in log file
		return li.original.Write(p)
	}

	return len(p), nil
}

// ----------------------------------------
// Tool Call Formatting
// ----------------------------------------

// formatDuration has been moved to MessageFormatter

// Tool formatting functions have been moved to ToolFormatter

// formatToolCallMessageFromViewModel creates a Message for a tool call with enhanced formatting
// This function is kept for backward compatibility, but new code should use ToolFormatter
func formatToolCallMessageFromViewModel(vm ToolCallViewModel, availWidth ...int) Message {
	formatter := NewToolFormatter()
	return formatter.CreateToolCallMessage(vm, availWidth...)
}

// formatToolResultMessageFromViewModel creates a Message for a tool result with enhanced formatting
// This function is kept for backward compatibility, but new code should use ToolFormatter
func formatToolResultMessageFromViewModel(vm ToolCallViewModel, availWidth ...int) Message {
	formatter := NewToolFormatter()
	return formatter.CreateToolResultMessage(vm, availWidth...)
}

// ----------------------------------------
// API Response Conversion Helpers
// ----------------------------------------

// convertSafetyRating converts API safety rating to our display format
// This function is kept for backward compatibility, but new code should use APIFormatter
func convertSafetyRating(apiRating *generativelanguagepb.SafetyRating) *SafetyRating {
	formatter := NewAPIFormatter()
	return formatter.ConvertSafetyRating(apiRating)
}

// convertGroundingMetadata converts API grounding metadata to our display format
// This function is kept for backward compatibility, but new code should use APIFormatter
func (m *Model) convertGroundingMetadata(apiMetadata *generativelanguagepb.GroundingMetadata) *GroundingMetadata {
	formatter := NewAPIFormatter()
	return formatter.ConvertGroundingMetadata(apiMetadata)
}

// convertFunctionCall has been moved to APIFormatter
