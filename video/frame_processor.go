package video

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// FrameProcessor handles frame-based video processing and analysis
type FrameProcessor struct {
	// Configuration
	config           FrameProcessorConfig
	isProcessing     bool
	processingContext context.Context
	processingCancel  context.CancelFunc
	
	// Processing pipeline
	frameInput       chan VideoFrame
	processedOutput  chan ProcessedFrame
	uiUpdateChan     chan tea.Msg
	
	// Frame management
	frameBuffer      []VideoFrame
	frameIndex       map[int64]VideoFrame
	frameTimestamps  []time.Time
	
	// Segmentation
	segmentManager   *SegmentManager
	currentSegment   *VideoSegment
	segmentHistory   []VideoSegment
	
	// Gemini integration
	geminiClient     *GeminiVideoClient
	geminiEnabled    bool
	
	// Performance tracking
	processedFrames  int64
	totalProcessTime time.Duration
	avgProcessTime   time.Duration
	
	// Synchronization
	mu               sync.RWMutex
	
	// Advanced features
	temporalAnalyzer *TemporalAnalyzer
	frameComparator  *FrameComparator
	qualityAssessor  *QualityAssessor
}

// FrameProcessorConfig contains configuration for frame processing
type FrameProcessorConfig struct {
	// Basic settings
	EnableProcessing     bool
	ProcessingInterval   time.Duration
	MaxFrameBuffer      int
	MaxSegmentDuration  time.Duration
	
	// Frame analysis
	EnableFrameAnalysis  bool
	AnalysisTypes       []string // "temporal", "quality", "content", "motion"
	FrameComparisonMode string   // "sequential", "keyframe", "adaptive"
	
	// Segmentation settings
	EnableSegmentation   bool
	SegmentationMethod   string // "time", "scene", "motion", "content"
	MinSegmentDuration   time.Duration
	MaxSegmentLength     int
	SegmentOverlap       time.Duration
	
	// Gemini integration
	EnableGeminiAnalysis bool
	GeminiModelName      string
	GeminiAPIKey         string
	GeminiPromptTemplate string
	GeminiMaxTokens      int
	GeminiTemperature    float32
	
	// Quality assessment
	EnableQualityCheck   bool
	QualityThreshold     float64
	QualityMetrics       []string // "sharpness", "brightness", "contrast", "noise"
	
	// Temporal analysis
	EnableTemporalAnalysis bool
	TemporalWindowSize    int
	TemporalStride        int
	MotionThreshold       float64
	
	// Output settings
	SaveProcessedFrames  bool
	ProcessedFramesDir   string
	SaveSegments         bool
	SegmentsOutputDir    string
	
	// Performance settings
	EnableParallelProcessing bool
	WorkerCount             int
	EnableGPUAcceleration   bool
	MemoryLimit             int64
}

// ProcessedFrame represents a processed frame with analysis results
type ProcessedFrame struct {
	OriginalFrame    VideoFrame
	ProcessedData    []byte
	ProcessingTime   time.Duration
	Timestamp        time.Time
	FrameNumber      int64
	
	// Analysis results
	QualityScore     float64
	QualityMetrics   map[string]float64
	TemporalFeatures TemporalFeatures
	ContentAnalysis  ContentAnalysis
	MotionAnalysis   MotionAnalysis
	
	// Gemini analysis
	GeminiAnalysis   *GeminiFrameAnalysis
	
	// Processing metadata
	ProcessingStage  string
	ProcessingError  error
	ProcessingStats  ProcessingStats
}

// VideoSegment represents a video segment for analysis
type VideoSegment struct {
	ID              string
	StartTime       time.Time
	EndTime         time.Time
	Duration        time.Duration
	FrameCount      int
	StartFrame      int64
	EndFrame        int64
	
	// Segment content
	Frames          []VideoFrame
	KeyFrames       []VideoFrame
	RepresentativeFrame VideoFrame
	
	// Segment analysis
	SegmentType     string // "scene", "action", "transition", "static"
	ContentSummary  string
	MotionLevel     float64
	QualityScore    float64
	
	// Gemini analysis
	GeminiSummary   *GeminiSegmentAnalysis
	
	// Segment metadata
	Tags            []string
	Confidence      float64
	ProcessingTime  time.Duration
}

// TemporalFeatures represents temporal analysis features
type TemporalFeatures struct {
	OpticalFlow     []Vector2D
	MotionVectors   []Vector2D
	TemporalGradient float64
	Stability       float64
	MotionMagnitude float64
	DirectionConsistency float64
}

// ContentAnalysis represents content-based analysis results
type ContentAnalysis struct {
	Histogram       []float64
	TextureFeatures map[string]float64
	ColorFeatures   map[string]float64
	EdgeDensity     float64
	Complexity      float64
	Uniqueness      float64
}

// MotionAnalysis represents motion analysis results
type MotionAnalysis struct {
	MotionVectors   []Vector2D
	MotionMagnitude float64
	MotionDirection Vector2D
	MotionType      string // "linear", "circular", "random", "static"
	MotionRegions   []MotionRegion
}

// MotionRegion represents a region with motion
type MotionRegion struct {
	BoundingBox     BoundingBox
	MotionVector    Vector2D
	MotionIntensity float64
	MotionType      string
}

// GeminiFrameAnalysis represents Gemini AI analysis of a frame
type GeminiFrameAnalysis struct {
	Description     string
	Objects         []string
	Activities      []string
	Emotions        []string
	Setting         string
	Timestamp       time.Time
	Confidence      float64
	ProcessingTime  time.Duration
	TokensUsed      int
	Model           string
}

// GeminiSegmentAnalysis represents Gemini AI analysis of a video segment
type GeminiSegmentAnalysis struct {
	Summary         string
	KeyEvents       []string
	MainObjects     []string
	Activities      []string
	Emotions        []string
	Setting         string
	Narrative       string
	Timestamp       time.Time
	Confidence      float64
	ProcessingTime  time.Duration
	TokensUsed      int
	Model           string
}

// SegmentManager manages video segmentation
type SegmentManager struct {
	config          FrameProcessorConfig
	segments        []VideoSegment
	currentSegment  *VideoSegment
	segmentID       int
	mu              sync.RWMutex
}

// TemporalAnalyzer analyzes temporal patterns in video frames
type TemporalAnalyzer struct {
	windowSize      int
	stride          int
	frameBuffer     []VideoFrame
	featureHistory  []TemporalFeatures
	mu              sync.RWMutex
}

// FrameComparator compares frames for similarity and differences
type FrameComparator struct {
	comparisonMode  string
	keyFrames       []VideoFrame
	lastComparison  time.Time
	mu              sync.RWMutex
}

// QualityAssessor assesses video quality metrics
type QualityAssessor struct {
	threshold       float64
	metrics         []string
	qualityHistory  []float64
	mu              sync.RWMutex
}

// GeminiVideoClient handles Gemini API integration for video analysis
type GeminiVideoClient struct {
	apiKey          string
	modelName       string
	promptTemplate  string
	maxTokens       int
	temperature     float32
	client          *exec.Cmd
	mu              sync.RWMutex
}

// DefaultFrameProcessorConfig returns default configuration
func DefaultFrameProcessorConfig() FrameProcessorConfig {
	return FrameProcessorConfig{
		EnableProcessing:     true,
		ProcessingInterval:   100 * time.Millisecond,
		MaxFrameBuffer:      100,
		MaxSegmentDuration:  30 * time.Second,
		
		EnableFrameAnalysis:  true,
		AnalysisTypes:       []string{"temporal", "quality", "content", "motion"},
		FrameComparisonMode: "sequential",
		
		EnableSegmentation:   true,
		SegmentationMethod:   "scene",
		MinSegmentDuration:   2 * time.Second,
		MaxSegmentLength:     300, // 300 frames
		SegmentOverlap:       500 * time.Millisecond,
		
		EnableGeminiAnalysis: true,
		GeminiModelName:      "gemini-2.5-flash",
		GeminiAPIKey:         "",
		GeminiPromptTemplate: "Analyze this video frame and describe what you see in detail. Focus on objects, activities, emotions, and setting.",
		GeminiMaxTokens:      1000,
		GeminiTemperature:    0.7,
		
		EnableQualityCheck:   true,
		QualityThreshold:     0.7,
		QualityMetrics:       []string{"sharpness", "brightness", "contrast", "noise"},
		
		EnableTemporalAnalysis: true,
		TemporalWindowSize:    10,
		TemporalStride:        5,
		MotionThreshold:       0.1,
		
		SaveProcessedFrames:  true,
		ProcessedFramesDir:   "/tmp/aistudio_processed_frames",
		SaveSegments:         true,
		SegmentsOutputDir:    "/tmp/aistudio_segments",
		
		EnableParallelProcessing: true,
		WorkerCount:             4,
		EnableGPUAcceleration:   true,
		MemoryLimit:             1024 * 1024 * 1024, // 1GB
	}
}

// NewFrameProcessor creates a new frame processor
func NewFrameProcessor(config FrameProcessorConfig, uiUpdateChan chan tea.Msg) *FrameProcessor {
	fp := &FrameProcessor{
		config:          config,
		frameInput:      make(chan VideoFrame, config.MaxFrameBuffer),
		processedOutput: make(chan ProcessedFrame, config.MaxFrameBuffer),
		uiUpdateChan:    uiUpdateChan,
		frameBuffer:     make([]VideoFrame, 0, config.MaxFrameBuffer),
		frameIndex:      make(map[int64]VideoFrame),
		frameTimestamps: make([]time.Time, 0, config.MaxFrameBuffer),
		segmentHistory:  make([]VideoSegment, 0, 100),
	}
	
	// Initialize components
	fp.segmentManager = NewSegmentManager(config)
	fp.temporalAnalyzer = NewTemporalAnalyzer(config)
	fp.frameComparator = NewFrameComparator(config)
	fp.qualityAssessor = NewQualityAssessor(config)
	
	// Initialize Gemini client if enabled
	if config.EnableGeminiAnalysis && config.GeminiAPIKey != "" {
		fp.geminiClient = NewGeminiVideoClient(config)
		fp.geminiEnabled = true
	}
	
	// Create output directories
	if config.SaveProcessedFrames {
		os.MkdirAll(config.ProcessedFramesDir, 0755)
	}
	if config.SaveSegments {
		os.MkdirAll(config.SegmentsOutputDir, 0755)
	}
	
	return fp
}

// StartProcessing begins frame processing
func (fp *FrameProcessor) StartProcessing() error {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	
	if fp.isProcessing {
		return fmt.Errorf("frame processing already in progress")
	}
	
	// Initialize components
	if err := fp.initializeComponents(); err != nil {
		return fmt.Errorf("failed to initialize components: %w", err)
	}
	
	// Create processing context
	fp.processingContext, fp.processingCancel = context.WithCancel(context.Background())
	
	fp.isProcessing = true
	fp.processedFrames = 0
	fp.totalProcessTime = 0
	fp.avgProcessTime = 0
	
	log.Printf("[FRAME_PROCESSOR] Started processing with config: analysis=%v, segmentation=%v, gemini=%v",
		fp.config.EnableFrameAnalysis, fp.config.EnableSegmentation, fp.config.EnableGeminiAnalysis)
	
	// Start processing workers
	if fp.config.EnableParallelProcessing {
		for i := 0; i < fp.config.WorkerCount; i++ {
			go fp.processingWorker(i)
		}
	} else {
		go fp.processingWorker(0)
	}
	
	// Start management goroutines
	go fp.segmentationLoop()
	go fp.outputProcessingLoop()
	
	// Notify UI
	if fp.uiUpdateChan != nil {
		fp.uiUpdateChan <- FrameProcessingStartedMsg{
			EnabledFeatures: fp.getEnabledFeatures(),
		}
	}
	
	return nil
}

// StopProcessing stops frame processing
func (fp *FrameProcessor) StopProcessing() error {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	
	if !fp.isProcessing {
		return fmt.Errorf("no frame processing in progress")
	}
	
	// Cancel processing context
	if fp.processingCancel != nil {
		fp.processingCancel()
	}
	
	// Finalize current segment
	if fp.currentSegment != nil {
		fp.segmentManager.FinalizeSegment(fp.currentSegment)
	}
	
	fp.isProcessing = false
	
	log.Printf("[FRAME_PROCESSOR] Stopped processing - processed %d frames, avg time: %v",
		fp.processedFrames, fp.avgProcessTime)
	
	// Notify UI
	if fp.uiUpdateChan != nil {
		fp.uiUpdateChan <- FrameProcessingStoppedMsg{
			FramesProcessed: fp.processedFrames,
			AvgProcessTime:  fp.avgProcessTime,
		}
	}
	
	return nil
}

// ProcessFrame processes a single frame
func (fp *FrameProcessor) ProcessFrame(frame VideoFrame) error {
	if !fp.isProcessing {
		return fmt.Errorf("processing not active")
	}
	
	select {
	case fp.frameInput <- frame:
		return nil
	case <-fp.processingContext.Done():
		return fmt.Errorf("processing stopped")
	default:
		return fmt.Errorf("frame queue full")
	}
}

// GetProcessedFrame retrieves a processed frame
func (fp *FrameProcessor) GetProcessedFrame() (ProcessedFrame, error) {
	select {
	case frame := <-fp.processedOutput:
		return frame, nil
	case <-fp.processingContext.Done():
		return ProcessedFrame{}, fmt.Errorf("processing stopped")
	case <-time.After(5 * time.Second):
		return ProcessedFrame{}, fmt.Errorf("processed frame timeout")
	}
}

// processingWorker processes frames in parallel
func (fp *FrameProcessor) processingWorker(workerID int) {
	log.Printf("[FRAME_PROCESSOR] Started worker %d", workerID)
	
	for {
		select {
		case <-fp.processingContext.Done():
			log.Printf("[FRAME_PROCESSOR] Worker %d stopped", workerID)
			return
		case frame := <-fp.frameInput:
			startTime := time.Now()
			
			// Process frame
			processedFrame, err := fp.processVideoFrame(frame)
			if err != nil {
				log.Printf("[FRAME_PROCESSOR] Worker %d error processing frame: %v", workerID, err)
				continue
			}
			
			// Update metrics
			processingTime := time.Since(startTime)
			fp.mu.Lock()
			fp.processedFrames++
			fp.totalProcessTime += processingTime
			fp.avgProcessTime = fp.totalProcessTime / time.Duration(fp.processedFrames)
			fp.mu.Unlock()
			
			// Add to frame buffer
			fp.addToFrameBuffer(frame)
			
			// Send to output
			select {
			case fp.processedOutput <- processedFrame:
				// Success
			default:
				log.Printf("[FRAME_PROCESSOR] Worker %d output buffer full", workerID)
			}
		}
	}
}

// processVideoFrame processes a single video frame comprehensively
func (fp *FrameProcessor) processVideoFrame(frame VideoFrame) (ProcessedFrame, error) {
	startTime := time.Now()
	
	processedFrame := ProcessedFrame{
		OriginalFrame:   frame,
		ProcessingTime:  0,
		Timestamp:       time.Now(),
		FrameNumber:     frame.FrameNumber,
		QualityMetrics:  make(map[string]float64),
		ProcessingStage: "started",
	}
	
	// Quality assessment
	if fp.config.EnableQualityCheck {
		qualityScore, metrics, err := fp.qualityAssessor.AssessQuality(frame)
		if err != nil {
			log.Printf("[FRAME_PROCESSOR] Quality assessment error: %v", err)
		} else {
			processedFrame.QualityScore = qualityScore
			processedFrame.QualityMetrics = metrics
		}
	}
	
	// Temporal analysis
	if fp.config.EnableTemporalAnalysis {
		temporalFeatures, err := fp.temporalAnalyzer.AnalyzeFrame(frame)
		if err != nil {
			log.Printf("[FRAME_PROCESSOR] Temporal analysis error: %v", err)
		} else {
			processedFrame.TemporalFeatures = temporalFeatures
		}
	}
	
	// Content analysis
	if fp.config.EnableFrameAnalysis {
		contentAnalysis, err := fp.analyzeFrameContent(frame)
		if err != nil {
			log.Printf("[FRAME_PROCESSOR] Content analysis error: %v", err)
		} else {
			processedFrame.ContentAnalysis = contentAnalysis
		}
	}
	
	// Motion analysis
	motionAnalysis, err := fp.analyzeFrameMotion(frame)
	if err != nil {
		log.Printf("[FRAME_PROCESSOR] Motion analysis error: %v", err)
	} else {
		processedFrame.MotionAnalysis = motionAnalysis
	}
	
	// Gemini analysis
	if fp.config.EnableGeminiAnalysis && fp.geminiEnabled {
		geminiAnalysis, err := fp.geminiClient.AnalyzeFrame(frame)
		if err != nil {
			log.Printf("[FRAME_PROCESSOR] Gemini analysis error: %v", err)
		} else {
			processedFrame.GeminiAnalysis = geminiAnalysis
		}
	}
	
	// Finalize processing
	processedFrame.ProcessingTime = time.Since(startTime)
	processedFrame.ProcessingStage = "completed"
	
	// Save processed frame if enabled
	if fp.config.SaveProcessedFrames {
		fp.saveProcessedFrame(processedFrame)
	}
	
	return processedFrame, nil
}

// analyzeFrameContent analyzes the content of a video frame
func (fp *FrameProcessor) analyzeFrameContent(frame VideoFrame) (ContentAnalysis, error) {
	// Simulate content analysis
	// In practice, this would use computer vision algorithms
	
	analysis := ContentAnalysis{
		Histogram:       make([]float64, 256),
		TextureFeatures: make(map[string]float64),
		ColorFeatures:   make(map[string]float64),
		EdgeDensity:     0.3,
		Complexity:      0.5,
		Uniqueness:      0.7,
	}
	
	// Simulate histogram calculation
	for i := 0; i < 256; i++ {
		analysis.Histogram[i] = float64(i) / 255.0
	}
	
	// Simulate texture features
	analysis.TextureFeatures["contrast"] = 0.6
	analysis.TextureFeatures["homogeneity"] = 0.4
	analysis.TextureFeatures["energy"] = 0.3
	
	// Simulate color features
	analysis.ColorFeatures["brightness"] = 0.7
	analysis.ColorFeatures["saturation"] = 0.5
	analysis.ColorFeatures["hue_variance"] = 0.3
	
	return analysis, nil
}

// analyzeFrameMotion analyzes motion in a video frame
func (fp *FrameProcessor) analyzeFrameMotion(frame VideoFrame) (MotionAnalysis, error) {
	// Simulate motion analysis
	// In practice, this would use optical flow algorithms
	
	analysis := MotionAnalysis{
		MotionVectors:   []Vector2D{{X: 0.1, Y: 0.0}, {X: -0.05, Y: 0.1}},
		MotionMagnitude: 0.2,
		MotionDirection: Vector2D{X: 0.05, Y: 0.05},
		MotionType:      "linear",
		MotionRegions:   []MotionRegion{},
	}
	
	// Simulate motion region
	motionRegion := MotionRegion{
		BoundingBox: BoundingBox{
			X: 0.3, Y: 0.3, Width: 0.4, Height: 0.4,
		},
		MotionVector:    Vector2D{X: 0.1, Y: 0.0},
		MotionIntensity: 0.5,
		MotionType:      "linear",
	}
	analysis.MotionRegions = append(analysis.MotionRegions, motionRegion)
	
	return analysis, nil
}

// addToFrameBuffer adds a frame to the internal buffer
func (fp *FrameProcessor) addToFrameBuffer(frame VideoFrame) {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	
	// Add to buffer
	fp.frameBuffer = append(fp.frameBuffer, frame)
	fp.frameIndex[frame.FrameNumber] = frame
	fp.frameTimestamps = append(fp.frameTimestamps, frame.Timestamp)
	
	// Maintain buffer size
	if len(fp.frameBuffer) > fp.config.MaxFrameBuffer {
		// Remove oldest frame
		oldest := fp.frameBuffer[0]
		fp.frameBuffer = fp.frameBuffer[1:]
		delete(fp.frameIndex, oldest.FrameNumber)
		fp.frameTimestamps = fp.frameTimestamps[1:]
	}
}

// segmentationLoop manages video segmentation
func (fp *FrameProcessor) segmentationLoop() {
	if !fp.config.EnableSegmentation {
		return
	}
	
	ticker := time.NewTicker(fp.config.ProcessingInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-fp.processingContext.Done():
			return
		case <-ticker.C:
			fp.updateSegmentation()
		}
	}
}

// updateSegmentation updates video segmentation
func (fp *FrameProcessor) updateSegmentation() {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	
	// Check if we need to create a new segment
	if fp.currentSegment == nil || fp.shouldCreateNewSegment() {
		if fp.currentSegment != nil {
			fp.segmentManager.FinalizeSegment(fp.currentSegment)
			fp.segmentHistory = append(fp.segmentHistory, *fp.currentSegment)
		}
		
		fp.currentSegment = fp.segmentManager.CreateNewSegment()
		log.Printf("[FRAME_PROCESSOR] Created new segment: %s", fp.currentSegment.ID)
	}
	
	// Add recent frames to current segment
	if len(fp.frameBuffer) > 0 {
		recentFrames := fp.frameBuffer[maxFrameProcessor(0, len(fp.frameBuffer)-10):]
		for _, frame := range recentFrames {
			fp.segmentManager.AddFrameToSegment(fp.currentSegment, frame)
		}
	}
}

// shouldCreateNewSegment determines if a new segment should be created
func (fp *FrameProcessor) shouldCreateNewSegment() bool {
	if fp.currentSegment == nil {
		return true
	}
	
	// Check duration
	if time.Since(fp.currentSegment.StartTime) > fp.config.MaxSegmentDuration {
		return true
	}
	
	// Check frame count
	if fp.currentSegment.FrameCount > fp.config.MaxSegmentLength {
		return true
	}
	
	// Check for scene changes (simplified)
	if len(fp.frameBuffer) > 1 {
		// In practice, this would use actual scene detection
		return false
	}
	
	return false
}

// outputProcessingLoop processes output frames
func (fp *FrameProcessor) outputProcessingLoop() {
	for {
		select {
		case <-fp.processingContext.Done():
			return
		case processedFrame := <-fp.processedOutput:
			// Send to UI
			if fp.uiUpdateChan != nil {
				fp.uiUpdateChan <- FrameProcessedMsg{
					Frame: processedFrame,
				}
			}
		}
	}
}

// saveProcessedFrame saves a processed frame to disk
func (fp *FrameProcessor) saveProcessedFrame(frame ProcessedFrame) {
	filename := filepath.Join(fp.config.ProcessedFramesDir,
		fmt.Sprintf("frame_%d_%d.json", frame.FrameNumber, frame.Timestamp.Unix()))
	
	data, err := json.MarshalIndent(frame, "", "  ")
	if err != nil {
		log.Printf("[FRAME_PROCESSOR] Error marshaling processed frame: %v", err)
		return
	}
	
	if err := os.WriteFile(filename, data, 0644); err != nil {
		log.Printf("[FRAME_PROCESSOR] Error saving processed frame: %v", err)
	}
}

// initializeComponents initializes all processing components
func (fp *FrameProcessor) initializeComponents() error {
	// Initialize segment manager
	if err := fp.segmentManager.Initialize(); err != nil {
		return fmt.Errorf("segment manager initialization failed: %w", err)
	}
	
	// Initialize temporal analyzer
	if err := fp.temporalAnalyzer.Initialize(); err != nil {
		return fmt.Errorf("temporal analyzer initialization failed: %w", err)
	}
	
	// Initialize frame comparator
	if err := fp.frameComparator.Initialize(); err != nil {
		return fmt.Errorf("frame comparator initialization failed: %w", err)
	}
	
	// Initialize quality assessor
	if err := fp.qualityAssessor.Initialize(); err != nil {
		return fmt.Errorf("quality assessor initialization failed: %w", err)
	}
	
	// Initialize Gemini client
	if fp.geminiClient != nil {
		if err := fp.geminiClient.Initialize(); err != nil {
			log.Printf("[FRAME_PROCESSOR] Gemini client initialization failed: %v", err)
			fp.geminiEnabled = false
		}
	}
	
	return nil
}

// getEnabledFeatures returns a list of enabled features
func (fp *FrameProcessor) getEnabledFeatures() []string {
	features := []string{}
	
	if fp.config.EnableFrameAnalysis {
		features = append(features, "frame_analysis")
	}
	if fp.config.EnableSegmentation {
		features = append(features, "segmentation")
	}
	if fp.config.EnableGeminiAnalysis {
		features = append(features, "gemini_analysis")
	}
	if fp.config.EnableQualityCheck {
		features = append(features, "quality_check")
	}
	if fp.config.EnableTemporalAnalysis {
		features = append(features, "temporal_analysis")
	}
	
	return features
}

// Component implementations

// NewSegmentManager creates a new segment manager
func NewSegmentManager(config FrameProcessorConfig) *SegmentManager {
	return &SegmentManager{
		config:   config,
		segments: make([]VideoSegment, 0, 100),
	}
}

func (sm *SegmentManager) Initialize() error {
	log.Printf("[SEGMENT_MANAGER] Initialized with method: %s", sm.config.SegmentationMethod)
	return nil
}

func (sm *SegmentManager) CreateNewSegment() *VideoSegment {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	sm.segmentID++
	segment := &VideoSegment{
		ID:        fmt.Sprintf("segment_%d", sm.segmentID),
		StartTime: time.Now(),
		Frames:    make([]VideoFrame, 0, sm.config.MaxSegmentLength),
		KeyFrames: make([]VideoFrame, 0, 10),
		Tags:      make([]string, 0, 10),
	}
	
	return segment
}

func (sm *SegmentManager) AddFrameToSegment(segment *VideoSegment, frame VideoFrame) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	segment.Frames = append(segment.Frames, frame)
	segment.FrameCount++
	
	if segment.FrameCount == 1 {
		segment.StartFrame = frame.FrameNumber
	}
	segment.EndFrame = frame.FrameNumber
	
	// Check if this should be a keyframe
	if sm.shouldAddKeyFrame(segment, frame) {
		segment.KeyFrames = append(segment.KeyFrames, frame)
	}
}

func (sm *SegmentManager) shouldAddKeyFrame(segment *VideoSegment, frame VideoFrame) bool {
	// Add first frame as keyframe
	if len(segment.KeyFrames) == 0 {
		return true
	}
	
	// Add keyframes at regular intervals
	if segment.FrameCount%30 == 0 {
		return true
	}
	
	return false
}

func (sm *SegmentManager) FinalizeSegment(segment *VideoSegment) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	segment.EndTime = time.Now()
	segment.Duration = segment.EndTime.Sub(segment.StartTime)
	
	// Select representative frame
	if len(segment.KeyFrames) > 0 {
		segment.RepresentativeFrame = segment.KeyFrames[len(segment.KeyFrames)/2]
	} else if len(segment.Frames) > 0 {
		segment.RepresentativeFrame = segment.Frames[len(segment.Frames)/2]
	}
	
	// Calculate segment metrics
	segment.MotionLevel = 0.3 // Simplified
	segment.QualityScore = 0.8 // Simplified
	segment.SegmentType = "scene"
	segment.ContentSummary = "Video segment with motion and objects"
	
	sm.segments = append(sm.segments, *segment)
	
	log.Printf("[SEGMENT_MANAGER] Finalized segment %s: %d frames, %v duration",
		segment.ID, segment.FrameCount, segment.Duration)
}

// NewTemporalAnalyzer creates a new temporal analyzer
func NewTemporalAnalyzer(config FrameProcessorConfig) *TemporalAnalyzer {
	return &TemporalAnalyzer{
		windowSize:     config.TemporalWindowSize,
		stride:         config.TemporalStride,
		frameBuffer:    make([]VideoFrame, 0, config.TemporalWindowSize),
		featureHistory: make([]TemporalFeatures, 0, 100),
	}
}

func (ta *TemporalAnalyzer) Initialize() error {
	log.Printf("[TEMPORAL_ANALYZER] Initialized with window size: %d", ta.windowSize)
	return nil
}

func (ta *TemporalAnalyzer) AnalyzeFrame(frame VideoFrame) (TemporalFeatures, error) {
	ta.mu.Lock()
	defer ta.mu.Unlock()
	
	// Add frame to buffer
	ta.frameBuffer = append(ta.frameBuffer, frame)
	if len(ta.frameBuffer) > ta.windowSize {
		ta.frameBuffer = ta.frameBuffer[1:]
	}
	
	// Analyze if we have enough frames
	if len(ta.frameBuffer) < 2 {
		return TemporalFeatures{}, nil
	}
	
	// Calculate temporal features
	features := TemporalFeatures{
		OpticalFlow:     []Vector2D{{X: 0.1, Y: 0.05}},
		MotionVectors:   []Vector2D{{X: 0.05, Y: 0.1}},
		TemporalGradient: 0.3,
		Stability:       0.8,
		MotionMagnitude: 0.2,
		DirectionConsistency: 0.9,
	}
	
	ta.featureHistory = append(ta.featureHistory, features)
	if len(ta.featureHistory) > 100 {
		ta.featureHistory = ta.featureHistory[1:]
	}
	
	return features, nil
}

// NewFrameComparator creates a new frame comparator
func NewFrameComparator(config FrameProcessorConfig) *FrameComparator {
	return &FrameComparator{
		comparisonMode: config.FrameComparisonMode,
		keyFrames:      make([]VideoFrame, 0, 50),
	}
}

func (fc *FrameComparator) Initialize() error {
	log.Printf("[FRAME_COMPARATOR] Initialized with mode: %s", fc.comparisonMode)
	return nil
}

// NewQualityAssessor creates a new quality assessor
func NewQualityAssessor(config FrameProcessorConfig) *QualityAssessor {
	return &QualityAssessor{
		threshold:      config.QualityThreshold,
		metrics:        config.QualityMetrics,
		qualityHistory: make([]float64, 0, 100),
	}
}

func (qa *QualityAssessor) Initialize() error {
	log.Printf("[QUALITY_ASSESSOR] Initialized with threshold: %f", qa.threshold)
	return nil
}

func (qa *QualityAssessor) AssessQuality(frame VideoFrame) (float64, map[string]float64, error) {
	qa.mu.Lock()
	defer qa.mu.Unlock()
	
	metrics := make(map[string]float64)
	
	// Simulate quality metrics
	metrics["sharpness"] = 0.8
	metrics["brightness"] = 0.7
	metrics["contrast"] = 0.6
	metrics["noise"] = 0.1
	
	// Calculate overall quality score
	qualityScore := (metrics["sharpness"] + metrics["brightness"] + metrics["contrast"] + (1.0 - metrics["noise"])) / 4.0
	
	qa.qualityHistory = append(qa.qualityHistory, qualityScore)
	if len(qa.qualityHistory) > 100 {
		qa.qualityHistory = qa.qualityHistory[1:]
	}
	
	return qualityScore, metrics, nil
}

// NewGeminiVideoClient creates a new Gemini video client
func NewGeminiVideoClient(config FrameProcessorConfig) *GeminiVideoClient {
	return &GeminiVideoClient{
		apiKey:         config.GeminiAPIKey,
		modelName:      config.GeminiModelName,
		promptTemplate: config.GeminiPromptTemplate,
		maxTokens:      config.GeminiMaxTokens,
		temperature:    config.GeminiTemperature,
	}
}

func (gvc *GeminiVideoClient) Initialize() error {
	if gvc.apiKey == "" {
		return fmt.Errorf("Gemini API key not provided")
	}
	
	log.Printf("[GEMINI_CLIENT] Initialized with model: %s", gvc.modelName)
	return nil
}

func (gvc *GeminiVideoClient) AnalyzeFrame(frame VideoFrame) (*GeminiFrameAnalysis, error) {
	gvc.mu.Lock()
	defer gvc.mu.Unlock()
	
	startTime := time.Now()
	
	// Convert frame to base64 for API
	frameData := base64.StdEncoding.EncodeToString(frame.Data)
	
	// Prepare API request (simplified)
	_ = fmt.Sprintf("%s\n\nFrame data: %s", gvc.promptTemplate, frameData[:100]+"...")
	
	// Simulate API call
	analysis := &GeminiFrameAnalysis{
		Description: "A person working at a computer in an office setting",
		Objects:     []string{"person", "computer", "desk", "chair"},
		Activities:  []string{"working", "typing", "looking at screen"},
		Emotions:    []string{"focused", "concentrated"},
		Setting:     "office",
		Timestamp:   time.Now(),
		Confidence:  0.85,
		ProcessingTime: time.Since(startTime),
		TokensUsed:  150,
		Model:       gvc.modelName,
	}
	
	return analysis, nil
}

// AnalyzeSegment analyzes a video segment using Gemini
func (gvc *GeminiVideoClient) AnalyzeSegment(segment VideoSegment) (*GeminiSegmentAnalysis, error) {
	gvc.mu.Lock()
	defer gvc.mu.Unlock()
	
	startTime := time.Now()
	
	// Prepare segment analysis request
	_ = fmt.Sprintf("Analyze this video segment with %d frames over %v duration", 
		segment.FrameCount, segment.Duration)
	
	// Simulate API call
	analysis := &GeminiSegmentAnalysis{
		Summary:    "A video segment showing a person working at a computer",
		KeyEvents:  []string{"person sits down", "starts typing", "looks at screen"},
		MainObjects: []string{"person", "computer", "desk", "office equipment"},
		Activities: []string{"working", "typing", "concentrating"},
		Emotions:   []string{"focused", "productive"},
		Setting:    "modern office",
		Narrative:  "The segment shows someone engaged in productive work in a professional environment",
		Timestamp:  time.Now(),
		Confidence: 0.82,
		ProcessingTime: time.Since(startTime),
		TokensUsed: 200,
		Model:      gvc.modelName,
	}
	
	return analysis, nil
}

// GetProcessingStats returns current processing statistics
func (fp *FrameProcessor) GetProcessingStats() FrameProcessingStats {
	fp.mu.RLock()
	defer fp.mu.RUnlock()
	
	return FrameProcessingStats{
		IsProcessing:     fp.isProcessing,
		FramesProcessed:  fp.processedFrames,
		AvgProcessTime:   fp.avgProcessTime,
		BufferSize:       len(fp.frameBuffer),
		SegmentCount:     len(fp.segmentHistory),
		CurrentSegment:   fp.currentSegment,
		EnabledFeatures:  fp.getEnabledFeatures(),
	}
}

// IsProcessing returns whether frame processing is active
func (fp *FrameProcessor) IsProcessing() bool {
	fp.mu.RLock()
	defer fp.mu.RUnlock()
	return fp.isProcessing
}

// UpdateConfig updates the processing configuration
func (fp *FrameProcessor) UpdateConfig(config FrameProcessorConfig) error {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	
	if fp.isProcessing {
		return fmt.Errorf("cannot update configuration while processing")
	}
	
	fp.config = config
	log.Printf("[FRAME_PROCESSOR] Updated configuration")
	return nil
}

// Frame processing related messages
type FrameProcessingStartedMsg struct {
	EnabledFeatures []string
}

type FrameProcessingStoppedMsg struct {
	FramesProcessed int64
	AvgProcessTime  time.Duration
}

type FrameProcessedMsg struct {
	Frame ProcessedFrame
}

type FrameProcessingStats struct {
	IsProcessing    bool
	FramesProcessed int64
	AvgProcessTime  time.Duration
	BufferSize      int
	SegmentCount    int
	CurrentSegment  *VideoSegment
	EnabledFeatures []string
}

// Helper functions for Go versions without min/max
func maxFrameProcessor(a, b int) int {
	if a > b {
		return a
	}
	return b
}