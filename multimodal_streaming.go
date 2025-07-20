package aistudio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tmc/aistudio/api"
	"github.com/tmc/aistudio/internal/helpers"
)

// MultimodalStreamingManager manages audio and image streaming to live models
type MultimodalStreamingManager struct {
	// Streaming components
	audioInputManager  *AudioInputManager
	imageCaptureManager *ImageCaptureManager
	
	// API integration
	apiClient    *api.Client
	liveClient   *api.LiveClient
	streamConfig *api.StreamClientConfig
	
	// Streaming state
	isStreaming      bool
	streamingContext context.Context
	streamingCancel  context.CancelFunc
	
	// Configuration
	config           MultimodalConfig
	uiUpdateChan     chan tea.Msg
	
	// Synchronization
	mu               sync.RWMutex
	
	// Data processing
	audioBuffer      []AudioInputChunk
	imageBuffer      []ImageFrame
	lastImageSent    time.Time
	lastAudioSent    time.Time
	
	// Streaming metrics
	audioChunksSent  int64
	imageFramesSent  int64
	bytesStreamed    int64
	streamStartTime  time.Time
}

// MultimodalConfig contains configuration for multimodal streaming
type MultimodalConfig struct {
	// Audio configuration
	AudioConfig       AudioInputConfig
	EnableAudio       bool
	AudioVADEnabled   bool
	AudioChunkSize    int
	AudioSendInterval time.Duration
	
	// Image configuration
	ImageConfig       ImageCaptureConfig
	EnableImages      bool
	ImageSendInterval time.Duration
	ImageQuality      int
	ImageMaxSize      int
	
	// Streaming configuration
	StreamingMode     string // "continuous", "voice_activated", "manual"
	MaxBufferSize     int
	EnableCompression bool
	
	// Live API configuration
	ModelName         string
	Temperature       float32
	MaxOutputTokens   int32
	SystemPrompt      string
}

// DefaultMultimodalConfig returns default configuration
func DefaultMultimodalConfig() MultimodalConfig {
	return MultimodalConfig{
		AudioConfig:       DefaultAudioInputConfig(),
		EnableAudio:       true,
		AudioVADEnabled:   true,
		AudioChunkSize:    4096,
		AudioSendInterval: 500 * time.Millisecond,
		
		ImageConfig:       DefaultImageCaptureConfig(),
		EnableImages:      true,
		ImageSendInterval: 2 * time.Second,
		ImageQuality:      75,
		ImageMaxSize:      1024 * 1024, // 1MB max
		
		StreamingMode:     "voice_activated",
		MaxBufferSize:     50,
		EnableCompression: true,
		
		ModelName:         "models/gemini-2.0-flash-live-001",
		Temperature:       0.7,
		MaxOutputTokens:   8192,
		SystemPrompt:      "You are a helpful assistant that can see the user's screen and hear their voice. Respond naturally to both visual and audio input.",
	}
}

// NewMultimodalStreamingManager creates a new multimodal streaming manager
func NewMultimodalStreamingManager(config MultimodalConfig, apiClient *api.Client, uiUpdateChan chan tea.Msg) *MultimodalStreamingManager {
	manager := &MultimodalStreamingManager{
		config:       config,
		apiClient:    apiClient,
		uiUpdateChan: uiUpdateChan,
		audioBuffer:  make([]AudioInputChunk, 0, config.MaxBufferSize),
		imageBuffer:  make([]ImageFrame, 0, config.MaxBufferSize),
	}
	
	// Initialize audio input manager if enabled
	if config.EnableAudio {
		manager.audioInputManager = NewAudioInputManager(config.AudioConfig, uiUpdateChan)
	}
	
	// Initialize image capture manager if enabled
	if config.EnableImages {
		manager.imageCaptureManager = NewImageCaptureManager(config.ImageConfig, uiUpdateChan)
	}
	
	return manager
}

// StartStreaming begins multimodal streaming
func (msm *MultimodalStreamingManager) StartStreaming() error {
	msm.mu.Lock()
	defer msm.mu.Unlock()
	
	if msm.isStreaming {
		return fmt.Errorf("streaming already in progress")
	}
	
	// Handle window selection if specified
	if err := msm.handleWindowSelection(); err != nil {
		return fmt.Errorf("failed to handle window selection: %w", err)
	}
	
	// Initialize live client
	if err := msm.initializeLiveClient(); err != nil {
		return fmt.Errorf("failed to initialize live client: %w", err)
	}
	
	// Create streaming context
	msm.streamingContext, msm.streamingCancel = context.WithCancel(context.Background())
	
	// Start audio input if enabled
	if msm.config.EnableAudio && msm.audioInputManager != nil {
		if err := msm.audioInputManager.StartRecording(); err != nil {
			msm.streamingCancel()
			return fmt.Errorf("failed to start audio input: %w", err)
		}
	}
	
	// Start image capture if enabled
	if msm.config.EnableImages && msm.imageCaptureManager != nil {
		if err := msm.imageCaptureManager.StartScreenCapture(); err != nil {
			msm.streamingCancel()
			return fmt.Errorf("failed to start image capture: %w", err)
		}
	}
	
	msm.isStreaming = true
	msm.streamStartTime = time.Now()
	
	log.Printf("[MULTIMODAL] Started streaming with audio=%v, images=%v, mode=%s", 
		msm.config.EnableAudio, msm.config.EnableImages, msm.config.StreamingMode)
	
	// Start processing goroutines
	go msm.processAudioInput()
	go msm.processImageInput()
	go msm.streamingLoop()
	
	// Notify UI
	if msm.uiUpdateChan != nil {
		msm.uiUpdateChan <- MultimodalStreamingStartedMsg{}
	}
	
	return nil
}

// StopStreaming stops multimodal streaming
func (msm *MultimodalStreamingManager) StopStreaming() error {
	msm.mu.Lock()
	defer msm.mu.Unlock()
	
	if !msm.isStreaming {
		return fmt.Errorf("no streaming in progress")
	}
	
	// Cancel streaming context
	if msm.streamingCancel != nil {
		msm.streamingCancel()
	}
	
	// Stop audio input
	if msm.audioInputManager != nil {
		msm.audioInputManager.StopRecording()
	}
	
	// Stop image capture
	if msm.imageCaptureManager != nil {
		msm.imageCaptureManager.StopScreenCapture()
	}
	
	// Close live client
	if msm.liveClient != nil {
		msm.liveClient.Close()
		msm.liveClient = nil
	}
	
	msm.isStreaming = false
	
	duration := time.Since(msm.streamStartTime)
	log.Printf("[MULTIMODAL] Stopped streaming after %v - sent %d audio chunks, %d image frames, %d bytes total", 
		duration, msm.audioChunksSent, msm.imageFramesSent, msm.bytesStreamed)
	
	// Notify UI
	if msm.uiUpdateChan != nil {
		msm.uiUpdateChan <- MultimodalStreamingStoppedMsg{
			Duration:        duration,
			AudioChunksSent: msm.audioChunksSent,
			ImageFramesSent: msm.imageFramesSent,
			BytesStreamed:   msm.bytesStreamed,
		}
	}
	
	return nil
}

// IsStreaming returns whether streaming is active
func (msm *MultimodalStreamingManager) IsStreaming() bool {
	msm.mu.RLock()
	defer msm.mu.RUnlock()
	return msm.isStreaming
}

// initializeLiveClient initializes the live API client
func (msm *MultimodalStreamingManager) initializeLiveClient() error {
	// Create stream client config
	msm.streamConfig = &api.StreamClientConfig{
		ModelName:       msm.config.ModelName,
		EnableWebSocket: true,
		Temperature:     msm.config.Temperature,
		TopP:            0.95,
		TopK:            40,
		MaxOutputTokens: msm.config.MaxOutputTokens,
		SystemPrompt:    msm.config.SystemPrompt,
	}
	
	// Create live client
	var err error
	msm.liveClient, err = api.NewLiveClient(msm.streamingContext, msm.apiClient.APIKey, msm.streamConfig, nil)
	if err != nil {
		return fmt.Errorf("failed to create live client: %w", err)
	}
	
	// Initialize live client
	if err := msm.liveClient.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize live client: %w", err)
	}
	
	// Start receiving responses
	go msm.processLiveResponses()
	
	return nil
}

// processAudioInput processes audio input chunks
func (msm *MultimodalStreamingManager) processAudioInput() {
	if msm.audioInputManager == nil {
		return
	}
	
	audioChan := msm.audioInputManager.GetAudioInputChannel()
	
	for {
		select {
		case <-msm.streamingContext.Done():
			return
		case chunk, ok := <-audioChan:
			if !ok {
				return
			}
			
			// Filter based on voice activity if enabled
			if msm.config.AudioVADEnabled && !chunk.IsVoice {
				continue
			}
			
			// Add to buffer
			msm.mu.Lock()
			if len(msm.audioBuffer) >= msm.config.MaxBufferSize {
				// Remove oldest chunk
				msm.audioBuffer = msm.audioBuffer[1:]
			}
			msm.audioBuffer = append(msm.audioBuffer, chunk)
			msm.mu.Unlock()
			
			if helpers.IsAudioTraceEnabled() {
				log.Printf("[MULTIMODAL] Added audio chunk to buffer: %d bytes, voice=%v", 
					len(chunk.Data), chunk.IsVoice)
			}
		}
	}
}

// processImageInput processes image frames
func (msm *MultimodalStreamingManager) processImageInput() {
	if msm.imageCaptureManager == nil {
		return
	}
	
	imageChan := msm.imageCaptureManager.GetImageChannel()
	
	for {
		select {
		case <-msm.streamingContext.Done():
			return
		case frame, ok := <-imageChan:
			if !ok {
				return
			}
			
			// Check size limits
			if len(frame.Data) > msm.config.ImageMaxSize {
				log.Printf("[MULTIMODAL] Skipping large image frame: %d bytes > %d", 
					len(frame.Data), msm.config.ImageMaxSize)
				continue
			}
			
			// Add to buffer
			msm.mu.Lock()
			if len(msm.imageBuffer) >= msm.config.MaxBufferSize {
				// Remove oldest frame
				msm.imageBuffer = msm.imageBuffer[1:]
			}
			msm.imageBuffer = append(msm.imageBuffer, frame)
			msm.mu.Unlock()
			
			log.Printf("[MULTIMODAL] Added image frame to buffer: %d bytes, format=%s", 
				len(frame.Data), frame.Format)
		}
	}
}

// streamingLoop manages the streaming of audio and image data
func (msm *MultimodalStreamingManager) streamingLoop() {
	audioTicker := time.NewTicker(msm.config.AudioSendInterval)
	imageTicker := time.NewTicker(msm.config.ImageSendInterval)
	
	defer audioTicker.Stop()
	defer imageTicker.Stop()
	
	for {
		select {
		case <-msm.streamingContext.Done():
			return
		case <-audioTicker.C:
			msm.sendAudioData()
		case <-imageTicker.C:
			msm.sendImageData()
		}
	}
}

// sendAudioData sends buffered audio data to the live API
func (msm *MultimodalStreamingManager) sendAudioData() {
	msm.mu.Lock()
	if len(msm.audioBuffer) == 0 {
		msm.mu.Unlock()
		return
	}
	
	// Get chunks to send
	chunksToSend := make([]AudioInputChunk, len(msm.audioBuffer))
	copy(chunksToSend, msm.audioBuffer)
	msm.audioBuffer = msm.audioBuffer[:0] // Clear buffer
	msm.mu.Unlock()
	
	// Combine audio chunks
	var combinedData []byte
	for _, chunk := range chunksToSend {
		combinedData = append(combinedData, chunk.Data...)
	}
	
	if len(combinedData) == 0 {
		return
	}
	
	// Create audio message for live API
	audioMessage := map[string]interface{}{
		"realtime_input": map[string]interface{}{
			"media_chunks": []map[string]interface{}{
				{
					"data":      base64.StdEncoding.EncodeToString(combinedData),
					"mime_type": "audio/pcm",
				},
			},
		},
	}
	
	// Send to live API
	if err := msm.sendToLiveAPI(audioMessage); err != nil {
		log.Printf("[MULTIMODAL] Error sending audio data: %v", err)
	} else {
		msm.audioChunksSent++
		msm.bytesStreamed += int64(len(combinedData))
		msm.lastAudioSent = time.Now()
		
		if helpers.IsAudioTraceEnabled() {
			log.Printf("[MULTIMODAL] Sent audio data: %d bytes from %d chunks", 
				len(combinedData), len(chunksToSend))
		}
	}
}

// sendImageData sends buffered image data to the live API
func (msm *MultimodalStreamingManager) sendImageData() {
	msm.mu.Lock()
	if len(msm.imageBuffer) == 0 {
		msm.mu.Unlock()
		return
	}
	
	// Get the latest frame
	frame := msm.imageBuffer[len(msm.imageBuffer)-1]
	msm.imageBuffer = msm.imageBuffer[:0] // Clear buffer
	msm.mu.Unlock()
	
	// Create image message for live API
	mimeType := fmt.Sprintf("image/%s", frame.Format)
	imageMessage := map[string]interface{}{
		"realtime_input": map[string]interface{}{
			"media_chunks": []map[string]interface{}{
				{
					"data":      base64.StdEncoding.EncodeToString(frame.Data),
					"mime_type": mimeType,
				},
			},
		},
	}
	
	// Send to live API
	if err := msm.sendToLiveAPI(imageMessage); err != nil {
		log.Printf("[MULTIMODAL] Error sending image data: %v", err)
	} else {
		msm.imageFramesSent++
		msm.bytesStreamed += int64(len(frame.Data))
		msm.lastImageSent = time.Now()
		
		log.Printf("[MULTIMODAL] Sent image frame: %d bytes, format=%s", 
			len(frame.Data), frame.Format)
	}
}

// sendToLiveAPI sends data to the live API
func (msm *MultimodalStreamingManager) sendToLiveAPI(message map[string]interface{}) error {
	if msm.liveClient == nil {
		return fmt.Errorf("live client not initialized")
	}
	
	// Convert message to JSON
	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	
	// Send message
	return msm.liveClient.SendRawMessage(jsonData)
}

// processLiveResponses processes responses from the live API
func (msm *MultimodalStreamingManager) processLiveResponses() {
	for {
		select {
		case <-msm.streamingContext.Done():
			return
		default:
			if msm.liveClient == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			
			// Receive response
			response, err := msm.liveClient.ReceiveMessage()
			if err != nil {
				log.Printf("[MULTIMODAL] Error receiving live response: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			
			// Process response
			msm.processLiveResponse(response)
		}
	}
}

// processLiveResponse processes a single response from the live API
func (msm *MultimodalStreamingManager) processLiveResponse(response *api.StreamOutput) {
	if response == nil {
		return
	}
	
	// Handle text responses
	if response.Text != "" {
		if msm.uiUpdateChan != nil {
			msm.uiUpdateChan <- MultimodalResponseMsg{
				Type:    "text",
				Content: response.Text,
			}
		}
	}
	
	// Handle audio responses
	if response.Audio != nil && len(response.Audio) > 0 {
		if msm.uiUpdateChan != nil {
			msm.uiUpdateChan <- MultimodalResponseMsg{
				Type:      "audio",
				Content:   "",
				AudioData: response.Audio,
			}
		}
	}
	
	// Handle function calls
	if response.FunctionCall != nil {
		if msm.uiUpdateChan != nil {
			msm.uiUpdateChan <- MultimodalResponseMsg{
				Type:         "function_call",
				Content:      response.FunctionCall.Name,
				FunctionCall: response.FunctionCall,
			}
		}
	}
}

// SendTextMessage sends a text message to the live API
func (msm *MultimodalStreamingManager) SendTextMessage(text string) error {
	if msm.liveClient == nil {
		return fmt.Errorf("live client not initialized")
	}
	
	return msm.liveClient.SendMessage(text)
}

// SendImageFile sends an image file to the live API
func (msm *MultimodalStreamingManager) SendImageFile(filename string) error {
	if msm.imageCaptureManager == nil {
		return fmt.Errorf("image capture manager not initialized")
	}
	
	frame, err := msm.imageCaptureManager.CaptureFromFile(filename)
	if err != nil {
		return fmt.Errorf("failed to load image file: %w", err)
	}
	
	// Create image message
	mimeType := fmt.Sprintf("image/%s", frame.Format)
	imageMessage := map[string]interface{}{
		"realtime_input": map[string]interface{}{
			"media_chunks": []map[string]interface{}{
				{
					"data":      base64.StdEncoding.EncodeToString(frame.Data),
					"mime_type": mimeType,
				},
			},
		},
	}
	
	return msm.sendToLiveAPI(imageMessage)
}

// CaptureScreenNow captures and sends a screen shot immediately
func (msm *MultimodalStreamingManager) CaptureScreenNow() error {
	if msm.imageCaptureManager == nil {
		return fmt.Errorf("image capture manager not initialized")
	}
	
	frame, err := msm.imageCaptureManager.CaptureScreenOnce()
	if err != nil {
		return fmt.Errorf("failed to capture screen: %w", err)
	}
	
	// Create image message
	mimeType := fmt.Sprintf("image/%s", frame.Format)
	imageMessage := map[string]interface{}{
		"realtime_input": map[string]interface{}{
			"media_chunks": []map[string]interface{}{
				{
					"data":      base64.StdEncoding.EncodeToString(frame.Data),
					"mime_type": mimeType,
				},
			},
		},
	}
	
	return msm.sendToLiveAPI(imageMessage)
}

// GetStreamingStats returns current streaming statistics
func (msm *MultimodalStreamingManager) GetStreamingStats() MultimodalStreamingStats {
	msm.mu.RLock()
	defer msm.mu.RUnlock()
	
	var duration time.Duration
	if msm.isStreaming {
		duration = time.Since(msm.streamStartTime)
	}
	
	return MultimodalStreamingStats{
		IsStreaming:     msm.isStreaming,
		Duration:        duration,
		AudioChunksSent: msm.audioChunksSent,
		ImageFramesSent: msm.imageFramesSent,
		BytesStreamed:   msm.bytesStreamed,
		LastAudioSent:   msm.lastAudioSent,
		LastImageSent:   msm.lastImageSent,
		AudioBufferSize: len(msm.audioBuffer),
		ImageBufferSize: len(msm.imageBuffer),
	}
}

// UpdateConfig updates the multimodal configuration
func (msm *MultimodalStreamingManager) UpdateConfig(config MultimodalConfig) error {
	msm.mu.Lock()
	defer msm.mu.Unlock()
	
	if msm.isStreaming {
		return fmt.Errorf("cannot update configuration while streaming")
	}
	
	msm.config = config
	
	// Update component configurations
	if msm.audioInputManager != nil {
		msm.audioInputManager.UpdateConfig(config.AudioConfig)
	}
	
	if msm.imageCaptureManager != nil {
		msm.imageCaptureManager.UpdateConfig(config.ImageConfig)
	}
	
	log.Printf("[MULTIMODAL] Updated configuration: audio=%v, images=%v, mode=%s", 
		config.EnableAudio, config.EnableImages, config.StreamingMode)
	
	return nil
}

// Multimodal streaming related messages
type MultimodalStreamingStartedMsg struct{}
type MultimodalStreamingStoppedMsg struct {
	Duration        time.Duration
	AudioChunksSent int64
	ImageFramesSent int64
	BytesStreamed   int64
}
type MultimodalResponseMsg struct {
	Type         string
	Content      string
	AudioData    []byte
	FunctionCall *generativelanguagepb.FunctionCall
}
type MultimodalStreamingStats struct {
	IsStreaming     bool
	Duration        time.Duration
	AudioChunksSent int64
	ImageFramesSent int64
	BytesStreamed   int64
	LastAudioSent   time.Time
	LastImageSent   time.Time
	AudioBufferSize int
	ImageBufferSize int
}

// GetAudioInputManager returns the audio input manager
func (msm *MultimodalStreamingManager) GetAudioInputManager() *AudioInputManager {
	return msm.audioInputManager
}

// GetImageCaptureManager returns the image capture manager
func (msm *MultimodalStreamingManager) GetImageCaptureManager() *ImageCaptureManager {
	return msm.imageCaptureManager
}

// SetStreamingMode sets the streaming mode
func (msm *MultimodalStreamingManager) SetStreamingMode(mode string) error {
	msm.mu.Lock()
	defer msm.mu.Unlock()
	
	validModes := []string{"continuous", "voice_activated", "manual"}
	for _, validMode := range validModes {
		if mode == validMode {
			msm.config.StreamingMode = mode
			log.Printf("[MULTIMODAL] Set streaming mode to: %s", mode)
			return nil
		}
	}
	
	return fmt.Errorf("invalid streaming mode: %s (valid: %v)", mode, validModes)
}

// ToggleStreaming toggles streaming state
func (msm *MultimodalStreamingManager) ToggleStreaming() error {
	if msm.IsStreaming() {
		return msm.StopStreaming()
	}
	return msm.StartStreaming()
}

// ToggleAudio toggles audio input
func (msm *MultimodalStreamingManager) ToggleAudio() error {
	if msm.audioInputManager == nil {
		return fmt.Errorf("audio input manager not initialized")
	}
	
	return msm.audioInputManager.ToggleRecording()
}

// ToggleImages toggles image capture
func (msm *MultimodalStreamingManager) ToggleImages() error {
	if msm.imageCaptureManager == nil {
		return fmt.Errorf("image capture manager not initialized")
	}
	
	return msm.imageCaptureManager.ToggleScreenCapture()
}

// GetCurrentCaptureWindow returns the currently set capture window
func (msm *MultimodalStreamingManager) GetCurrentCaptureWindow() string {
	if msm.imageCaptureManager == nil {
		return ""
	}
	return msm.imageCaptureManager.GetCurrentCaptureWindow()
}

// handleWindowSelection handles window selection logic at startup
func (msm *MultimodalStreamingManager) handleWindowSelection() error {
	// Only handle if images are enabled and we have a capture window specified
	if !msm.config.EnableImages || msm.imageCaptureManager == nil {
		return nil
	}
	
	captureWindow := msm.config.ImageConfig.CaptureWindow
	if captureWindow == "" {
		return nil
	}
	
	// Check if the capture window is a window ID (starts with number or 0x)
	if isWindowID(captureWindow) {
		// Direct window ID, use as-is
		return msm.imageCaptureManager.SetCaptureWindow(captureWindow)
	}
	
	// Try to find window by name first
	if window, err := msm.imageCaptureManager.GetWindowByName(captureWindow); err == nil {
		log.Printf("[MULTIMODAL] Found window by name: %s -> %s", captureWindow, window.ID)
		return msm.imageCaptureManager.SetCaptureWindow(window.ID)
	}
	
	// Try to find window by process name
	if window, err := msm.imageCaptureManager.GetWindowByProcess(captureWindow); err == nil {
		log.Printf("[MULTIMODAL] Found window by process: %s -> %s", captureWindow, window.ID)
		return msm.imageCaptureManager.SetCaptureWindow(window.ID)
	}
	
	// If we can't find the window, log a warning but continue
	log.Printf("[MULTIMODAL] Warning: Could not find window or process '%s', using full screen", captureWindow)
	return msm.imageCaptureManager.ClearCaptureWindow()
}

// isWindowID checks if a string looks like a window ID
func isWindowID(s string) bool {
	if len(s) == 0 {
		return false
	}
	
	// Check for hex format (0x...)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return true
	}
	
	// Check if it's all digits
	for _, char := range s {
		if char < '0' || char > '9' {
			return false
		}
	}
	
	return true
}