package aistudio

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tmc/aistudio/internal/helpers"
)

// AudioInputManager handles microphone input and streaming
type AudioInputManager struct {
	isRecording      bool
	recordingProcess *exec.Cmd
	recordingContext context.Context
	recordingCancel  context.CancelFunc
	inputDevice      string
	sampleRate       int
	channels         int
	format           string
	bufferSize       int
	
	// Audio data channels
	audioInputChan   chan AudioInputChunk
	uiUpdateChan     chan tea.Msg
	
	// Synchronization
	mu               sync.RWMutex
	
	// Configuration
	enableVAD        bool  // Voice Activity Detection
	silenceThreshold float64
	chunkDuration    time.Duration
}

// AudioInputChunk represents a chunk of audio input data
type AudioInputChunk struct {
	Data      []byte
	Timestamp time.Time
	Duration  time.Duration
	SampleRate int
	Channels   int
	Format     string
	IsVoice    bool  // Voice Activity Detection result
}

// AudioInputConfig contains configuration for audio input
type AudioInputConfig struct {
	InputDevice      string
	SampleRate       int
	Channels         int
	Format           string
	BufferSize       int
	EnableVAD        bool
	SilenceThreshold float64
	ChunkDuration    time.Duration
}

// DefaultAudioInputConfig returns default configuration for audio input
func DefaultAudioInputConfig() AudioInputConfig {
	return AudioInputConfig{
		InputDevice:      "default",  // Use system default microphone
		SampleRate:       48000,      // 48kHz to match output
		Channels:         1,          // Mono
		Format:           "s16le",    // 16-bit signed little-endian
		BufferSize:       4096,       // 4KB buffer
		EnableVAD:        true,       // Enable voice activity detection
		SilenceThreshold: 0.01,       // Silence threshold for VAD
		ChunkDuration:    100 * time.Millisecond, // 100ms chunks
	}
}

// NewAudioInputManager creates a new audio input manager
func NewAudioInputManager(config AudioInputConfig, uiUpdateChan chan tea.Msg) *AudioInputManager {
	return &AudioInputManager{
		inputDevice:      config.InputDevice,
		sampleRate:       config.SampleRate,
		channels:         config.Channels,
		format:           config.Format,
		bufferSize:       config.BufferSize,
		enableVAD:        config.EnableVAD,
		silenceThreshold: config.SilenceThreshold,
		chunkDuration:    config.ChunkDuration,
		audioInputChan:   make(chan AudioInputChunk, 50), // Buffer for 50 chunks
		uiUpdateChan:     uiUpdateChan,
	}
}

// StartRecording begins audio input recording
func (aim *AudioInputManager) StartRecording() error {
	aim.mu.Lock()
	defer aim.mu.Unlock()
	
	if aim.isRecording {
		return fmt.Errorf("recording already in progress")
	}
	
	// Create recording context
	aim.recordingContext, aim.recordingCancel = context.WithCancel(context.Background())
	
	// Start recording process based on platform
	var err error
	if aim.isMacOS() {
		err = aim.startMacOSRecording()
	} else {
		err = aim.startLinuxRecording()
	}
	
	if err != nil {
		aim.recordingCancel()
		return fmt.Errorf("failed to start recording: %w", err)
	}
	
	aim.isRecording = true
	log.Printf("[AUDIO_INPUT] Started recording with device: %s, sample rate: %d, channels: %d", 
		aim.inputDevice, aim.sampleRate, aim.channels)
	
	// Start processing goroutine
	go aim.processAudioInput()
	
	// Notify UI
	if aim.uiUpdateChan != nil {
		aim.uiUpdateChan <- AudioInputStartedMsg{}
	}
	
	return nil
}

// StopRecording stops audio input recording
func (aim *AudioInputManager) StopRecording() error {
	aim.mu.Lock()
	defer aim.mu.Unlock()
	
	if !aim.isRecording {
		return fmt.Errorf("no recording in progress")
	}
	
	// Cancel recording context
	if aim.recordingCancel != nil {
		aim.recordingCancel()
	}
	
	// Stop recording process
	if aim.recordingProcess != nil {
		if err := aim.recordingProcess.Process.Signal(os.Interrupt); err != nil {
			log.Printf("[AUDIO_INPUT] Error stopping recording process: %v", err)
			// Force kill if interrupt fails
			aim.recordingProcess.Process.Kill()
		}
		aim.recordingProcess.Wait()
		aim.recordingProcess = nil
	}
	
	aim.isRecording = false
	log.Printf("[AUDIO_INPUT] Stopped recording")
	
	// Notify UI
	if aim.uiUpdateChan != nil {
		aim.uiUpdateChan <- AudioInputStoppedMsg{}
	}
	
	return nil
}

// IsRecording returns whether recording is active
func (aim *AudioInputManager) IsRecording() bool {
	aim.mu.RLock()
	defer aim.mu.RUnlock()
	return aim.isRecording
}

// GetAudioInputChannel returns the channel for receiving audio input chunks
func (aim *AudioInputManager) GetAudioInputChannel() <-chan AudioInputChunk {
	return aim.audioInputChan
}

// startMacOSRecording starts recording on macOS using sox
func (aim *AudioInputManager) startMacOSRecording() error {
	// Check if sox is available
	if _, err := exec.LookPath("sox"); err != nil {
		return fmt.Errorf("sox not found - install with: brew install sox")
	}
	
	// sox command for macOS: sox -d -t raw -r 48000 -c 1 -e s -b 16 -
	args := []string{
		"-d",              // Use default input device
		"-t", "raw",       // Raw output format
		"-r", fmt.Sprintf("%d", aim.sampleRate), // Sample rate
		"-c", fmt.Sprintf("%d", aim.channels),   // Channels
		"-e", "s",         // Signed encoding
		"-b", "16",        // 16-bit
		"-",               // Output to stdout
	}
	
	aim.recordingProcess = exec.CommandContext(aim.recordingContext, "sox", args...)
	
	// Set up stdout pipe for audio data
	stdout, err := aim.recordingProcess.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	
	// Start the process
	if err := aim.recordingProcess.Start(); err != nil {
		return fmt.Errorf("failed to start sox process: %w", err)
	}
	
	// Start reading audio data
	go aim.readAudioData(stdout)
	
	return nil
}

// startLinuxRecording starts recording on Linux using arecord
func (aim *AudioInputManager) startLinuxRecording() error {
	// Check if arecord is available
	if _, err := exec.LookPath("arecord"); err != nil {
		return fmt.Errorf("arecord not found - install with: sudo apt-get install alsa-utils")
	}
	
	// arecord command: arecord -f S16_LE -r 48000 -c 1 -t raw
	args := []string{
		"-f", "S16_LE",    // 16-bit signed little-endian
		"-r", fmt.Sprintf("%d", aim.sampleRate), // Sample rate
		"-c", fmt.Sprintf("%d", aim.channels),   // Channels
		"-t", "raw",       // Raw format
		"-",               // Output to stdout
	}
	
	aim.recordingProcess = exec.CommandContext(aim.recordingContext, "arecord", args...)
	
	// Set up stdout pipe for audio data
	stdout, err := aim.recordingProcess.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	
	// Start the process
	if err := aim.recordingProcess.Start(); err != nil {
		return fmt.Errorf("failed to start arecord process: %w", err)
	}
	
	// Start reading audio data
	go aim.readAudioData(stdout)
	
	return nil
}

// readAudioData reads audio data from the recording process
func (aim *AudioInputManager) readAudioData(reader io.ReadCloser) {
	defer reader.Close()
	
	buffer := make([]byte, aim.bufferSize)
	
	for {
		select {
		case <-aim.recordingContext.Done():
			return
		default:
			n, err := reader.Read(buffer)
			if err != nil {
				if err != io.EOF {
					log.Printf("[AUDIO_INPUT] Error reading audio data: %v", err)
				}
				return
			}
			
			if n > 0 {
				// Create audio chunk
				chunk := AudioInputChunk{
					Data:       make([]byte, n),
					Timestamp:  time.Now(),
					Duration:   aim.chunkDuration,
					SampleRate: aim.sampleRate,
					Channels:   aim.channels,
					Format:     aim.format,
				}
				copy(chunk.Data, buffer[:n])
				
				// Apply voice activity detection if enabled
				if aim.enableVAD {
					chunk.IsVoice = aim.detectVoiceActivity(chunk.Data)
				} else {
					chunk.IsVoice = true // Assume all audio is voice if VAD disabled
				}
				
				// Send to processing channel
				select {
				case aim.audioInputChan <- chunk:
					if helpers.IsAudioTraceEnabled() {
						log.Printf("[AUDIO_INPUT] Captured chunk: %d bytes, voice: %v", n, chunk.IsVoice)
					}
				default:
					log.Printf("[AUDIO_INPUT] Warning: audio input buffer full, dropping chunk")
				}
			}
		}
	}
}

// detectVoiceActivity performs simple voice activity detection
func (aim *AudioInputManager) detectVoiceActivity(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	
	// Calculate RMS energy for voice detection
	var sum int64
	samples := len(data) / 2 // 16-bit samples
	
	for i := 0; i < len(data)-1; i += 2 {
		sample := int16(data[i]) | int16(data[i+1])<<8
		sum += int64(sample) * int64(sample)
	}
	
	if samples == 0 {
		return false
	}
	
	rms := float64(sum) / float64(samples)
	energy := rms / (32768 * 32768) // Normalize to 0-1 range
	
	return energy > aim.silenceThreshold
}

// processAudioInput processes audio input chunks
func (aim *AudioInputManager) processAudioInput() {
	log.Printf("[AUDIO_INPUT] Started audio input processing goroutine")
	
	for {
		select {
		case <-aim.recordingContext.Done():
			log.Printf("[AUDIO_INPUT] Audio input processing stopped")
			return
		case chunk, ok := <-aim.audioInputChan:
			if !ok {
				return
			}
			
			// Only process chunks with voice activity if VAD is enabled
			if aim.enableVAD && !chunk.IsVoice {
				continue
			}
			
			// Send to UI for processing
			if aim.uiUpdateChan != nil {
				aim.uiUpdateChan <- AudioInputChunkMsg{Chunk: chunk}
			}
		}
	}
}

// isMacOS checks if running on macOS
func (aim *AudioInputManager) isMacOS() bool {
	return exec.Command("uname", "-s").Run() == nil
}

// Audio input related messages
type AudioInputStartedMsg struct{}
type AudioInputStoppedMsg struct{}
type AudioInputChunkMsg struct {
	Chunk AudioInputChunk
}
type AudioInputErrorMsg struct {
	Error error
}

// ToggleRecording toggles recording state
func (aim *AudioInputManager) ToggleRecording() error {
	if aim.IsRecording() {
		return aim.StopRecording()
	}
	return aim.StartRecording()
}

// GetInputDevices returns available input devices (platform-specific)
func (aim *AudioInputManager) GetInputDevices() ([]string, error) {
	if aim.isMacOS() {
		return aim.getMacOSInputDevices()
	}
	return aim.getLinuxInputDevices()
}

// getMacOSInputDevices gets available input devices on macOS
func (aim *AudioInputManager) getMacOSInputDevices() ([]string, error) {
	// Use system_profiler to get audio devices
	cmd := exec.Command("system_profiler", "SPAudioDataType")
	output, err := cmd.Output()
	if err != nil {
		return []string{"default"}, nil // Fallback to default
	}
	
	// Parse output (simplified - would need more robust parsing)
	devices := []string{"default"}
	if len(output) > 0 {
		// Add system default as primary option
		devices = append(devices, "Built-in Microphone")
	}
	
	return devices, nil
}

// getLinuxInputDevices gets available input devices on Linux
func (aim *AudioInputManager) getLinuxInputDevices() ([]string, error) {
	// Use arecord to list devices
	cmd := exec.Command("arecord", "-l")
	output, err := cmd.Output()
	if err != nil {
		return []string{"default"}, nil // Fallback to default
	}
	
	// Parse output (simplified)
	devices := []string{"default"}
	if len(output) > 0 {
		// Add common device names
		devices = append(devices, "hw:0,0", "plughw:0,0")
	}
	
	return devices, nil
}

// SetInputDevice sets the input device
func (aim *AudioInputManager) SetInputDevice(device string) error {
	aim.mu.Lock()
	defer aim.mu.Unlock()
	
	if aim.isRecording {
		return fmt.Errorf("cannot change input device while recording")
	}
	
	aim.inputDevice = device
	log.Printf("[AUDIO_INPUT] Set input device to: %s", device)
	return nil
}

// GetConfig returns current audio input configuration
func (aim *AudioInputManager) GetConfig() AudioInputConfig {
	aim.mu.RLock()
	defer aim.mu.RUnlock()
	
	return AudioInputConfig{
		InputDevice:      aim.inputDevice,
		SampleRate:       aim.sampleRate,
		Channels:         aim.channels,
		Format:           aim.format,
		BufferSize:       aim.bufferSize,
		EnableVAD:        aim.enableVAD,
		SilenceThreshold: aim.silenceThreshold,
		ChunkDuration:    aim.chunkDuration,
	}
}

// UpdateConfig updates audio input configuration
func (aim *AudioInputManager) UpdateConfig(config AudioInputConfig) error {
	aim.mu.Lock()
	defer aim.mu.Unlock()
	
	if aim.isRecording {
		return fmt.Errorf("cannot update configuration while recording")
	}
	
	aim.inputDevice = config.InputDevice
	aim.sampleRate = config.SampleRate
	aim.channels = config.Channels
	aim.format = config.Format
	aim.bufferSize = config.BufferSize
	aim.enableVAD = config.EnableVAD
	aim.silenceThreshold = config.SilenceThreshold
	aim.chunkDuration = config.ChunkDuration
	
	log.Printf("[AUDIO_INPUT] Updated configuration: device=%s, rate=%d, channels=%d", 
		aim.inputDevice, aim.sampleRate, aim.channels)
	
	return nil
}