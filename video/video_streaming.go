package video

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// VideoStreamingManager handles real-time video streaming with adaptive quality
type VideoStreamingManager struct {
	// Configuration
	config           VideoStreamingConfig
	isStreaming      bool
	streamingContext context.Context
	streamingCancel  context.CancelFunc
	
	// Video pipeline
	frameInput       chan VideoFrame
	frameOutput      chan ProcessedVideoFrame
	uiUpdateChan     chan tea.Msg
	
	// Adaptive quality
	currentFPS       int
	currentBitrate   int
	currentResolution VideoResolution
	networkMetrics   NetworkMetrics
	
	// Codec management
	currentCodec     VideoCodec
	codecInstances   map[VideoCodec]*CodecInstance
	
	// Synchronization
	mu               sync.RWMutex
	
	// Performance tracking
	frameCount       int64
	droppedFrames    int64
	lastFrameTime    time.Time
	avgProcessTime   time.Duration
	
	// Quality adaptation
	qualityController *QualityController
	performanceMonitor *PerformanceMonitor
}

// VideoStreamingConfig contains configuration for video streaming
type VideoStreamingConfig struct {
	// Basic settings
	OutputFormat      string
	EnableStreaming   bool
	StreamingURL      string
	
	// Quality settings
	InitialFPS        int
	MinFPS           int
	MaxFPS           int
	InitialBitrate   int
	MinBitrate       int
	MaxBitrate       int
	InitialResolution VideoResolution
	SupportedResolutions []VideoResolution
	
	// Codec settings
	PreferredCodec   VideoCodec
	FallbackCodecs   []VideoCodec
	HardwareAccel    bool
	
	// Adaptation settings
	EnableAdaptiveQuality bool
	QualityAdaptInterval  time.Duration
	NetworkTestInterval   time.Duration
	
	// Buffer settings
	MaxBufferSize    int
	MinBufferSize    int
	TargetLatency    time.Duration
	
	// Advanced settings
	EnableRealTimeMode bool
	EnableLowLatency   bool
	EnableQualityLogging bool
	ThreadCount        int
}

// VideoResolution represents video resolution configuration
type VideoResolution struct {
	Width  int
	Height int
	Name   string // e.g., "1080p", "720p", "480p"
}

// VideoCodec represents supported video codecs
type VideoCodec string

const (
	CodecH264 VideoCodec = "h264"
	CodecH265 VideoCodec = "h265"
	CodecVP8  VideoCodec = "vp8"
	CodecVP9  VideoCodec = "vp9"
	CodecAV1  VideoCodec = "av1"
)

// VideoFrame represents a single video frame
type VideoFrame struct {
	Data        []byte
	Width       int
	Height      int
	Format      string
	Timestamp   time.Time
	FrameNumber int64
	Source      string
}

// ProcessedVideoFrame represents a processed video frame ready for streaming
type ProcessedVideoFrame struct {
	Data            []byte
	Width           int
	Height          int
	Format          string
	Timestamp       time.Time
	FrameNumber     int64
	ProcessingTime  time.Duration
	CompressionRatio float64
	QualityScore    float64
	Bitrate         int
	Codec           VideoCodec
}

// NetworkMetrics tracks network performance for quality adaptation
type NetworkMetrics struct {
	Bandwidth       int64  // bytes per second
	Latency         time.Duration
	PacketLoss      float64
	Jitter          time.Duration
	LastUpdated     time.Time
	ThroughputHistory []int64
}

// CodecInstance represents an active codec encoder/decoder
type CodecInstance struct {
	Codec           VideoCodec
	EncoderProcess  *exec.Cmd
	DecoderProcess  *exec.Cmd
	InputPipe       *os.File
	OutputPipe      *os.File
	IsHardwareAccel bool
	ProcessingTime  time.Duration
	QualityLevel    int
}

// QualityController manages adaptive quality based on performance metrics
type QualityController struct {
	targetFPS        int
	targetBitrate    int
	targetResolution VideoResolution
	adaptationHistory []QualityAdaptation
	lastAdaptation   time.Time
	mu               sync.RWMutex
}

// QualityAdaptation represents a quality adaptation decision
type QualityAdaptation struct {
	Timestamp    time.Time
	FromFPS      int
	ToFPS        int
	FromBitrate  int
	ToBitrate    int
	FromRes      VideoResolution
	ToRes        VideoResolution
	Reason       string
	Success      bool
}

// PerformanceMonitor tracks system performance metrics
type PerformanceMonitor struct {
	CPUUsage         float64
	MemoryUsage      float64
	GPUUsage         float64
	NetworkBandwidth int64
	DiskIO           int64
	LastUpdate       time.Time
	mu               sync.RWMutex
}

// Predefined resolutions
var (
	Resolution4K    = VideoResolution{Width: 3840, Height: 2160, Name: "4K"}
	Resolution1080p = VideoResolution{Width: 1920, Height: 1080, Name: "1080p"}
	Resolution720p  = VideoResolution{Width: 1280, Height: 720, Name: "720p"}
	Resolution480p  = VideoResolution{Width: 854, Height: 480, Name: "480p"}
	Resolution360p  = VideoResolution{Width: 640, Height: 360, Name: "360p"}
)

// DefaultVideoStreamingConfig returns default configuration for video streaming
func DefaultVideoStreamingConfig() VideoStreamingConfig {
	return VideoStreamingConfig{
		OutputFormat:      "mp4",
		EnableStreaming:   true,
		StreamingURL:      "",
		
		InitialFPS:        30,
		MinFPS:           15,
		MaxFPS:           60,
		InitialBitrate:   2000000, // 2 Mbps
		MinBitrate:       500000,  // 500 Kbps
		MaxBitrate:       10000000, // 10 Mbps
		InitialResolution: Resolution1080p,
		SupportedResolutions: []VideoResolution{
			Resolution4K, Resolution1080p, Resolution720p, Resolution480p, Resolution360p,
		},
		
		PreferredCodec: CodecH264,
		FallbackCodecs: []VideoCodec{CodecVP8, CodecVP9, CodecH265},
		HardwareAccel:  true,
		
		EnableAdaptiveQuality: true,
		QualityAdaptInterval:  5 * time.Second,
		NetworkTestInterval:   10 * time.Second,
		
		MaxBufferSize:    100,
		MinBufferSize:    10,
		TargetLatency:    100 * time.Millisecond,
		
		EnableRealTimeMode:   true,
		EnableLowLatency:     true,
		EnableQualityLogging: false,
		ThreadCount:          4,
	}
}

// NewVideoStreamingManager creates a new video streaming manager
func NewVideoStreamingManager(config VideoStreamingConfig, uiUpdateChan chan tea.Msg) *VideoStreamingManager {
	vsm := &VideoStreamingManager{
		config:           config,
		frameInput:       make(chan VideoFrame, config.MaxBufferSize),
		frameOutput:      make(chan ProcessedVideoFrame, config.MaxBufferSize),
		uiUpdateChan:     uiUpdateChan,
		currentFPS:       config.InitialFPS,
		currentBitrate:   config.InitialBitrate,
		currentResolution: config.InitialResolution,
		currentCodec:     config.PreferredCodec,
		codecInstances:   make(map[VideoCodec]*CodecInstance),
		networkMetrics:   NetworkMetrics{
			LastUpdated: time.Now(),
			ThroughputHistory: make([]int64, 0, 10),
		},
	}
	
	// Initialize quality controller
	vsm.qualityController = &QualityController{
		targetFPS:        config.InitialFPS,
		targetBitrate:    config.InitialBitrate,
		targetResolution: config.InitialResolution,
		adaptationHistory: make([]QualityAdaptation, 0, 100),
	}
	
	// Initialize performance monitor
	vsm.performanceMonitor = &PerformanceMonitor{
		LastUpdate: time.Now(),
	}
	
	return vsm
}

// StartStreaming begins video streaming with adaptive quality
func (vsm *VideoStreamingManager) StartStreaming() error {
	vsm.mu.Lock()
	defer vsm.mu.Unlock()
	
	if vsm.isStreaming {
		return fmt.Errorf("video streaming already in progress")
	}
	
	// Verify codec availability
	if err := vsm.verifyCodecSupport(); err != nil {
		return fmt.Errorf("codec verification failed: %w", err)
	}
	
	// Initialize codec instances
	if err := vsm.initializeCodecs(); err != nil {
		return fmt.Errorf("codec initialization failed: %w", err)
	}
	
	// Create streaming context
	vsm.streamingContext, vsm.streamingCancel = context.WithCancel(context.Background())
	
	vsm.isStreaming = true
	vsm.frameCount = 0
	vsm.droppedFrames = 0
	vsm.lastFrameTime = time.Now()
	
	log.Printf("[VIDEO_STREAMING] Started streaming: %s at %dx%d, %d fps, %d bps, codec: %s",
		vsm.config.OutputFormat, vsm.currentResolution.Width, vsm.currentResolution.Height,
		vsm.currentFPS, vsm.currentBitrate, vsm.currentCodec)
	
	// Start processing goroutines
	go vsm.frameProcessingLoop()
	go vsm.qualityAdaptationLoop()
	go vsm.networkMonitoringLoop()
	go vsm.performanceMonitoringLoop()
	
	// Notify UI
	if vsm.uiUpdateChan != nil {
		vsm.uiUpdateChan <- VideoStreamingStartedMsg{
			Resolution: vsm.currentResolution,
			FPS:        vsm.currentFPS,
			Bitrate:    vsm.currentBitrate,
			Codec:      vsm.currentCodec,
		}
	}
	
	return nil
}

// StopStreaming stops video streaming
func (vsm *VideoStreamingManager) StopStreaming() error {
	vsm.mu.Lock()
	defer vsm.mu.Unlock()
	
	if !vsm.isStreaming {
		return fmt.Errorf("no video streaming in progress")
	}
	
	// Cancel streaming context
	if vsm.streamingCancel != nil {
		vsm.streamingCancel()
	}
	
	// Close codec instances
	vsm.shutdownCodecs()
	
	vsm.isStreaming = false
	
	log.Printf("[VIDEO_STREAMING] Stopped streaming - processed %d frames, dropped %d frames",
		vsm.frameCount, vsm.droppedFrames)
	
	// Notify UI
	if vsm.uiUpdateChan != nil {
		vsm.uiUpdateChan <- VideoStreamingStoppedMsg{
			FramesProcessed: vsm.frameCount,
			FramesDropped:   vsm.droppedFrames,
			Duration:        time.Since(vsm.lastFrameTime),
		}
	}
	
	return nil
}

// ProcessFrame processes a single video frame for streaming
func (vsm *VideoStreamingManager) ProcessFrame(frame VideoFrame) error {
	if !vsm.isStreaming {
		return fmt.Errorf("streaming not active")
	}
	
	select {
	case vsm.frameInput <- frame:
		return nil
	case <-vsm.streamingContext.Done():
		return fmt.Errorf("streaming stopped")
	default:
		vsm.droppedFrames++
		return fmt.Errorf("frame buffer full, frame dropped")
	}
}

// GetProcessedFrame retrieves a processed frame from the output channel
func (vsm *VideoStreamingManager) GetProcessedFrame() (ProcessedVideoFrame, error) {
	select {
	case frame := <-vsm.frameOutput:
		return frame, nil
	case <-vsm.streamingContext.Done():
		return ProcessedVideoFrame{}, fmt.Errorf("streaming stopped")
	case <-time.After(vsm.config.TargetLatency * 2):
		return ProcessedVideoFrame{}, fmt.Errorf("frame timeout")
	}
}

// frameProcessingLoop processes incoming video frames
func (vsm *VideoStreamingManager) frameProcessingLoop() {
	log.Printf("[VIDEO_STREAMING] Started frame processing loop")
	
	for {
		select {
		case <-vsm.streamingContext.Done():
			log.Printf("[VIDEO_STREAMING] Frame processing loop stopped")
			return
		case frame := <-vsm.frameInput:
			startTime := time.Now()
			
			// Process frame based on current settings
			processedFrame, err := vsm.processVideoFrame(frame)
			if err != nil {
				log.Printf("[VIDEO_STREAMING] Error processing frame: %v", err)
				continue
			}
			
			// Update metrics
			processingTime := time.Since(startTime)
			vsm.avgProcessTime = (vsm.avgProcessTime + processingTime) / 2
			vsm.frameCount++
			vsm.lastFrameTime = time.Now()
			
			// Send to output channel
			select {
			case vsm.frameOutput <- processedFrame:
				// Success
			default:
				vsm.droppedFrames++
				log.Printf("[VIDEO_STREAMING] Output buffer full, dropped processed frame")
			}
		}
	}
}

// processVideoFrame processes a single video frame with current codec settings
func (vsm *VideoStreamingManager) processVideoFrame(frame VideoFrame) (ProcessedVideoFrame, error) {
	startTime := time.Now()
	
	// Get current codec instance
	codec := vsm.codecInstances[vsm.currentCodec]
	if codec == nil {
		return ProcessedVideoFrame{}, fmt.Errorf("codec instance not available: %s", vsm.currentCodec)
	}
	
	// Resize frame if needed
	resizedFrame, err := vsm.resizeFrame(frame, vsm.currentResolution)
	if err != nil {
		return ProcessedVideoFrame{}, fmt.Errorf("frame resize failed: %w", err)
	}
	
	// Encode frame
	encodedData, err := vsm.encodeFrame(resizedFrame, codec)
	if err != nil {
		return ProcessedVideoFrame{}, fmt.Errorf("frame encoding failed: %w", err)
	}
	
	// Calculate compression ratio
	compressionRatio := float64(len(encodedData)) / float64(len(resizedFrame.Data))
	
	// Estimate quality score (simplified)
	qualityScore := vsm.estimateQualityScore(compressionRatio, vsm.currentBitrate)
	
	processedFrame := ProcessedVideoFrame{
		Data:            encodedData,
		Width:           vsm.currentResolution.Width,
		Height:          vsm.currentResolution.Height,
		Format:          string(vsm.currentCodec),
		Timestamp:       frame.Timestamp,
		FrameNumber:     frame.FrameNumber,
		ProcessingTime:  time.Since(startTime),
		CompressionRatio: compressionRatio,
		QualityScore:    qualityScore,
		Bitrate:         vsm.currentBitrate,
		Codec:           vsm.currentCodec,
	}
	
	return processedFrame, nil
}

// resizeFrame resizes a video frame to target resolution
func (vsm *VideoStreamingManager) resizeFrame(frame VideoFrame, targetRes VideoResolution) (VideoFrame, error) {
	if frame.Width == targetRes.Width && frame.Height == targetRes.Height {
		return frame, nil // No resize needed
	}
	
	// Create temporary files for ffmpeg processing
	inputFile, err := os.CreateTemp("", "video_input_*.raw")
	if err != nil {
		return VideoFrame{}, fmt.Errorf("failed to create temp input file: %w", err)
	}
	defer os.Remove(inputFile.Name())
	
	outputFile, err := os.CreateTemp("", "video_output_*.raw")
	if err != nil {
		return VideoFrame{}, fmt.Errorf("failed to create temp output file: %w", err)
	}
	defer os.Remove(outputFile.Name())
	
	// Write frame data to input file
	if _, err := inputFile.Write(frame.Data); err != nil {
		return VideoFrame{}, fmt.Errorf("failed to write input data: %w", err)
	}
	inputFile.Close()
	
	// Use ffmpeg to resize
	cmd := exec.CommandContext(vsm.streamingContext, "ffmpeg",
		"-y", // Overwrite output
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"-s", fmt.Sprintf("%dx%d", frame.Width, frame.Height),
		"-r", "30",
		"-i", inputFile.Name(),
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
			targetRes.Width, targetRes.Height, targetRes.Width, targetRes.Height),
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		outputFile.Name(),
	)
	
	if err := cmd.Run(); err != nil {
		return VideoFrame{}, fmt.Errorf("ffmpeg resize failed: %w", err)
	}
	
	// Read resized data
	resizedData, err := os.ReadFile(outputFile.Name())
	if err != nil {
		return VideoFrame{}, fmt.Errorf("failed to read resized data: %w", err)
	}
	
	resizedFrame := VideoFrame{
		Data:        resizedData,
		Width:       targetRes.Width,
		Height:      targetRes.Height,
		Format:      frame.Format,
		Timestamp:   frame.Timestamp,
		FrameNumber: frame.FrameNumber,
		Source:      frame.Source,
	}
	
	return resizedFrame, nil
}

// encodeFrame encodes a video frame using the specified codec
func (vsm *VideoStreamingManager) encodeFrame(frame VideoFrame, codec *CodecInstance) ([]byte, error) {
	// Create temporary files for encoding
	inputFile, err := os.CreateTemp("", "encode_input_*.raw")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp input file: %w", err)
	}
	defer os.Remove(inputFile.Name())
	
	outputFile, err := os.CreateTemp("", "encode_output_*."+string(codec.Codec))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp output file: %w", err)
	}
	defer os.Remove(outputFile.Name())
	
	// Write frame data
	if _, err := inputFile.Write(frame.Data); err != nil {
		return nil, fmt.Errorf("failed to write input data: %w", err)
	}
	inputFile.Close()
	
	// Build ffmpeg command based on codec
	args := []string{
		"-y", // Overwrite output
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"-s", fmt.Sprintf("%dx%d", frame.Width, frame.Height),
		"-r", strconv.Itoa(vsm.currentFPS),
		"-i", inputFile.Name(),
	}
	
	// Add codec-specific options
	args = append(args, vsm.getCodecArgs(codec.Codec)...)
	
	// Add bitrate control
	args = append(args,
		"-b:v", strconv.Itoa(vsm.currentBitrate),
		"-maxrate", strconv.Itoa(vsm.currentBitrate*2),
		"-bufsize", strconv.Itoa(vsm.currentBitrate),
	)
	
	// Add hardware acceleration if enabled
	if codec.IsHardwareAccel && vsm.config.HardwareAccel {
		args = append(args, vsm.getHardwareAccelArgs(codec.Codec)...)
	}
	
	// Add output options
	args = append(args,
		"-f", vsm.getOutputFormat(codec.Codec),
		outputFile.Name(),
	)
	
	// Execute encoding
	cmd := exec.CommandContext(vsm.streamingContext, "ffmpeg", args...)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("encoding failed: %w", err)
	}
	
	// Read encoded data
	encodedData, err := os.ReadFile(outputFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read encoded data: %w", err)
	}
	
	return encodedData, nil
}

// getCodecArgs returns codec-specific ffmpeg arguments
func (vsm *VideoStreamingManager) getCodecArgs(codec VideoCodec) []string {
	switch codec {
	case CodecH264:
		return []string{
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-tune", "zerolatency",
			"-profile:v", "baseline",
			"-level", "3.1",
		}
	case CodecH265:
		return []string{
			"-c:v", "libx265",
			"-preset", "ultrafast",
			"-tune", "zerolatency",
		}
	case CodecVP8:
		return []string{
			"-c:v", "libvpx",
			"-speed", "16",
			"-rt",
		}
	case CodecVP9:
		return []string{
			"-c:v", "libvpx-vp9",
			"-speed", "8",
			"-rt",
		}
	default:
		return []string{"-c:v", "libx264"}
	}
}

// getHardwareAccelArgs returns hardware acceleration arguments
func (vsm *VideoStreamingManager) getHardwareAccelArgs(codec VideoCodec) []string {
	switch codec {
	case CodecH264:
		return []string{
			"-hwaccel", "auto",
			"-c:v", "h264_videotoolbox", // macOS
		}
	case CodecH265:
		return []string{
			"-hwaccel", "auto",
			"-c:v", "hevc_videotoolbox", // macOS
		}
	default:
		return []string{}
	}
}

// getOutputFormat returns the output format for a codec
func (vsm *VideoStreamingManager) getOutputFormat(codec VideoCodec) string {
	switch codec {
	case CodecH264, CodecH265:
		return "mp4"
	case CodecVP8, CodecVP9:
		return "webm"
	default:
		return "mp4"
	}
}

// estimateQualityScore estimates quality score based on compression and bitrate
func (vsm *VideoStreamingManager) estimateQualityScore(compressionRatio float64, bitrate int) float64 {
	// Simplified quality estimation
	bitrateScore := float64(bitrate) / float64(vsm.config.MaxBitrate)
	compressionScore := 1.0 - compressionRatio
	
	return (bitrateScore + compressionScore) / 2.0
}

// verifyCodecSupport verifies that required codecs are available
func (vsm *VideoStreamingManager) verifyCodecSupport() error {
	// Check ffmpeg availability
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found: %w", err)
	}
	
	// Test codec support
	codecs := []VideoCodec{vsm.config.PreferredCodec}
	codecs = append(codecs, vsm.config.FallbackCodecs...)
	
	for _, codec := range codecs {
		if err := vsm.testCodecSupport(codec); err != nil {
			log.Printf("[VIDEO_STREAMING] Codec %s not supported: %v", codec, err)
		} else {
			log.Printf("[VIDEO_STREAMING] Codec %s supported", codec)
		}
	}
	
	return nil
}

// testCodecSupport tests if a codec is supported
func (vsm *VideoStreamingManager) testCodecSupport(codec VideoCodec) error {
	args := []string{"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1"}
	args = append(args, vsm.getCodecArgs(codec)...)
	args = append(args, "-f", "null", "-")
	
	cmd := exec.Command("ffmpeg", args...)
	return cmd.Run()
}

// initializeCodecs initializes codec instances
func (vsm *VideoStreamingManager) initializeCodecs() error {
	codecs := []VideoCodec{vsm.config.PreferredCodec}
	codecs = append(codecs, vsm.config.FallbackCodecs...)
	
	for _, codec := range codecs {
		if err := vsm.testCodecSupport(codec); err == nil {
			instance := &CodecInstance{
				Codec:           codec,
				IsHardwareAccel: vsm.config.HardwareAccel,
				QualityLevel:    80,
			}
			vsm.codecInstances[codec] = instance
			log.Printf("[VIDEO_STREAMING] Initialized codec: %s", codec)
		}
	}
	
	if len(vsm.codecInstances) == 0 {
		return fmt.Errorf("no supported codecs found")
	}
	
	return nil
}

// shutdownCodecs shuts down all codec instances
func (vsm *VideoStreamingManager) shutdownCodecs() {
	for codec, instance := range vsm.codecInstances {
		if instance.EncoderProcess != nil {
			instance.EncoderProcess.Process.Kill()
		}
		if instance.DecoderProcess != nil {
			instance.DecoderProcess.Process.Kill()
		}
		if instance.InputPipe != nil {
			instance.InputPipe.Close()
		}
		if instance.OutputPipe != nil {
			instance.OutputPipe.Close()
		}
		log.Printf("[VIDEO_STREAMING] Shutdown codec: %s", codec)
	}
	vsm.codecInstances = make(map[VideoCodec]*CodecInstance)
}

// qualityAdaptationLoop manages adaptive quality based on performance
func (vsm *VideoStreamingManager) qualityAdaptationLoop() {
	if !vsm.config.EnableAdaptiveQuality {
		return
	}
	
	ticker := time.NewTicker(vsm.config.QualityAdaptInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-vsm.streamingContext.Done():
			return
		case <-ticker.C:
			vsm.adaptQuality()
		}
	}
}

// adaptQuality adapts video quality based on current performance metrics
func (vsm *VideoStreamingManager) adaptQuality() {
	vsm.mu.Lock()
	defer vsm.mu.Unlock()
	
	// Get current performance metrics
	cpuUsage := vsm.performanceMonitor.CPUUsage
	memoryUsage := vsm.performanceMonitor.MemoryUsage
	networkBandwidth := vsm.networkMetrics.Bandwidth
	
	// Calculate adaptation recommendations
	targetFPS := vsm.currentFPS
	targetBitrate := vsm.currentBitrate
	targetResolution := vsm.currentResolution
	
	// Adapt FPS based on CPU usage
	if cpuUsage > 80 {
		targetFPS = maxInt(vsm.config.MinFPS, vsm.currentFPS-5)
	} else if cpuUsage < 50 && vsm.currentFPS < vsm.config.MaxFPS {
		targetFPS = min(vsm.config.MaxFPS, vsm.currentFPS+5)
	}
	
	// Adapt bitrate based on network bandwidth
	if networkBandwidth > 0 {
		optimalBitrate := int(float64(networkBandwidth) * 0.8) // Use 80% of bandwidth
		if optimalBitrate < vsm.currentBitrate {
			targetBitrate = maxInt(vsm.config.MinBitrate, optimalBitrate)
		} else if optimalBitrate > vsm.currentBitrate*2 {
			targetBitrate = min(vsm.config.MaxBitrate, vsm.currentBitrate*2)
		}
	}
	
	// Adapt resolution based on overall performance
	if cpuUsage > 90 || memoryUsage > 85 {
		targetResolution = vsm.getNextLowerResolution(vsm.currentResolution)
	} else if cpuUsage < 40 && memoryUsage < 60 {
		targetResolution = vsm.getNextHigherResolution(vsm.currentResolution)
	}
	
	// Apply adaptations if changes are significant
	if targetFPS != vsm.currentFPS || targetBitrate != vsm.currentBitrate || targetResolution != vsm.currentResolution {
		vsm.applyQualityAdaptation(targetFPS, targetBitrate, targetResolution)
	}
}

// applyQualityAdaptation applies quality adaptation changes
func (vsm *VideoStreamingManager) applyQualityAdaptation(fps, bitrate int, resolution VideoResolution) {
	adaptation := QualityAdaptation{
		Timestamp:   time.Now(),
		FromFPS:     vsm.currentFPS,
		ToFPS:       fps,
		FromBitrate: vsm.currentBitrate,
		ToBitrate:   bitrate,
		FromRes:     vsm.currentResolution,
		ToRes:       resolution,
		Reason:      "Performance optimization",
	}
	
	// Apply changes
	vsm.currentFPS = fps
	vsm.currentBitrate = bitrate
	vsm.currentResolution = resolution
	
	adaptation.Success = true
	vsm.qualityController.adaptationHistory = append(vsm.qualityController.adaptationHistory, adaptation)
	
	log.Printf("[VIDEO_STREAMING] Quality adapted: %dx%d@%dfps, %d bps",
		resolution.Width, resolution.Height, fps, bitrate)
	
	// Notify UI
	if vsm.uiUpdateChan != nil {
		vsm.uiUpdateChan <- VideoQualityAdaptedMsg{
			Resolution: resolution,
			FPS:        fps,
			Bitrate:    bitrate,
			Reason:     adaptation.Reason,
		}
	}
}

// getNextLowerResolution returns the next lower resolution from supported list
func (vsm *VideoStreamingManager) getNextLowerResolution(current VideoResolution) VideoResolution {
	for i, res := range vsm.config.SupportedResolutions {
		if res.Width == current.Width && res.Height == current.Height {
			if i+1 < len(vsm.config.SupportedResolutions) {
				return vsm.config.SupportedResolutions[i+1]
			}
		}
	}
	return current
}

// getNextHigherResolution returns the next higher resolution from supported list
func (vsm *VideoStreamingManager) getNextHigherResolution(current VideoResolution) VideoResolution {
	for i, res := range vsm.config.SupportedResolutions {
		if res.Width == current.Width && res.Height == current.Height {
			if i > 0 {
				return vsm.config.SupportedResolutions[i-1]
			}
		}
	}
	return current
}

// networkMonitoringLoop monitors network performance
func (vsm *VideoStreamingManager) networkMonitoringLoop() {
	ticker := time.NewTicker(vsm.config.NetworkTestInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-vsm.streamingContext.Done():
			return
		case <-ticker.C:
			vsm.updateNetworkMetrics()
		}
	}
}

// updateNetworkMetrics updates network performance metrics
func (vsm *VideoStreamingManager) updateNetworkMetrics() {
	// Simplified network metrics collection
	// In practice, this would use actual network monitoring
	
	vsm.networkMetrics.Bandwidth = 5000000 // 5 Mbps default
	vsm.networkMetrics.Latency = 20 * time.Millisecond
	vsm.networkMetrics.PacketLoss = 0.1 // 0.1%
	vsm.networkMetrics.Jitter = 5 * time.Millisecond
	vsm.networkMetrics.LastUpdated = time.Now()
	
	// Update throughput history
	vsm.networkMetrics.ThroughputHistory = append(vsm.networkMetrics.ThroughputHistory, vsm.networkMetrics.Bandwidth)
	if len(vsm.networkMetrics.ThroughputHistory) > 10 {
		vsm.networkMetrics.ThroughputHistory = vsm.networkMetrics.ThroughputHistory[1:]
	}
}

// performanceMonitoringLoop monitors system performance
func (vsm *VideoStreamingManager) performanceMonitoringLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-vsm.streamingContext.Done():
			return
		case <-ticker.C:
			vsm.updatePerformanceMetrics()
		}
	}
}

// updatePerformanceMetrics updates system performance metrics
func (vsm *VideoStreamingManager) updatePerformanceMetrics() {
	vsm.performanceMonitor.mu.Lock()
	defer vsm.performanceMonitor.mu.Unlock()
	
	// Simplified performance metrics collection
	// In practice, this would use actual system monitoring
	
	vsm.performanceMonitor.CPUUsage = 45.0 // 45% CPU usage
	vsm.performanceMonitor.MemoryUsage = 60.0 // 60% memory usage
	vsm.performanceMonitor.GPUUsage = 30.0 // 30% GPU usage
	vsm.performanceMonitor.NetworkBandwidth = vsm.networkMetrics.Bandwidth
	vsm.performanceMonitor.DiskIO = 1024 * 1024 // 1 MB/s
	vsm.performanceMonitor.LastUpdate = time.Now()
}

// GetStreamingStats returns current streaming statistics
func (vsm *VideoStreamingManager) GetStreamingStats() VideoStreamingStats {
	vsm.mu.RLock()
	defer vsm.mu.RUnlock()
	
	return VideoStreamingStats{
		IsStreaming:       vsm.isStreaming,
		CurrentFPS:        vsm.currentFPS,
		CurrentBitrate:    vsm.currentBitrate,
		CurrentResolution: vsm.currentResolution,
		CurrentCodec:      vsm.currentCodec,
		FramesProcessed:   vsm.frameCount,
		FramesDropped:     vsm.droppedFrames,
		AvgProcessingTime: vsm.avgProcessTime,
		NetworkMetrics:    vsm.networkMetrics,
		PerformanceMetrics: *vsm.performanceMonitor,
	}
}

// IsStreaming returns whether video streaming is active
func (vsm *VideoStreamingManager) IsStreaming() bool {
	vsm.mu.RLock()
	defer vsm.mu.RUnlock()
	return vsm.isStreaming
}

// SetTargetFPS sets the target FPS for streaming
func (vsm *VideoStreamingManager) SetTargetFPS(fps int) error {
	if fps < vsm.config.MinFPS || fps > vsm.config.MaxFPS {
		return fmt.Errorf("FPS %d out of range [%d, %d]", fps, vsm.config.MinFPS, vsm.config.MaxFPS)
	}
	
	vsm.mu.Lock()
	defer vsm.mu.Unlock()
	
	vsm.currentFPS = fps
	log.Printf("[VIDEO_STREAMING] Set target FPS to: %d", fps)
	return nil
}

// SetTargetBitrate sets the target bitrate for streaming
func (vsm *VideoStreamingManager) SetTargetBitrate(bitrate int) error {
	if bitrate < vsm.config.MinBitrate || bitrate > vsm.config.MaxBitrate {
		return fmt.Errorf("bitrate %d out of range [%d, %d]", bitrate, vsm.config.MinBitrate, vsm.config.MaxBitrate)
	}
	
	vsm.mu.Lock()
	defer vsm.mu.Unlock()
	
	vsm.currentBitrate = bitrate
	log.Printf("[VIDEO_STREAMING] Set target bitrate to: %d bps", bitrate)
	return nil
}

// SetTargetResolution sets the target resolution for streaming
func (vsm *VideoStreamingManager) SetTargetResolution(resolution VideoResolution) error {
	vsm.mu.Lock()
	defer vsm.mu.Unlock()
	
	vsm.currentResolution = resolution
	log.Printf("[VIDEO_STREAMING] Set target resolution to: %dx%d (%s)", 
		resolution.Width, resolution.Height, resolution.Name)
	return nil
}

// UpdateConfig updates the streaming configuration
func (vsm *VideoStreamingManager) UpdateConfig(config VideoStreamingConfig) error {
	vsm.mu.Lock()
	defer vsm.mu.Unlock()
	
	if vsm.isStreaming {
		return fmt.Errorf("cannot update configuration while streaming")
	}
	
	vsm.config = config
	log.Printf("[VIDEO_STREAMING] Updated configuration")
	return nil
}

// Video streaming related messages
type VideoStreamingStartedMsg struct {
	Resolution VideoResolution
	FPS        int
	Bitrate    int
	Codec      VideoCodec
}

type VideoStreamingStoppedMsg struct {
	FramesProcessed int64
	FramesDropped   int64
	Duration        time.Duration
}

type VideoQualityAdaptedMsg struct {
	Resolution VideoResolution
	FPS        int
	Bitrate    int
	Reason     string
}

type VideoStreamingStats struct {
	IsStreaming        bool
	CurrentFPS         int
	CurrentBitrate     int
	CurrentResolution  VideoResolution
	CurrentCodec       VideoCodec
	FramesProcessed    int64
	FramesDropped      int64
	AvgProcessingTime  time.Duration
	NetworkMetrics     NetworkMetrics
	PerformanceMetrics PerformanceMonitor
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}