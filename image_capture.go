package aistudio

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ImageCaptureManager handles image and screen capture functionality
type ImageCaptureManager struct {
	isCapturing      bool
	captureInterval  time.Duration
	captureContext   context.Context
	captureCancel    context.CancelFunc
	outputFormat     string
	compressionLevel int
	
	// Screen capture specific
	screenRegion     *ScreenRegion
	displayID        int
	captureWindow    string
	
	// Image data channels
	imageChan        chan ImageFrame
	uiUpdateChan     chan tea.Msg
	
	// Synchronization
	mu               sync.RWMutex
	
	// Configuration
	enableThumbnails bool
	thumbnailSize    int
	enableTimestamp  bool
	captureQuality   int
}

// ScreenRegion defines a rectangular region for screen capture
type ScreenRegion struct {
	X      int
	Y      int
	Width  int
	Height int
}

// ImageFrame represents a captured image frame
type ImageFrame struct {
	Data        []byte
	Format      string
	Width       int
	Height      int
	Timestamp   time.Time
	Source      string // "screen", "camera", "file"
	Filename    string
	Size        int64
	Thumbnail   []byte // Optional thumbnail data
}

// WindowInfo represents information about a window
type WindowInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ProcessName    string `json:"process_name"`
	Bundle         string `json:"bundle,omitempty"`
	X              int    `json:"x"`
	Y              int    `json:"y"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	IsMinimized    bool   `json:"is_minimized"`
	IsVisible      bool   `json:"is_visible"`
	IsOnScreen     bool   `json:"is_on_screen"`
	OwnerPID       int    `json:"owner_pid"`
}

// ImageCaptureConfig contains configuration for image capture
type ImageCaptureConfig struct {
	CaptureInterval  time.Duration
	OutputFormat     string
	CompressionLevel int
	ScreenRegion     *ScreenRegion
	DisplayID        int
	CaptureWindow    string
	EnableThumbnails bool
	ThumbnailSize    int
	EnableTimestamp  bool
	CaptureQuality   int
}

// DefaultImageCaptureConfig returns default configuration for image capture
func DefaultImageCaptureConfig() ImageCaptureConfig {
	return ImageCaptureConfig{
		CaptureInterval:  2 * time.Second, // Capture every 2 seconds
		OutputFormat:     "png",           // PNG format
		CompressionLevel: 6,               // Medium compression
		ScreenRegion:     nil,             // Full screen
		DisplayID:        1,               // Primary display
		CaptureWindow:    "",              // No specific window
		EnableThumbnails: true,            // Generate thumbnails
		ThumbnailSize:    150,             // 150x150 thumbnails
		EnableTimestamp:  true,            // Add timestamp to filename
		CaptureQuality:   80,              // 80% quality for JPEG
	}
}

// NewImageCaptureManager creates a new image capture manager
func NewImageCaptureManager(config ImageCaptureConfig, uiUpdateChan chan tea.Msg) *ImageCaptureManager {
	return &ImageCaptureManager{
		captureInterval:  config.CaptureInterval,
		outputFormat:     config.OutputFormat,
		compressionLevel: config.CompressionLevel,
		screenRegion:     config.ScreenRegion,
		displayID:        config.DisplayID,
		captureWindow:    config.CaptureWindow,
		enableThumbnails: config.EnableThumbnails,
		thumbnailSize:    config.ThumbnailSize,
		enableTimestamp:  config.EnableTimestamp,
		captureQuality:   config.CaptureQuality,
		imageChan:        make(chan ImageFrame, 20), // Buffer for 20 frames
		uiUpdateChan:     uiUpdateChan,
	}
}

// StartScreenCapture begins periodic screen capture
func (icm *ImageCaptureManager) StartScreenCapture() error {
	icm.mu.Lock()
	defer icm.mu.Unlock()
	
	if icm.isCapturing {
		return fmt.Errorf("screen capture already in progress")
	}
	
	// Verify screen capture tools are available
	if err := icm.checkScreenCaptureTools(); err != nil {
		return fmt.Errorf("screen capture tools not available: %w", err)
	}
	
	// Create capture context
	icm.captureContext, icm.captureCancel = context.WithCancel(context.Background())
	
	icm.isCapturing = true
	log.Printf("[IMAGE_CAPTURE] Started screen capture with interval: %v, format: %s", 
		icm.captureInterval, icm.outputFormat)
	
	// Start capture goroutine
	go icm.captureLoop()
	
	// Notify UI
	if icm.uiUpdateChan != nil {
		icm.uiUpdateChan <- ImageCaptureStartedMsg{}
	}
	
	return nil
}

// StopScreenCapture stops screen capture
func (icm *ImageCaptureManager) StopScreenCapture() error {
	icm.mu.Lock()
	defer icm.mu.Unlock()
	
	if !icm.isCapturing {
		return fmt.Errorf("no screen capture in progress")
	}
	
	// Cancel capture context
	if icm.captureCancel != nil {
		icm.captureCancel()
	}
	
	icm.isCapturing = false
	log.Printf("[IMAGE_CAPTURE] Stopped screen capture")
	
	// Notify UI
	if icm.uiUpdateChan != nil {
		icm.uiUpdateChan <- ImageCaptureStoppedMsg{}
	}
	
	return nil
}

// IsCapturing returns whether screen capture is active
func (icm *ImageCaptureManager) IsCapturing() bool {
	icm.mu.RLock()
	defer icm.mu.RUnlock()
	return icm.isCapturing
}

// GetImageChannel returns the channel for receiving image frames
func (icm *ImageCaptureManager) GetImageChannel() <-chan ImageFrame {
	return icm.imageChan
}

// captureLoop runs the periodic screen capture
func (icm *ImageCaptureManager) captureLoop() {
	log.Printf("[IMAGE_CAPTURE] Started capture loop")
	ticker := time.NewTicker(icm.captureInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-icm.captureContext.Done():
			log.Printf("[IMAGE_CAPTURE] Capture loop stopped")
			return
		case <-ticker.C:
			if _, err := icm.captureScreen(); err != nil {
				log.Printf("[IMAGE_CAPTURE] Error capturing screen: %v", err)
				if icm.uiUpdateChan != nil {
					icm.uiUpdateChan <- ImageCaptureErrorMsg{Error: err}
				}
			}
		}
	}
}

// captureScreen captures a single screen frame
func (icm *ImageCaptureManager) captureScreen() (ImageFrame, error) {
	var frame ImageFrame
	var err error
	
	if icm.isMacOS() {
		frame, err = icm.captureMacOSScreen()
	} else {
		frame, err = icm.captureLinuxScreen()
	}
	
	if err != nil {
		return ImageFrame{}, fmt.Errorf("failed to capture screen: %w", err)
	}
	
	// Generate thumbnail if enabled
	if icm.enableThumbnails {
		thumbnail, err := icm.generateThumbnail(frame.Data, frame.Format)
		if err != nil {
			log.Printf("[IMAGE_CAPTURE] Warning: failed to generate thumbnail: %v", err)
		} else {
			frame.Thumbnail = thumbnail
		}
	}
	
	// Send to processing channel
	select {
	case icm.imageChan <- frame:
		log.Printf("[IMAGE_CAPTURE] Captured frame: %d bytes, format: %s", len(frame.Data), frame.Format)
	default:
		log.Printf("[IMAGE_CAPTURE] Warning: image buffer full, dropping frame")
	}
	
	return frame, nil
}

// captureMacOSScreen captures screen on macOS using screencapture
func (icm *ImageCaptureManager) captureMacOSScreen() (ImageFrame, error) {
	args := []string{
		"-x",              // No sound
		"-t", icm.outputFormat, // Format
	}
	
	// Add display selection
	if icm.displayID > 0 {
		args = append(args, "-D", fmt.Sprintf("%d", icm.displayID))
	}
	
	// Add region if specified
	if icm.screenRegion != nil {
		regionStr := fmt.Sprintf("%d,%d,%d,%d", 
			icm.screenRegion.X, icm.screenRegion.Y,
			icm.screenRegion.Width, icm.screenRegion.Height)
		args = append(args, "-R", regionStr)
	}
	
	// Add window selection if specified
	if icm.captureWindow != "" {
		args = append(args, "-l", icm.captureWindow)
	}
	
	// Create temporary file
	filename := icm.generateFilename()
	args = append(args, filename)
	
	// Execute screencapture
	cmd := exec.CommandContext(icm.captureContext, "screencapture", args...)
	if err := cmd.Run(); err != nil {
		return ImageFrame{}, fmt.Errorf("screencapture failed: %w", err)
	}
	
	// Read the captured file
	data, err := os.ReadFile(filename)
	if err != nil {
		return ImageFrame{}, fmt.Errorf("failed to read captured file: %w", err)
	}
	
	// Clean up temporary file
	defer os.Remove(filename)
	
	// Get image dimensions (simplified - could use image library for actual dimensions)
	width, height := icm.getImageDimensions(data, icm.outputFormat)
	
	frame := ImageFrame{
		Data:      data,
		Format:    icm.outputFormat,
		Width:     width,
		Height:    height,
		Timestamp: time.Now(),
		Source:    "screen",
		Filename:  filename,
		Size:      int64(len(data)),
	}
	
	return frame, nil
}

// captureLinuxScreen captures screen on Linux using scrot or gnome-screenshot
func (icm *ImageCaptureManager) captureLinuxScreen() (ImageFrame, error) {
	var cmd *exec.Cmd
	filename := icm.generateFilename()
	
	// Try scrot first
	if _, err := exec.LookPath("scrot"); err == nil {
		args := []string{
			"-z",              // Compression
			"-q", fmt.Sprintf("%d", icm.captureQuality), // Quality
		}
		
		// Add region if specified
		if icm.screenRegion != nil {
			regionStr := fmt.Sprintf("%d,%d,%d,%d", 
				icm.screenRegion.X, icm.screenRegion.Y,
				icm.screenRegion.Width, icm.screenRegion.Height)
			args = append(args, "-a", regionStr)
		}
		
		args = append(args, filename)
		cmd = exec.CommandContext(icm.captureContext, "scrot", args...)
		
	} else if _, err := exec.LookPath("gnome-screenshot"); err == nil {
		args := []string{
			"-f", filename,     // Output file
		}
		
		// Add window selection if specified
		if icm.captureWindow != "" {
			args = append(args, "-w")
		}
		
		cmd = exec.CommandContext(icm.captureContext, "gnome-screenshot", args...)
		
	} else {
		return ImageFrame{}, fmt.Errorf("no screen capture tool found (scrot or gnome-screenshot required)")
	}
	
	// Execute capture command
	if err := cmd.Run(); err != nil {
		return ImageFrame{}, fmt.Errorf("screen capture failed: %w", err)
	}
	
	// Read the captured file
	data, err := os.ReadFile(filename)
	if err != nil {
		return ImageFrame{}, fmt.Errorf("failed to read captured file: %w", err)
	}
	
	// Clean up temporary file
	defer os.Remove(filename)
	
	// Get image dimensions
	width, height := icm.getImageDimensions(data, icm.outputFormat)
	
	frame := ImageFrame{
		Data:      data,
		Format:    icm.outputFormat,
		Width:     width,
		Height:    height,
		Timestamp: time.Now(),
		Source:    "screen",
		Filename:  filename,
		Size:      int64(len(data)),
	}
	
	return frame, nil
}

// checkScreenCaptureTools verifies that required tools are available
func (icm *ImageCaptureManager) checkScreenCaptureTools() error {
	if icm.isMacOS() {
		// screencapture is built-in on macOS
		if _, err := exec.LookPath("screencapture"); err != nil {
			return fmt.Errorf("screencapture command not found")
		}
		return nil
	}
	
	// Check for Linux tools
	if _, err := exec.LookPath("scrot"); err == nil {
		return nil
	}
	if _, err := exec.LookPath("gnome-screenshot"); err == nil {
		return nil
	}
	
	return fmt.Errorf("no screen capture tool found - install scrot or gnome-screenshot")
}

// generateFilename generates a unique filename for captures
func (icm *ImageCaptureManager) generateFilename() string {
	timestamp := time.Now().Format("20060102_150405")
	if icm.enableTimestamp {
		return fmt.Sprintf("/tmp/aistudio_capture_%s.%s", timestamp, icm.outputFormat)
	}
	return fmt.Sprintf("/tmp/aistudio_capture.%s", icm.outputFormat)
}

// getImageDimensions returns image dimensions (simplified implementation)
func (icm *ImageCaptureManager) getImageDimensions(data []byte, format string) (int, int) {
	// This is a simplified implementation - in practice, you'd use image libraries
	// to decode the image and get actual dimensions
	switch format {
	case "png":
		return icm.getPNGDimensions(data)
	case "jpg", "jpeg":
		return icm.getJPEGDimensions(data)
	default:
		return 1920, 1080 // Default dimensions
	}
}

// getPNGDimensions extracts dimensions from PNG data
func (icm *ImageCaptureManager) getPNGDimensions(data []byte) (int, int) {
	// Simplified PNG dimension extraction
	if len(data) < 24 {
		return 1920, 1080
	}
	
	// PNG dimensions are at bytes 16-23
	width := int(data[16])<<24 | int(data[17])<<16 | int(data[18])<<8 | int(data[19])
	height := int(data[20])<<24 | int(data[21])<<16 | int(data[22])<<8 | int(data[23])
	
	return width, height
}

// getJPEGDimensions extracts dimensions from JPEG data
func (icm *ImageCaptureManager) getJPEGDimensions(data []byte) (int, int) {
	// Simplified JPEG dimension extraction - would need proper JPEG parser
	// For now, return default
	return 1920, 1080
}

// generateThumbnail generates a thumbnail from image data
func (icm *ImageCaptureManager) generateThumbnail(data []byte, format string) ([]byte, error) {
	// Create a temporary file for the original image
	tempFile, err := os.CreateTemp("", fmt.Sprintf("thumb_*.%s", format))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	
	// Write original image data
	if _, err := tempFile.Write(data); err != nil {
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}
	tempFile.Close()
	
	// Create thumbnail using sips (macOS) or convert (Linux)
	thumbFile := fmt.Sprintf("%s_thumb.%s", tempFile.Name(), format)
	defer os.Remove(thumbFile)
	
	var cmd *exec.Cmd
	if icm.isMacOS() {
		// Use sips to resize
		cmd = exec.Command("sips", 
			"-Z", fmt.Sprintf("%d", icm.thumbnailSize),
			tempFile.Name(),
			"--out", thumbFile)
	} else {
		// Use ImageMagick convert
		cmd = exec.Command("convert", 
			tempFile.Name(),
			"-resize", fmt.Sprintf("%dx%d", icm.thumbnailSize, icm.thumbnailSize),
			thumbFile)
	}
	
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("thumbnail generation failed: %w", err)
	}
	
	// Read thumbnail data
	thumbData, err := os.ReadFile(thumbFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read thumbnail: %w", err)
	}
	
	return thumbData, nil
}

// isMacOS checks if running on macOS
func (icm *ImageCaptureManager) isMacOS() bool {
	cmd := exec.Command("uname", "-s")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return string(output) == "Darwin\n"
}

// CaptureScreenOnce captures a single screen frame immediately
func (icm *ImageCaptureManager) CaptureScreenOnce() (ImageFrame, error) {
	return icm.captureScreen()
}

// CaptureFromFile loads an image from a file
func (icm *ImageCaptureManager) CaptureFromFile(filename string) (ImageFrame, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return ImageFrame{}, fmt.Errorf("failed to read file: %w", err)
	}
	
	// Determine format from filename
	format := "png"
	if len(filename) > 4 {
		switch filename[len(filename)-4:] {
		case ".jpg", ".jpeg":
			format = "jpg"
		case ".png":
			format = "png"
		}
	}
	
	width, height := icm.getImageDimensions(data, format)
	
	frame := ImageFrame{
		Data:      data,
		Format:    format,
		Width:     width,
		Height:    height,
		Timestamp: time.Now(),
		Source:    "file",
		Filename:  filename,
		Size:      int64(len(data)),
	}
	
	return frame, nil
}

// Image capture related messages
type ImageCaptureStartedMsg struct{}
type ImageCaptureStoppedMsg struct{}
type ImageCaptureErrorMsg struct {
	Error error
}
type ImageFrameMsg struct {
	Frame ImageFrame
}

// ToggleScreenCapture toggles screen capture state
func (icm *ImageCaptureManager) ToggleScreenCapture() error {
	if icm.IsCapturing() {
		return icm.StopScreenCapture()
	}
	return icm.StartScreenCapture()
}

// SetCaptureInterval updates the capture interval
func (icm *ImageCaptureManager) SetCaptureInterval(interval time.Duration) error {
	icm.mu.Lock()
	defer icm.mu.Unlock()
	
	if interval < 100*time.Millisecond {
		return fmt.Errorf("capture interval too short (minimum 100ms)")
	}
	
	icm.captureInterval = interval
	log.Printf("[IMAGE_CAPTURE] Set capture interval to: %v", interval)
	return nil
}

// SetScreenRegion sets the screen region to capture
func (icm *ImageCaptureManager) SetScreenRegion(region *ScreenRegion) {
	icm.mu.Lock()
	defer icm.mu.Unlock()
	
	icm.screenRegion = region
	if region != nil {
		log.Printf("[IMAGE_CAPTURE] Set screen region: %dx%d at (%d,%d)", 
			region.Width, region.Height, region.X, region.Y)
	} else {
		log.Printf("[IMAGE_CAPTURE] Cleared screen region (full screen)")
	}
}

// GetConfig returns current image capture configuration
func (icm *ImageCaptureManager) GetConfig() ImageCaptureConfig {
	icm.mu.RLock()
	defer icm.mu.RUnlock()
	
	return ImageCaptureConfig{
		CaptureInterval:  icm.captureInterval,
		OutputFormat:     icm.outputFormat,
		CompressionLevel: icm.compressionLevel,
		ScreenRegion:     icm.screenRegion,
		DisplayID:        icm.displayID,
		CaptureWindow:    icm.captureWindow,
		EnableThumbnails: icm.enableThumbnails,
		ThumbnailSize:    icm.thumbnailSize,
		EnableTimestamp:  icm.enableTimestamp,
		CaptureQuality:   icm.captureQuality,
	}
}

// UpdateConfig updates image capture configuration
func (icm *ImageCaptureManager) UpdateConfig(config ImageCaptureConfig) error {
	icm.mu.Lock()
	defer icm.mu.Unlock()
	
	if icm.isCapturing {
		return fmt.Errorf("cannot update configuration while capturing")
	}
	
	icm.captureInterval = config.CaptureInterval
	icm.outputFormat = config.OutputFormat
	icm.compressionLevel = config.CompressionLevel
	icm.screenRegion = config.ScreenRegion
	icm.displayID = config.DisplayID
	icm.captureWindow = config.CaptureWindow
	icm.enableThumbnails = config.EnableThumbnails
	icm.thumbnailSize = config.ThumbnailSize
	icm.enableTimestamp = config.EnableTimestamp
	icm.captureQuality = config.CaptureQuality
	
	log.Printf("[IMAGE_CAPTURE] Updated configuration: interval=%v, format=%s", 
		icm.captureInterval, icm.outputFormat)
	
	return nil
}

// StreamFromCamera starts streaming from camera (if available)
func (icm *ImageCaptureManager) StreamFromCamera() error {
	// This would require additional camera streaming implementation
	// For now, return not implemented
	return fmt.Errorf("camera streaming not implemented")
}

// GetAvailableDisplays returns available display IDs
func (icm *ImageCaptureManager) GetAvailableDisplays() ([]int, error) {
	if icm.isMacOS() {
		return icm.getMacOSDisplays()
	}
	return icm.getLinuxDisplays()
}

// getMacOSDisplays gets available displays on macOS
func (icm *ImageCaptureManager) getMacOSDisplays() ([]int, error) {
	// Use system_profiler to get display info
	cmd := exec.Command("system_profiler", "SPDisplaysDataType")
	output, err := cmd.Output()
	if err != nil {
		return []int{1}, nil // Fallback to primary display
	}
	
	// Parse output (simplified)
	displays := []int{1}
	if len(output) > 0 {
		// Could parse actual display IDs from output
		displays = append(displays, 2) // Assume secondary display might exist
	}
	
	return displays, nil
}

// getLinuxDisplays gets available displays on Linux
func (icm *ImageCaptureManager) getLinuxDisplays() ([]int, error) {
	// Use xrandr to get display info
	cmd := exec.Command("xrandr", "--listmonitors")
	output, err := cmd.Output()
	if err != nil {
		return []int{0}, nil // Fallback to default
	}
	
	// Parse output (simplified)
	displays := []int{0}
	if len(output) > 0 {
		// Could parse actual display information
		displays = append(displays, 1)
	}
	
	return displays, nil
}

// ListWindows returns a list of all available windows
func (icm *ImageCaptureManager) ListWindows() ([]WindowInfo, error) {
	if icm.isMacOS() {
		return icm.listMacOSWindows()
	}
	return icm.listLinuxWindows()
}

// listMacOSWindows lists windows on macOS using window server
func (icm *ImageCaptureManager) listMacOSWindows() ([]WindowInfo, error) {
	var windows []WindowInfo
	
	// Method 1: Use AppleScript to get window information
	script := `
		tell application "System Events"
			set windowList to {}
			repeat with proc in (every process whose background only is false)
				try
					set procName to name of proc
					repeat with win in (every window of proc)
						try
							set windowName to name of win
							set windowPos to position of win
							set windowSize to size of win
							set windowID to id of win
							set windowList to windowList & {procName & "|" & windowName & "|" & (item 1 of windowPos) & "|" & (item 2 of windowPos) & "|" & (item 1 of windowSize) & "|" & (item 2 of windowSize) & "|" & windowID}
						end try
					end repeat
				end try
			end repeat
			return windowList
		end tell
	`
	
	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("[IMAGE_CAPTURE] Warning: AppleScript method failed: %v", err)
		return icm.listMacOSWindowsAlternative()
	}
	
	// Parse the AppleScript output
	windows = icm.parseAppleScriptOutput(string(output))
	
	return windows, nil
}

// listMacOSWindowsAlternative uses alternative method for macOS window listing
func (icm *ImageCaptureManager) listMacOSWindowsAlternative() ([]WindowInfo, error) {
	var windows []WindowInfo
	
	// Method 2: Use screencapture -l to list windows
	cmd := exec.Command("screencapture", "-l")
	output, err := cmd.Output()
	if err != nil {
		return windows, fmt.Errorf("failed to list windows: %w", err)
	}
	
	// Parse screencapture output
	windows = icm.parseScreencaptureOutput(string(output))
	
	return windows, nil
}

// parseAppleScriptOutput parses AppleScript output into WindowInfo structures
func (icm *ImageCaptureManager) parseAppleScriptOutput(output string) []WindowInfo {
	var windows []WindowInfo
	
	// Split by lines and parse each window entry
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		
		// Parse format: "processName|windowName|x|y|width|height|windowID"
		parts := strings.Split(line, "|")
		if len(parts) >= 7 {
			var x, y, width, height int
			fmt.Sscanf(parts[2], "%d", &x)
			fmt.Sscanf(parts[3], "%d", &y)
			fmt.Sscanf(parts[4], "%d", &width)
			fmt.Sscanf(parts[5], "%d", &height)
			
			window := WindowInfo{
				ID:          parts[6],
				Name:        parts[1],
				ProcessName: parts[0],
				X:           x,
				Y:           y,
				Width:       width,
				Height:      height,
				IsVisible:   true,
				IsOnScreen:  true,
			}
			windows = append(windows, window)
		}
	}
	
	return windows
}

// parseScreencaptureOutput parses screencapture -l output into WindowInfo structures
func (icm *ImageCaptureManager) parseScreencaptureOutput(output string) []WindowInfo {
	var windows []WindowInfo
	
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		
		// Parse screencapture output format
		// Example: "kCGWindowListOptionAll  22 [1920x1080] TextEdit"
		parts := strings.Fields(line)
		if len(parts) >= 4 {
			windowID := parts[1]
			windowName := strings.Join(parts[3:], " ")
			
			// Parse dimensions if available
			var width, height int
			if len(parts) >= 3 && strings.Contains(parts[2], "x") {
				dimensionStr := strings.Trim(parts[2], "[]")
				fmt.Sscanf(dimensionStr, "%dx%d", &width, &height)
			}
			
			window := WindowInfo{
				ID:          windowID,
				Name:        windowName,
				ProcessName: windowName, // Best guess
				Width:       width,
				Height:      height,
				IsVisible:   true,
				IsOnScreen:  true,
			}
			windows = append(windows, window)
		}
	}
	
	return windows
}

// listLinuxWindows lists windows on Linux using various methods
func (icm *ImageCaptureManager) listLinuxWindows() ([]WindowInfo, error) {
	var windows []WindowInfo
	
	// Method 1: Try wmctrl first (most comprehensive)
	if windows, err := icm.listLinuxWindowsWmctrl(); err == nil && len(windows) > 0 {
		return windows, nil
	}
	
	// Method 2: Try xwininfo
	if windows, err := icm.listLinuxWindowsXwininfo(); err == nil && len(windows) > 0 {
		return windows, nil
	}
	
	// Method 3: Try xdotool
	if windows, err := icm.listLinuxWindowsXdotool(); err == nil && len(windows) > 0 {
		return windows, nil
	}
	
	return windows, fmt.Errorf("no supported window listing method found")
}

// listLinuxWindowsWmctrl uses wmctrl to list windows on Linux
func (icm *ImageCaptureManager) listLinuxWindowsWmctrl() ([]WindowInfo, error) {
	var windows []WindowInfo
	
	cmd := exec.Command("wmctrl", "-l", "-G")
	output, err := cmd.Output()
	if err != nil {
		return windows, fmt.Errorf("wmctrl not available: %w", err)
	}
	
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		
		// Parse wmctrl output format
		// Example: "0x02000003  0 1920 1080 1920 1080 hostname Title"
		parts := strings.Fields(line)
		if len(parts) >= 7 {
			windowID := parts[0]
			var x, y, width, height int
			fmt.Sscanf(parts[2], "%d", &x)
			fmt.Sscanf(parts[3], "%d", &y)
			fmt.Sscanf(parts[4], "%d", &width)
			fmt.Sscanf(parts[5], "%d", &height)
			
			windowName := strings.Join(parts[7:], " ")
			
			window := WindowInfo{
				ID:         windowID,
				Name:       windowName,
				X:          x,
				Y:          y,
				Width:      width,
				Height:     height,
				IsVisible:  true,
				IsOnScreen: true,
			}
			windows = append(windows, window)
		}
	}
	
	return windows, nil
}

// listLinuxWindowsXwininfo uses xwininfo to list windows
func (icm *ImageCaptureManager) listLinuxWindowsXwininfo() ([]WindowInfo, error) {
	var windows []WindowInfo
	
	// Get window tree first
	cmd := exec.Command("xwininfo", "-tree", "-root")
	output, err := cmd.Output()
	if err != nil {
		return windows, fmt.Errorf("xwininfo not available: %w", err)
	}
	
	// Parse xwininfo output (simplified)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "0x") && strings.Contains(line, "\"") {
			// Parse window line
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				windowID := parts[0]
				
				// Extract window name from quotes
				start := strings.Index(line, "\"")
				end := strings.LastIndex(line, "\"")
				var windowName string
				if start >= 0 && end > start {
					windowName = line[start+1 : end]
				}
				
				window := WindowInfo{
					ID:         windowID,
					Name:       windowName,
					IsVisible:  true,
					IsOnScreen: true,
				}
				windows = append(windows, window)
			}
		}
	}
	
	return windows, nil
}

// listLinuxWindowsXdotool uses xdotool to list windows
func (icm *ImageCaptureManager) listLinuxWindowsXdotool() ([]WindowInfo, error) {
	var windows []WindowInfo
	
	cmd := exec.Command("xdotool", "search", "--name", ".*")
	output, err := cmd.Output()
	if err != nil {
		return windows, fmt.Errorf("xdotool not available: %w", err)
	}
	
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		
		windowID := line
		
		// Get window name
		nameCmd := exec.Command("xdotool", "getwindowname", windowID)
		nameOutput, err := nameCmd.Output()
		if err != nil {
			continue
		}
		
		windowName := strings.TrimSpace(string(nameOutput))
		
		window := WindowInfo{
			ID:         windowID,
			Name:       windowName,
			IsVisible:  true,
			IsOnScreen: true,
		}
		windows = append(windows, window)
	}
	
	return windows, nil
}

// GetWindowByName finds a window by name (case-insensitive substring match)
func (icm *ImageCaptureManager) GetWindowByName(name string) (*WindowInfo, error) {
	windows, err := icm.ListWindows()
	if err != nil {
		return nil, fmt.Errorf("failed to list windows: %w", err)
	}
	
	name = strings.ToLower(name)
	for _, window := range windows {
		if strings.Contains(strings.ToLower(window.Name), name) {
			return &window, nil
		}
		if strings.Contains(strings.ToLower(window.ProcessName), name) {
			return &window, nil
		}
	}
	
	return nil, fmt.Errorf("window not found: %s", name)
}

// GetWindowByProcess finds a window by process name
func (icm *ImageCaptureManager) GetWindowByProcess(processName string) (*WindowInfo, error) {
	windows, err := icm.ListWindows()
	if err != nil {
		return nil, fmt.Errorf("failed to list windows: %w", err)
	}
	
	processName = strings.ToLower(processName)
	for _, window := range windows {
		if strings.Contains(strings.ToLower(window.ProcessName), processName) {
			return &window, nil
		}
	}
	
	return nil, fmt.Errorf("window not found for process: %s", processName)
}

// SetCaptureWindow sets the window to capture by window ID
func (icm *ImageCaptureManager) SetCaptureWindow(windowID string) error {
	icm.mu.Lock()
	defer icm.mu.Unlock()
	
	icm.captureWindow = windowID
	log.Printf("[IMAGE_CAPTURE] Set capture window to: %s", windowID)
	return nil
}

// SetCaptureWindowByName sets the window to capture by window name
func (icm *ImageCaptureManager) SetCaptureWindowByName(name string) error {
	window, err := icm.GetWindowByName(name)
	if err != nil {
		return fmt.Errorf("failed to find window: %w", err)
	}
	
	return icm.SetCaptureWindow(window.ID)
}

// SetCaptureWindowByProcess sets the window to capture by process name
func (icm *ImageCaptureManager) SetCaptureWindowByProcess(processName string) error {
	window, err := icm.GetWindowByProcess(processName)
	if err != nil {
		return fmt.Errorf("failed to find window for process: %w", err)
	}
	
	return icm.SetCaptureWindow(window.ID)
}

// ClearCaptureWindow clears the window capture setting (capture full screen)
func (icm *ImageCaptureManager) ClearCaptureWindow() error {
	icm.mu.Lock()
	defer icm.mu.Unlock()
	
	icm.captureWindow = ""
	log.Printf("[IMAGE_CAPTURE] Cleared capture window (full screen)")
	return nil
}

// GetCurrentCaptureWindow returns the currently set capture window
func (icm *ImageCaptureManager) GetCurrentCaptureWindow() string {
	icm.mu.RLock()
	defer icm.mu.RUnlock()
	
	return icm.captureWindow
}