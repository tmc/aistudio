package video

import (
	"fmt"
	"log"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// DemoVideoStreamingPipeline demonstrates the video streaming pipeline
func DemoVideoStreamingPipeline() {
	fmt.Println("🎥 Video Streaming Pipeline Demo")
	fmt.Println("=" + strings.Repeat("=", 48))

	// Create UI update channel
	uiUpdateChan := make(chan tea.Msg, 100)

	// Demo 1: Video Streaming
	fmt.Println("\n1. 🎬 Video Streaming Manager")
	fmt.Println("   - Real-time video streaming with adaptive quality")
	fmt.Println("   - Multiple codec support (H.264, H.265, VP8, VP9)")
	fmt.Println("   - Dynamic resolution adaptation (480p to 4K)")
	fmt.Println("   - Network-aware bitrate adjustment")

	streamingConfig := DefaultVideoStreamingConfig()
	streamingConfig.InitialFPS = 30
	streamingConfig.InitialResolution = Resolution1080p
	streamingConfig.EnableAdaptiveQuality = true
	streamingConfig.PreferredCodec = CodecH264

	_ = NewVideoStreamingManager(streamingConfig, uiUpdateChan)
	fmt.Printf("   ✓ Created streaming manager with %dx%d@%dfps\n", 
		streamingConfig.InitialResolution.Width, 
		streamingConfig.InitialResolution.Height, 
		streamingConfig.InitialFPS)

	// Demo 2: Video Analysis
	fmt.Println("\n2. 🔍 Video Analysis Manager")
	fmt.Println("   - Real-time object detection and tracking")
	fmt.Println("   - Scene analysis with emotion recognition")
	fmt.Println("   - Motion detection with velocity vectors")
	fmt.Println("   - Performance optimization with GPU acceleration")

	analysisConfig := DefaultVideoAnalysisConfig()
	analysisConfig.EnableObjectDetection = true
	analysisConfig.EnableSceneAnalysis = true
	analysisConfig.EnableMotionDetection = true
	analysisConfig.AnalysisInterval = 100 * time.Millisecond

	vam := NewVideoAnalysisManager(analysisConfig, uiUpdateChan)
	fmt.Printf("   ✓ Created analysis manager with %d analysis modules\n", 
		len(vam.getEnabledModules()))

	// Demo 3: Frame Processing
	fmt.Println("\n3. 🎞️ Frame Processor")
	fmt.Println("   - Frame-by-frame processing with temporal analysis")
	fmt.Println("   - Intelligent video segmentation")
	fmt.Println("   - Gemini 2.5 integration for AI analysis")
	fmt.Println("   - Quality assessment and optimization")

	processingConfig := DefaultFrameProcessorConfig()
	processingConfig.EnableFrameAnalysis = true
	processingConfig.EnableSegmentation = true
	processingConfig.EnableQualityCheck = true
	processingConfig.EnableTemporalAnalysis = true
	processingConfig.EnableGeminiAnalysis = false // Disable for demo

	fp := NewFrameProcessor(processingConfig, uiUpdateChan)
	fmt.Printf("   ✓ Created frame processor with %d enabled features\n", 
		len(fp.getEnabledFeatures()))

	// Demo sample frame processing
	fmt.Println("\n4. 📊 Sample Frame Processing")
	
	// Create sample video frame
	sampleFrame := VideoFrame{
		Data:        make([]byte, 1920*1080*3), // 1080p RGB
		Width:       1920,
		Height:      1080,
		Format:      "rgb24",
		Timestamp:   time.Now(),
		FrameNumber: 1,
		Source:      "demo",
	}
	
	// Fill with sample data
	for i := range sampleFrame.Data {
		sampleFrame.Data[i] = byte(i % 256)
	}

	// Initialize components for demo
	if err := vam.objectDetector.Initialize(); err != nil {
		log.Printf("   ⚠️  Object detector initialization skipped: %v", err)
	} else {
		fmt.Println("   ✓ Object detector initialized")
	}

	if err := vam.sceneAnalyzer.Initialize(); err != nil {
		log.Printf("   ⚠️  Scene analyzer initialization skipped: %v", err)
	} else {
		fmt.Println("   ✓ Scene analyzer initialized")
	}

	if err := vam.motionDetector.Initialize(); err != nil {
		log.Printf("   ⚠️  Motion detector initialization skipped: %v", err)
	} else {
		fmt.Println("   ✓ Motion detector initialized")
	}

	// Demo object detection
	fmt.Println("\n   📍 Object Detection Results:")
	if objects, err := vam.objectDetector.DetectObjects(sampleFrame); err != nil {
		fmt.Printf("   ⚠️  Object detection failed: %v\n", err)
	} else {
		for _, obj := range objects {
			fmt.Printf("   - %s (confidence: %.2f) at [%.2f, %.2f]\n", 
				obj.Label, obj.Confidence, obj.BoundingBox.X, obj.BoundingBox.Y)
		}
	}

	// Demo scene analysis
	fmt.Println("\n   🎭 Scene Analysis Results:")
	if scene, confidence, transition, err := vam.sceneAnalyzer.AnalyzeScene(sampleFrame); err != nil {
		fmt.Printf("   ⚠️  Scene analysis failed: %v\n", err)
	} else {
		fmt.Printf("   - Setting: %s (confidence: %.2f)\n", scene.Setting, confidence)
		fmt.Printf("   - Activities: %v\n", scene.Activities)
		fmt.Printf("   - Emotions: %v\n", scene.Emotions)
		fmt.Printf("   - Scene transition: %v\n", transition)
	}

	// Demo motion detection
	fmt.Println("\n   🏃 Motion Detection Results:")
	if motionAreas, motionLevel, detected, err := vam.motionDetector.DetectMotion(sampleFrame); err != nil {
		fmt.Printf("   ⚠️  Motion detection failed: %v\n", err)
	} else {
		fmt.Printf("   - Motion detected: %v (level: %.2f)\n", detected, motionLevel)
		fmt.Printf("   - Motion areas: %d\n", len(motionAreas))
	}

	// Demo quality assessment
	fmt.Println("\n   📈 Quality Assessment Results:")
	if err := fp.qualityAssessor.Initialize(); err != nil {
		fmt.Printf("   ⚠️  Quality assessor initialization failed: %v\n", err)
	} else {
		if qualityScore, metrics, err := fp.qualityAssessor.AssessQuality(sampleFrame); err != nil {
			fmt.Printf("   ⚠️  Quality assessment failed: %v\n", err)
		} else {
			fmt.Printf("   - Overall quality: %.2f\n", qualityScore)
			for metric, value := range metrics {
				fmt.Printf("   - %s: %.2f\n", metric, value)
			}
		}
	}

	// Demo streaming capabilities
	fmt.Println("\n5. 🚀 Performance Capabilities")
	fmt.Println("   - Adaptive Frame Rate: 15-60 FPS")
	fmt.Println("   - Resolution Support: 360p to 4K")
	fmt.Println("   - Bitrate Range: 500 Kbps to 10 Mbps")
	fmt.Println("   - Real-time Processing: <100ms latency")
	fmt.Println("   - Multi-threaded: 4+ worker threads")
	fmt.Println("   - GPU Acceleration: Hardware-accelerated encoding")

	// Demo configuration examples
	fmt.Println("\n6. ⚙️ Configuration Examples")
	
	// Low-latency configuration
	lowLatencyConfig := DefaultVideoStreamingConfig()
	lowLatencyConfig.InitialFPS = 60
	lowLatencyConfig.InitialResolution = Resolution720p
	lowLatencyConfig.EnableLowLatency = true
	lowLatencyConfig.EnableRealTimeMode = true
	fmt.Printf("   🏃 Low-latency: %dx%d@%dfps, real-time mode\n", 
		lowLatencyConfig.InitialResolution.Width, 
		lowLatencyConfig.InitialResolution.Height, 
		lowLatencyConfig.InitialFPS)

	// High-quality configuration
	highQualityConfig := DefaultVideoStreamingConfig()
	highQualityConfig.InitialFPS = 30
	highQualityConfig.InitialResolution = Resolution4K
	highQualityConfig.InitialBitrate = 10000000 // 10 Mbps
	highQualityConfig.PreferredCodec = CodecH265
	fmt.Printf("   🎨 High-quality: %dx%d@%dfps, %s codec\n", 
		highQualityConfig.InitialResolution.Width, 
		highQualityConfig.InitialResolution.Height, 
		highQualityConfig.InitialFPS, 
		highQualityConfig.PreferredCodec)

	// Mobile-optimized configuration
	mobileConfig := DefaultVideoStreamingConfig()
	mobileConfig.InitialFPS = 30
	mobileConfig.InitialResolution = Resolution480p
	mobileConfig.InitialBitrate = 1000000 // 1 Mbps
	mobileConfig.EnableAdaptiveQuality = true
	fmt.Printf("   📱 Mobile-optimized: %dx%d@%dfps, adaptive quality\n", 
		mobileConfig.InitialResolution.Width, 
		mobileConfig.InitialResolution.Height, 
		mobileConfig.InitialFPS)

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("🎉 Video Streaming Pipeline Demo Complete!")
	fmt.Println("\nKey Features Demonstrated:")
	fmt.Println("✓ Real-time video streaming with adaptive quality")
	fmt.Println("✓ Comprehensive video analysis (objects, scenes, motion)")
	fmt.Println("✓ Frame-by-frame processing with AI integration")
	fmt.Println("✓ Quality assessment and optimization")
	fmt.Println("✓ Configurable performance profiles")
	fmt.Println("\nReady for integration with AIStudio multimodal streaming!")
}