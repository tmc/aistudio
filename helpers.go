package aistudio

import (
	"fmt"
	"io"
	"log"
	"math"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"bytes"
	"encoding/json"

	"github.com/charmbracelet/lipgloss"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

// formatMessage creates a Message from sender and content
func formatMessage(sender senderName, content string) Message {
	return Message{
		Sender:    sender,
		Content:   content,
		HasAudio:  false, // Default, can be updated later
		Timestamp: time.Now(),
	}
}

// formatError creates an error Message
func formatError(err error) Message {
	return Message{
		Sender:    "System",
		Content:   fmt.Sprintf("Error: %v", err),
		HasAudio:  false,
		Timestamp: time.Now(),
	}
}

// formatErrorString formats an error as a string for the UI
func formatErrorString(err error) string {
	return errorStyle.Render(fmt.Sprintf("Error: %v", err))
}

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
			// Send message to update UI if needed (optional)
			// tea.Batch(logMessageMsg{message: trimmedMessage})? Needs careful handling.
		}
	}

	// Write to the original log output (e.g., file)
	if li.original != nil {
		// Write original bytes to preserve formatting in log file
		return li.original.Write(p)
	}

	return len(p), nil
}

// formatDuration converts total seconds into MM:SS format.
func formatDuration(totalSeconds float64) string {
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	// Round to nearest second for display consistency
	ts := int(math.Round(totalSeconds))
	minutes := ts / 60
	seconds := ts % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// formatToolCallMessage creates a Message for a tool call with enhanced formatting
func formatToolCallMessage(toolCall ToolCall, status ToolCallStatus) Message {
	var content strings.Builder
	toolCallStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")) // Magenta style for tool calls
	argsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))     // Gray for args

	content.WriteString(toolCallStyle.Render(fmt.Sprintf("🔧 Tool Call: %s (ID: %s)", toolCall.Name, toolCall.ID)))
	content.WriteString("\n")
	content.WriteString(toolCallStyle.Render(fmt.Sprintf("   Status: %s", status)))
	content.WriteString("\n")

	// Format arguments
	if len(toolCall.Arguments) > 0 {
		var args bytes.Buffer
		if err := json.Indent(&args, toolCall.Arguments, "   ", "  "); err != nil {
			// Fallback if indent fails
			content.WriteString(argsStyle.Render(fmt.Sprintf("   Arguments: %s", string(toolCall.Arguments))))
		} else {
			content.WriteString(argsStyle.Render("   Arguments:"))
			content.WriteString("\n")
			content.WriteString(argsStyle.Render(args.String()))
		}
		content.WriteString("\n")
	} else {
		content.WriteString(argsStyle.Render("   Arguments: (none)"))
		content.WriteString("\n")
	}

	return Message{
		Sender:     "System",
		Content:    content.String(), // Use the pre-formatted content
		IsToolCall: true,
		ToolCall:   &toolCall,
		//ToolStatus: status,
		Timestamp: time.Now(),
	}
}

func formatToolResultMessage(toolCallID, toolName string, result *structpb.Struct, status ToolCallStatus) Message {
	var content strings.Builder
	toolResultStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	resultStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("248"))

	content.WriteString(toolResultStyle.Render(fmt.Sprintf("✅ Tool Result: %s (ID: %s)", toolName, toolCallID)))
	content.WriteString("\n")

	if result != nil && len(result.Fields) > 0 {
		content.WriteString(resultStyle.Render("   Result:"))
		content.WriteString("\n")

		jsonBytes, err := protojson.Marshal(result)
		if err != nil {
			content.WriteString(resultStyle.Render(fmt.Sprintf("   Error marshaling result: %v", err)))
		} else {
			var prettyJSON bytes.Buffer
			if err := json.Indent(&prettyJSON, jsonBytes, "   ", "  "); err != nil {
				content.WriteString(resultStyle.Render(fmt.Sprintf("   %s", string(jsonBytes))))
			} else {
				content.WriteString(resultStyle.Render(prettyJSON.String()))
			}
		}
		content.WriteString("\n")
	} else {
		content.WriteString(resultStyle.Render("   Result: (empty)"))
		content.WriteString("\n")
	}

	return Message{
		Sender:       "System",
		Content:      content.String(),
		IsToolResult: true,
		ToolResponse: &ToolResponse{
			Id:       toolCallID,
			Name:     toolName,
			Response: result,
		},
		ToolCall:  &ToolCall{ID: toolCallID, Name: toolName},
		Timestamp: time.Now(),
	}
}
