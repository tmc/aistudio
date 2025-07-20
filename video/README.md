# Video Streaming Pipeline

This package implements a comprehensive real-time video streaming pipeline for the AIStudio project with advanced features including adaptive quality control, real-time analysis, and frame-based processing.

## Architecture Overview

The video streaming pipeline consists of three main components:

1. **Video Streaming Manager** (`video_streaming.go`) - Handles real-time video streaming with adaptive quality
2. **Video Analysis Manager** (`video_analysis.go`) - Provides video analysis capabilities with object detection, scene analysis, and motion detection
3. **Frame Processor** (`frame_processor.go`) - Manages frame-based processing and video segmentation with Gemini 2.5 integration

## Features

### Video Streaming (`video_streaming.go`)

- **Real-time Video Streaming**: Supports streaming with configurable frame rates (15-60 FPS)
- **Adaptive Quality Control**: Automatically adjusts resolution, bitrate, and frame rate based on system performance
- **Multiple Video Codecs**: Support for H.264, H.265, VP8, VP9, and AV1
- **Resolution Adaptation**: Dynamic resolution switching from 360p to 4K
- **Network-aware Quality**: Adjusts quality based on network conditions
- **Hardware Acceleration**: Optional GPU acceleration for encoding/decoding
- **Performance Monitoring**: Real-time performance metrics and quality scoring

#### Supported Resolutions
- 4K (3840x2160)
- 1080p (1920x1080)
- 720p (1280x720)
- 480p (854x480)
- 360p (640x360)

#### Supported Codecs
- H.264 (default) - Best compatibility
- H.265 - Better compression
- VP8 - WebM support
- VP9 - Advanced WebM
- AV1 - Next-generation codec

### Video Analysis (`video_analysis.go`)

- **Object Detection**: Real-time object detection with bounding boxes and tracking
- **Scene Analysis**: Comprehensive scene understanding with emotions, activities, and settings
- **Motion Detection**: Advanced motion detection with velocity and direction analysis
- **Performance Optimizations**: GPU acceleration and multi-threading support
- **Confidence Scoring**: Quality metrics for analysis results

#### Object Detection Features
- Support for 20+ object types (person, car, truck, bus, bicycle, etc.)
- Object tracking across frames
- Velocity and direction calculation
- Confidence scoring and filtering

#### Scene Analysis Features
- Setting detection (indoor/outdoor, office, home, etc.)
- Activity recognition (working, walking, sitting, etc.)
- Emotion detection (happy, sad, focused, etc.)
- Lighting and weather analysis
- Scene transition detection

#### Motion Detection Features
- Optical flow calculation
- Motion area detection
- Motion type classification (linear, circular, random)
- Velocity and direction analysis
- Background subtraction

### Frame Processing (`frame_processor.go`)

- **Timestamp-based Analysis**: Frame-by-frame processing with temporal analysis
- **Video Segmentation**: Automatic video segmentation based on scene changes
- **Gemini 2.5 Integration**: Advanced AI analysis using Gemini 2.5 Flash model
- **Multi-frame Processing**: Temporal analysis across frame sequences
- **Quality Assessment**: Comprehensive quality metrics (sharpness, brightness, contrast, noise)
- **Parallel Processing**: Multi-worker frame processing for performance

#### Segmentation Features
- Time-based segmentation
- Scene-based segmentation
- Motion-based segmentation
- Configurable segment length and overlap
- Keyframe extraction

#### Gemini Integration Features
- Frame-by-frame analysis
- Video segment summarization
- Natural language descriptions
- Activity and emotion recognition
- Narrative generation

## Usage Examples

### Basic Video Streaming

```go
import "github.com/tmc/aistudio/video"

// Create configuration
config := video.DefaultVideoStreamingConfig()
config.InitialFPS = 30
config.InitialResolution = video.Resolution1080p
config.EnableAdaptiveQuality = true

// Create streaming manager
uiUpdateChan := make(chan tea.Msg, 100)
vsm := video.NewVideoStreamingManager(config, uiUpdateChan)

// Start streaming
if err := vsm.StartStreaming(); err != nil {
    log.Fatal(err)
}
defer vsm.StopStreaming()

// Process frames
frame := video.VideoFrame{
    Data:        frameData,
    Width:       1920,
    Height:      1080,
    Format:      "rgb24",
    Timestamp:   time.Now(),
    FrameNumber: 1,
    Source:      "camera",
}

if err := vsm.ProcessFrame(frame); err != nil {
    log.Printf("Error processing frame: %v", err)
}

// Get processed frame
processedFrame, err := vsm.GetProcessedFrame()
if err != nil {
    log.Printf("Error getting processed frame: %v", err)
}
```

### Video Analysis

```go
// Create analysis configuration
config := video.DefaultVideoAnalysisConfig()
config.EnableObjectDetection = true
config.EnableSceneAnalysis = true
config.EnableMotionDetection = true

// Create analysis manager
vam := video.NewVideoAnalysisManager(config, uiUpdateChan)

// Start analysis
if err := vam.StartAnalysis(); err != nil {
    log.Fatal(err)
}
defer vam.StopAnalysis()

// Analyze frame
if err := vam.AnalyzeFrame(frame); err != nil {
    log.Printf("Error analyzing frame: %v", err)
}

// Get analysis result
result, err := vam.GetAnalysisResult()
if err != nil {
    log.Printf("Error getting analysis result: %v", err)
}

// Process results
fmt.Printf("Detected %d objects\n", result.ObjectCount)
fmt.Printf("Scene: %s (confidence: %.2f)\n", result.Scene.Setting, result.SceneConfidence)
fmt.Printf("Motion detected: %v (level: %.2f)\n", result.MotionDetected, result.MotionLevel)
```

### Frame Processing with Segmentation

```go
// Create processing configuration
config := video.DefaultFrameProcessorConfig()
config.EnableSegmentation = true
config.EnableGeminiAnalysis = true
config.GeminiAPIKey = "your-api-key"

// Create frame processor
fp := video.NewFrameProcessor(config, uiUpdateChan)

// Start processing
if err := fp.StartProcessing(); err != nil {
    log.Fatal(err)
}
defer fp.StopProcessing()

// Process frame
if err := fp.ProcessFrame(frame); err != nil {
    log.Printf("Error processing frame: %v", err)
}

// Get processed frame
processedFrame, err := fp.GetProcessedFrame()
if err != nil {
    log.Printf("Error getting processed frame: %v", err)
}

// Access processing results
fmt.Printf("Quality score: %.2f\n", processedFrame.QualityScore)
fmt.Printf("Processing time: %v\n", processedFrame.ProcessingTime)
if processedFrame.GeminiAnalysis != nil {
    fmt.Printf("Gemini analysis: %s\n", processedFrame.GeminiAnalysis.Description)
}
```

## Configuration Options

### Video Streaming Configuration

```go
type VideoStreamingConfig struct {
    // Quality settings
    InitialFPS        int           // Starting frame rate (15-60)
    InitialBitrate    int           // Starting bitrate (500K-10M)
    InitialResolution VideoResolution // Starting resolution
    
    // Codec settings
    PreferredCodec    VideoCodec    // Preferred codec (h264, h265, vp8, vp9, av1)
    HardwareAccel     bool          // Enable hardware acceleration
    
    // Adaptation settings
    EnableAdaptiveQuality bool      // Enable dynamic quality adaptation
    QualityAdaptInterval  time.Duration // Quality adaptation interval
    
    // Performance settings
    EnableRealTimeMode bool         // Enable real-time optimizations
    EnableLowLatency   bool         // Enable low-latency mode
    ThreadCount        int          // Number of processing threads
}
```

### Video Analysis Configuration

```go
type VideoAnalysisConfig struct {
    // Analysis modules
    EnableObjectDetection bool      // Enable object detection
    EnableSceneAnalysis   bool      // Enable scene analysis
    EnableMotionDetection bool      // Enable motion detection
    
    // Object detection settings
    ObjectDetectionModel  string    // Model name (yolov8n, etc.)
    ConfidenceThreshold   float64   // Minimum confidence (0.0-1.0)
    MaxObjectsPerFrame    int       // Maximum objects per frame
    
    // Performance settings
    EnableGPUAcceleration bool      // Enable GPU acceleration
    ThreadCount           int       // Number of processing threads
    AnalysisInterval      time.Duration // Analysis interval
}
```

### Frame Processing Configuration

```go
type FrameProcessorConfig struct {
    // Processing settings
    EnableFrameAnalysis    bool      // Enable frame analysis
    EnableSegmentation     bool      // Enable video segmentation
    EnableGeminiAnalysis   bool      // Enable Gemini AI analysis
    
    // Segmentation settings
    SegmentationMethod     string    // Method (time, scene, motion, content)
    MinSegmentDuration     time.Duration // Minimum segment duration
    MaxSegmentLength       int       // Maximum frames per segment
    
    // Gemini settings
    GeminiModelName        string    // Gemini model (gemini-2.5-flash)
    GeminiAPIKey          string    // API key
    GeminiPromptTemplate   string    // Analysis prompt template
    
    // Performance settings
    EnableParallelProcessing bool    // Enable parallel processing
    WorkerCount             int      // Number of worker threads
}
```

## Performance Optimizations

### Adaptive Quality Control

The streaming manager automatically adjusts quality based on:
- **CPU Usage**: Reduces FPS when CPU > 80%
- **Memory Usage**: Lowers resolution when memory > 85%
- **Network Bandwidth**: Adjusts bitrate based on available bandwidth
- **Processing Time**: Optimizes settings for target latency

### Hardware Acceleration

Supports hardware acceleration for:
- **macOS**: VideoToolbox (H.264/H.265)
- **Linux**: VAAPI, NVENC (GPU-dependent)
- **Windows**: D3D11VA, DXVA2

### Multi-threading

- **Parallel Processing**: Multiple worker threads for frame processing
- **Pipeline Processing**: Separate threads for encoding, analysis, and output
- **Async Operations**: Non-blocking frame processing and analysis

## Dependencies

### Required System Dependencies

- **FFmpeg**: For video encoding/decoding and format conversion
- **Optional**: Hardware acceleration libraries (VideoToolbox, VAAPI, etc.)

### Go Dependencies

- `github.com/charmbracelet/bubbletea`: UI framework integration
- Standard library packages for concurrency and networking

## Testing

Run the comprehensive test suite:

```bash
# Run all tests
go test ./video

# Run specific test
go test ./video -run TestVideoStreaming

# Run benchmarks
go test ./video -bench=.

# Run with verbose output
go test ./video -v
```

### Test Coverage

- **Unit Tests**: Individual component testing
- **Integration Tests**: End-to-end pipeline testing
- **Performance Tests**: Benchmarking and stress testing
- **Error Handling**: Comprehensive error condition testing

## Error Handling

The pipeline includes comprehensive error handling for:
- **Codec Initialization**: Fallback to alternative codecs
- **Network Issues**: Automatic quality adaptation
- **Resource Constraints**: Graceful degradation
- **API Failures**: Retry logic and fallback mechanisms

## Monitoring and Metrics

### Performance Metrics

- **Frame Rate**: Actual vs. target FPS
- **Bitrate**: Current encoding bitrate
- **Quality Score**: Objective quality assessment
- **Processing Time**: Frame processing latency
- **Buffer Utilization**: Input/output buffer usage

### Quality Metrics

- **Sharpness**: Edge detection and clarity
- **Brightness**: Luminance analysis
- **Contrast**: Dynamic range assessment
- **Noise**: Artifact detection

### System Metrics

- **CPU Usage**: Processing load
- **Memory Usage**: RAM utilization
- **GPU Usage**: Hardware acceleration usage
- **Network Bandwidth**: Available bandwidth

## Integration with AIStudio

The video pipeline integrates seamlessly with the existing AIStudio architecture:

- **UI Integration**: Bubble Tea message system for real-time updates
- **API Integration**: Compatible with existing API client architecture
- **Configuration**: Follows AIStudio configuration patterns
- **Logging**: Integrated with AIStudio logging system

## Future Enhancements

- **WebRTC Support**: Direct browser streaming
- **Cloud Processing**: Offload processing to cloud services
- **ML Model Training**: Custom model training for specific use cases
- **Advanced Analytics**: Detailed performance analytics dashboard
- **Mobile Support**: iOS/Android streaming capabilities

## License

This code is part of the AIStudio project and follows the same licensing terms.