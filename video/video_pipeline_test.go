package video

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestVideoStreamingManager tests the video streaming functionality
func TestVideoStreamingManager(t *testing.T) {
	// Create test configuration
	config := DefaultVideoStreamingConfig()
	config.InitialFPS = 30
	config.InitialBitrate = 1000000
	config.InitialResolution = Resolution720p
	config.EnableAdaptiveQuality = true
	
	// Create UI update channel
	uiUpdateChan := make(chan tea.Msg, 100)
	
	// Create video streaming manager
	vsm := NewVideoStreamingManager(config, uiUpdateChan)
	
	// Test initial state
	if vsm.IsStreaming() {
		t.Error("Expected streaming to be false initially")
	}
	
	// Test starting streaming
	if err := vsm.StartStreaming(); err != nil {
		t.Fatalf("Failed to start streaming: %v", err)
	}
	
	if !vsm.IsStreaming() {
		t.Error("Expected streaming to be true after starting")
	}
	
	// Test processing frames
	testFrame := VideoFrame{
		Data:        make([]byte, 1024),
		Width:       1280,
		Height:      720,
		Format:      "rgb24",
		Timestamp:   time.Now(),
		FrameNumber: 1,
		Source:      "test",
	}
	
	if err := vsm.ProcessFrame(testFrame); err != nil {
		t.Errorf("Failed to process frame: %v", err)
	}
	
	// Test getting processed frame with timeout
	go func() {
		time.Sleep(100 * time.Millisecond)
		processedFrame, err := vsm.GetProcessedFrame()
		if err != nil {
			t.Errorf("Error getting processed frame: %v", err)
		}
		if processedFrame.FrameNumber != 1 {
			t.Errorf("Expected frame number 1, got %d", processedFrame.FrameNumber)
		}
	}()
	
	// Test stopping streaming
	if err := vsm.StopStreaming(); err != nil {
		t.Errorf("Failed to stop streaming: %v", err)
	}
	
	if vsm.IsStreaming() {
		t.Error("Expected streaming to be false after stopping")
	}
	
	// Test streaming statistics
	stats := vsm.GetStreamingStats()
	if stats.IsStreaming {
		t.Error("Expected stats to show streaming as false")
	}
}

// TestVideoAnalysisManager tests the video analysis functionality
func TestVideoAnalysisManager(t *testing.T) {
	// Create test configuration
	config := DefaultVideoAnalysisConfig()
	config.EnableObjectDetection = true
	config.EnableSceneAnalysis = true
	config.EnableMotionDetection = true
	config.AnalysisInterval = 10 * time.Millisecond
	
	// Create UI update channel
	uiUpdateChan := make(chan tea.Msg, 100)
	
	// Create video analysis manager
	vam := NewVideoAnalysisManager(config, uiUpdateChan)
	
	// Test initial state
	if vam.IsAnalyzing() {
		t.Error("Expected analyzing to be false initially")
	}
	
	// Test starting analysis
	if err := vam.StartAnalysis(); err != nil {
		t.Fatalf("Failed to start analysis: %v", err)
	}
	
	if !vam.IsAnalyzing() {
		t.Error("Expected analyzing to be true after starting")
	}
	
	// Test analyzing frames
	testFrame := VideoFrame{
		Data:        make([]byte, 2048), // Larger data to trigger object detection
		Width:       1920,
		Height:      1080,
		Format:      "rgb24",
		Timestamp:   time.Now(),
		FrameNumber: 1,
		Source:      "test",
	}
	
	if err := vam.AnalyzeFrame(testFrame); err != nil {
		t.Errorf("Failed to analyze frame: %v", err)
	}
	
	// Test getting analysis result with timeout
	go func() {
		time.Sleep(100 * time.Millisecond)
		result, err := vam.GetAnalysisResult()
		if err != nil {
			t.Errorf("Error getting analysis result: %v", err)
		}
		if result.FrameNumber != 1 {
			t.Errorf("Expected frame number 1, got %d", result.FrameNumber)
		}
		if result.ObjectCount == 0 {
			t.Error("Expected to detect at least one object")
		}
	}()
	
	// Test stopping analysis
	if err := vam.StopAnalysis(); err != nil {
		t.Errorf("Failed to stop analysis: %v", err)
	}
	
	if vam.IsAnalyzing() {
		t.Error("Expected analyzing to be false after stopping")
	}
	
	// Test analysis statistics
	stats := vam.GetAnalysisStats()
	if stats.IsAnalyzing {
		t.Error("Expected stats to show analyzing as false")
	}
	if stats.FramesAnalyzed == 0 {
		t.Error("Expected at least one frame to be analyzed")
	}
}

// TestFrameProcessor tests the frame processing functionality
func TestFrameProcessor(t *testing.T) {
	// Create test configuration
	config := DefaultFrameProcessorConfig()
	config.EnableFrameAnalysis = true
	config.EnableSegmentation = true
	config.EnableQualityCheck = true
	config.EnableTemporalAnalysis = true
	config.ProcessingInterval = 10 * time.Millisecond
	
	// Create UI update channel
	uiUpdateChan := make(chan tea.Msg, 100)
	
	// Create frame processor
	fp := NewFrameProcessor(config, uiUpdateChan)
	
	// Test initial state
	if fp.IsProcessing() {
		t.Error("Expected processing to be false initially")
	}
	
	// Test starting processing
	if err := fp.StartProcessing(); err != nil {
		t.Fatalf("Failed to start processing: %v", err)
	}
	
	if !fp.IsProcessing() {
		t.Error("Expected processing to be true after starting")
	}
	
	// Test processing frames
	testFrame := VideoFrame{
		Data:        make([]byte, 1024),
		Width:       1280,
		Height:      720,
		Format:      "rgb24",
		Timestamp:   time.Now(),
		FrameNumber: 1,
		Source:      "test",
	}
	
	if err := fp.ProcessFrame(testFrame); err != nil {
		t.Errorf("Failed to process frame: %v", err)
	}
	
	// Test getting processed frame with timeout
	go func() {
		time.Sleep(100 * time.Millisecond)
		processedFrame, err := fp.GetProcessedFrame()
		if err != nil {
			t.Errorf("Error getting processed frame: %v", err)
		}
		if processedFrame.FrameNumber != 1 {
			t.Errorf("Expected frame number 1, got %d", processedFrame.FrameNumber)
		}
		if processedFrame.QualityScore == 0 {
			t.Error("Expected quality score to be calculated")
		}
	}()
	
	// Test stopping processing
	if err := fp.StopProcessing(); err != nil {
		t.Errorf("Failed to stop processing: %v", err)
	}
	
	if fp.IsProcessing() {
		t.Error("Expected processing to be false after stopping")
	}
	
	// Test processing statistics
	stats := fp.GetProcessingStats()
	if stats.IsProcessing {
		t.Error("Expected stats to show processing as false")
	}
	if stats.FramesProcessed == 0 {
		t.Error("Expected at least one frame to be processed")
	}
}

// TestVideoResolutionAdaptation tests video resolution adaptation
func TestVideoResolutionAdaptation(t *testing.T) {
	config := DefaultVideoStreamingConfig()
	config.EnableAdaptiveQuality = true
	config.InitialResolution = Resolution1080p
	
	uiUpdateChan := make(chan tea.Msg, 100)
	vsm := NewVideoStreamingManager(config, uiUpdateChan)
	
	// Test setting target resolution
	if err := vsm.SetTargetResolution(Resolution720p); err != nil {
		t.Errorf("Failed to set target resolution: %v", err)
	}
	
	// Test setting target FPS
	if err := vsm.SetTargetFPS(30); err != nil {
		t.Errorf("Failed to set target FPS: %v", err)
	}
	
	// Test setting target bitrate
	if err := vsm.SetTargetBitrate(2000000); err != nil {
		t.Errorf("Failed to set target bitrate: %v", err)
	}
	
	// Test invalid values
	if err := vsm.SetTargetFPS(5); err == nil {
		t.Error("Expected error for invalid FPS")
	}
	
	if err := vsm.SetTargetBitrate(100000); err == nil {
		t.Error("Expected error for invalid bitrate")
	}
}

// TestObjectDetection tests object detection functionality
func TestObjectDetection(t *testing.T) {
	config := DefaultVideoAnalysisConfig()
	od := NewObjectDetector(config)
	
	// Test initialization
	if err := od.Initialize(); err != nil {
		t.Fatalf("Failed to initialize object detector: %v", err)
	}
	
	// Test object detection
	testFrame := VideoFrame{
		Data:        make([]byte, 2048),
		Width:       1920,
		Height:      1080,
		Format:      "rgb24",
		Timestamp:   time.Now(),
		FrameNumber: 1,
		Source:      "test",
	}
	
	objects, err := od.DetectObjects(testFrame)
	if err != nil {
		t.Errorf("Failed to detect objects: %v", err)
	}
	
	if len(objects) == 0 {
		t.Error("Expected to detect at least one object")
	}
	
	// Test object properties
	if len(objects) > 0 {
		obj := objects[0]
		if obj.Label == "" {
			t.Error("Expected object to have a label")
		}
		if obj.Confidence == 0 {
			t.Error("Expected object to have confidence > 0")
		}
		if obj.TrackingID == "" {
			t.Error("Expected object to have tracking ID")
		}
	}
}

// TestSceneAnalysis tests scene analysis functionality
func TestSceneAnalysis(t *testing.T) {
	config := DefaultVideoAnalysisConfig()
	sa := NewSceneAnalyzer(config)
	
	// Test initialization
	if err := sa.Initialize(); err != nil {
		t.Fatalf("Failed to initialize scene analyzer: %v", err)
	}
	
	// Test scene analysis
	testFrame := VideoFrame{
		Data:        make([]byte, 1024),
		Width:       1280,
		Height:      720,
		Format:      "rgb24",
		Timestamp:   time.Now(),
		FrameNumber: 1,
		Source:      "test",
	}
	
	scene, confidence, transition, err := sa.AnalyzeScene(testFrame)
	if err != nil {
		t.Errorf("Failed to analyze scene: %v", err)
	}
	
	if scene.Setting == "" {
		t.Error("Expected scene to have a setting")
	}
	
	if confidence == 0 {
		t.Error("Expected scene confidence > 0")
	}
	
	if len(scene.Objects) == 0 {
		t.Error("Expected scene to have objects")
	}
	
	// Test transition detection (first scene should not be a transition)
	if transition {
		t.Error("Expected first scene to not be a transition")
	}
}

// TestMotionDetection tests motion detection functionality
func TestMotionDetection(t *testing.T) {
	config := DefaultVideoAnalysisConfig()
	md := NewMotionDetector(config)
	
	// Test initialization
	if err := md.Initialize(); err != nil {
		t.Fatalf("Failed to initialize motion detector: %v", err)
	}
	
	// Test motion detection with first frame
	testFrame1 := VideoFrame{
		Data:        make([]byte, 1024),
		Width:       1280,
		Height:      720,
		Format:      "rgb24",
		Timestamp:   time.Now(),
		FrameNumber: 1,
		Source:      "test",
	}
	
	// Fill with test data
	for i := range testFrame1.Data {
		testFrame1.Data[i] = byte(i % 256)
	}
	
	_, _, detected, err := md.DetectMotion(testFrame1)
	if err != nil {
		t.Errorf("Failed to detect motion: %v", err)
	}
	
	// First frame should not have motion
	if detected {
		t.Error("Expected no motion on first frame")
	}
	
	// Test motion detection with second frame (different data)
	testFrame2 := VideoFrame{
		Data:        make([]byte, 1024),
		Width:       1280,
		Height:      720,
		Format:      "rgb24",
		Timestamp:   time.Now(),
		FrameNumber: 2,
		Source:      "test",
	}
	
	// Fill with different test data
	for i := range testFrame2.Data {
		testFrame2.Data[i] = byte((i + 100) % 256)
	}
	
	motionAreas2, motionLevel2, detected2, err := md.DetectMotion(testFrame2)
	if err != nil {
		t.Errorf("Failed to detect motion: %v", err)
	}
	
	// Second frame should have motion
	if !detected2 {
		t.Error("Expected motion on second frame")
	}
	
	if motionLevel2 == 0 {
		t.Error("Expected motion level > 0")
	}
	
	if len(motionAreas2) == 0 {
		t.Error("Expected motion areas")
	}
}

// TestVideoSegmentation tests video segmentation functionality
func TestVideoSegmentation(t *testing.T) {
	config := DefaultFrameProcessorConfig()
	config.EnableSegmentation = true
	config.MaxSegmentLength = 10
	
	sm := NewSegmentManager(config)
	
	// Test initialization
	if err := sm.Initialize(); err != nil {
		t.Fatalf("Failed to initialize segment manager: %v", err)
	}
	
	// Test creating new segment
	segment := sm.CreateNewSegment()
	if segment == nil {
		t.Error("Expected segment to be created")
	}
	
	if segment.ID == "" {
		t.Error("Expected segment to have ID")
	}
	
	// Test adding frames to segment
	for i := 0; i < 5; i++ {
		testFrame := VideoFrame{
			Data:        make([]byte, 1024),
			Width:       1280,
			Height:      720,
			Format:      "rgb24",
			Timestamp:   time.Now(),
			FrameNumber: int64(i + 1),
			Source:      "test",
		}
		
		sm.AddFrameToSegment(segment, testFrame)
	}
	
	if segment.FrameCount != 5 {
		t.Errorf("Expected segment to have 5 frames, got %d", segment.FrameCount)
	}
	
	if len(segment.Frames) != 5 {
		t.Errorf("Expected segment frames length to be 5, got %d", len(segment.Frames))
	}
	
	// Test finalizing segment
	sm.FinalizeSegment(segment)
	
	if segment.Duration == 0 {
		t.Error("Expected segment to have duration > 0")
	}
	
	if segment.StartFrame == 0 {
		t.Error("Expected segment to have start frame > 0")
	}
	
	if segment.EndFrame == 0 {
		t.Error("Expected segment to have end frame > 0")
	}
}

// TestQualityAssessment tests quality assessment functionality
func TestQualityAssessment(t *testing.T) {
	config := DefaultFrameProcessorConfig()
	qa := NewQualityAssessor(config)
	
	// Test initialization
	if err := qa.Initialize(); err != nil {
		t.Fatalf("Failed to initialize quality assessor: %v", err)
	}
	
	// Test quality assessment
	testFrame := VideoFrame{
		Data:        make([]byte, 1024),
		Width:       1280,
		Height:      720,
		Format:      "rgb24",
		Timestamp:   time.Now(),
		FrameNumber: 1,
		Source:      "test",
	}
	
	qualityScore, metrics, err := qa.AssessQuality(testFrame)
	if err != nil {
		t.Errorf("Failed to assess quality: %v", err)
	}
	
	if qualityScore == 0 {
		t.Error("Expected quality score > 0")
	}
	
	if qualityScore > 1 {
		t.Error("Expected quality score <= 1")
	}
	
	if len(metrics) == 0 {
		t.Error("Expected quality metrics")
	}
	
	// Test expected metrics
	expectedMetrics := []string{"sharpness", "brightness", "contrast", "noise"}
	for _, metric := range expectedMetrics {
		if _, exists := metrics[metric]; !exists {
			t.Errorf("Expected metric %s to exist", metric)
		}
	}
}

// TestTemporalAnalysis tests temporal analysis functionality
func TestTemporalAnalysis(t *testing.T) {
	config := DefaultFrameProcessorConfig()
	config.TemporalWindowSize = 5
	
	ta := NewTemporalAnalyzer(config)
	
	// Test initialization
	if err := ta.Initialize(); err != nil {
		t.Fatalf("Failed to initialize temporal analyzer: %v", err)
	}
	
	// Test temporal analysis with multiple frames
	for i := 0; i < 10; i++ {
		testFrame := VideoFrame{
			Data:        make([]byte, 1024),
			Width:       1280,
			Height:      720,
			Format:      "rgb24",
			Timestamp:   time.Now(),
			FrameNumber: int64(i + 1),
			Source:      "test",
		}
		
		// Fill with varying data
		for j := range testFrame.Data {
			testFrame.Data[j] = byte((j + i*10) % 256)
		}
		
		features, err := ta.AnalyzeFrame(testFrame)
		if err != nil {
			t.Errorf("Failed to analyze frame temporally: %v", err)
		}
		
		// After the first frame, we should have temporal features
		if i > 0 {
			if len(features.OpticalFlow) == 0 {
				t.Error("Expected optical flow data")
			}
			
			if len(features.MotionVectors) == 0 {
				t.Error("Expected motion vectors")
			}
			
			if features.Stability == 0 {
				t.Error("Expected stability > 0")
			}
		}
	}
}

// BenchmarkVideoStreaming benchmarks video streaming performance
func BenchmarkVideoStreaming(b *testing.B) {
	config := DefaultVideoStreamingConfig()
	config.EnableAdaptiveQuality = false // Disable for consistent benchmarking
	
	uiUpdateChan := make(chan tea.Msg, 1000)
	vsm := NewVideoStreamingManager(config, uiUpdateChan)
	
	if err := vsm.StartStreaming(); err != nil {
		b.Fatalf("Failed to start streaming: %v", err)
	}
	defer vsm.StopStreaming()
	
	testFrame := VideoFrame{
		Data:        make([]byte, 1920*1080*3), // 1080p RGB
		Width:       1920,
		Height:      1080,
		Format:      "rgb24",
		Timestamp:   time.Now(),
		FrameNumber: 1,
		Source:      "benchmark",
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		testFrame.FrameNumber = int64(i + 1)
		testFrame.Timestamp = time.Now()
		
		if err := vsm.ProcessFrame(testFrame); err != nil {
			b.Errorf("Failed to process frame: %v", err)
		}
	}
}

// BenchmarkVideoAnalysis benchmarks video analysis performance
func BenchmarkVideoAnalysis(b *testing.B) {
	config := DefaultVideoAnalysisConfig()
	config.AnalysisInterval = 10 * time.Millisecond // Faster for benchmarking
	
	uiUpdateChan := make(chan tea.Msg, 1000)
	vam := NewVideoAnalysisManager(config, uiUpdateChan)
	
	if err := vam.StartAnalysis(); err != nil {
		b.Fatalf("Failed to start analysis: %v", err)
	}
	defer vam.StopAnalysis()
	
	testFrame := VideoFrame{
		Data:        make([]byte, 1920*1080*3), // 1080p RGB
		Width:       1920,
		Height:      1080,
		Format:      "rgb24",
		Timestamp:   time.Now(),
		FrameNumber: 1,
		Source:      "benchmark",
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		testFrame.FrameNumber = int64(i + 1)
		testFrame.Timestamp = time.Now()
		
		if err := vam.AnalyzeFrame(testFrame); err != nil {
			b.Errorf("Failed to analyze frame: %v", err)
		}
	}
}

// BenchmarkFrameProcessing benchmarks frame processing performance
func BenchmarkFrameProcessing(b *testing.B) {
	config := DefaultFrameProcessorConfig()
	config.ProcessingInterval = 10 * time.Millisecond // Faster for benchmarking
	
	uiUpdateChan := make(chan tea.Msg, 1000)
	fp := NewFrameProcessor(config, uiUpdateChan)
	
	if err := fp.StartProcessing(); err != nil {
		b.Fatalf("Failed to start processing: %v", err)
	}
	defer fp.StopProcessing()
	
	testFrame := VideoFrame{
		Data:        make([]byte, 1920*1080*3), // 1080p RGB
		Width:       1920,
		Height:      1080,
		Format:      "rgb24",
		Timestamp:   time.Now(),
		FrameNumber: 1,
		Source:      "benchmark",
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		testFrame.FrameNumber = int64(i + 1)
		testFrame.Timestamp = time.Now()
		
		if err := fp.ProcessFrame(testFrame); err != nil {
			b.Errorf("Failed to process frame: %v", err)
		}
	}
}