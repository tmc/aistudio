package audio

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// VADManager manages advanced Voice Activity Detection
type VADManager struct {
	// Configuration
	config           VADConfig
	customVAD        CustomVAD
	
	// State tracking
	isActive         bool
	currentState     VADState
	lastTransition   time.Time
	
	// Audio analysis
	energyBuffer     []float32
	zeroCrossBuffer  []int
	spectralBuffer   [][]float32
	
	// Statistics
	speechFrames     int64
	silenceFrames    int64
	totalFrames      int64
	
	// Synchronization
	mu               sync.RWMutex
	
	// Processing
	inputChan        chan AudioFrame
	outputChan       chan VADResult
	
	// Context
	ctx              context.Context
	cancel           context.CancelFunc
}

// VADConfig holds Voice Activity Detection configuration
type VADConfig struct {
	// Basic thresholds
	EnergyThreshold      float32
	ZeroCrossThreshold   int
	SpectralThreshold    float32
	
	// Advanced settings
	FrameSize            int           // Samples per frame
	SampleRate           int           // Sample rate in Hz
	MinSpeechDuration    time.Duration // Minimum duration to consider as speech
	MinSilenceDuration   time.Duration // Minimum duration to consider as silence
	HangoverTime         time.Duration // Time to wait before transitioning to silence
	
	// Feature flags
	UseEnergyDetection   bool
	UseZeroCrossing      bool
	UseSpectralAnalysis  bool
	UseMLModel           bool
	// TODO: Add MLModelPath field for loading custom VAD models
	// TODO: Add MLModelType field (tensorflow, onnx, pytorch)
	// TODO: Add MLConfidenceThreshold for ML-based decisions
	
	// Smoothing
	SmoothingWindow      int
	AdaptiveThreshold    bool
	NoiseFloor           float32
}

// CustomVAD interface for pluggable VAD implementations
// TODO: Implement WebRTC VAD wrapper as a CustomVAD
// TODO: Implement DNN-based VAD using TensorFlow Lite
// TODO: Implement Silero VAD integration
// TODO: Add support for cloud-based VAD services
type CustomVAD interface {
	ProcessFrame(frame AudioFrame) (isSpeech bool, confidence float32)
	UpdateNoiseProfile(frame AudioFrame)
	Reset()
}

// VADState represents the current state of voice detection
type VADState int

const (
	VADSilence VADState = iota
	VADSpeechOnset
	VADSpeech
	VADSpeechOffset
)

// AudioFrame represents a frame of audio for VAD processing
type AudioFrame struct {
	Data       []float32
	Timestamp  time.Time
	FrameIndex int64
}

// VADResult represents the result of VAD analysis
type VADResult struct {
	Frame           AudioFrame
	IsSpeech        bool
	Confidence      float32
	State           VADState
	Energy          float32
	ZeroCrossings   int
	SpectralFlux    float32
	Features        map[string]float32
}

// DefaultVADConfig returns default VAD configuration
func DefaultVADConfig() VADConfig {
	return VADConfig{
		EnergyThreshold:      0.01,
		ZeroCrossThreshold:   25,
		SpectralThreshold:    0.3,
		FrameSize:            512,
		SampleRate:           16000,
		MinSpeechDuration:    200 * time.Millisecond,
		MinSilenceDuration:   300 * time.Millisecond,
		HangoverTime:         150 * time.Millisecond,
		UseEnergyDetection:   true,
		UseZeroCrossing:      true,
		UseSpectralAnalysis:  true,
		UseMLModel:           false,
		SmoothingWindow:      5,
		AdaptiveThreshold:    true,
		NoiseFloor:           0.001,
	}
}

// NewVADManager creates a new Voice Activity Detection manager
func NewVADManager(config VADConfig) *VADManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	bufferSize := config.SmoothingWindow
	
	manager := &VADManager{
		config:          config,
		currentState:    VADSilence,
		energyBuffer:    make([]float32, 0, bufferSize),
		zeroCrossBuffer: make([]int, 0, bufferSize),
		spectralBuffer:  make([][]float32, 0, bufferSize),
		inputChan:       make(chan AudioFrame, 100),
		outputChan:      make(chan VADResult, 100),
		ctx:             ctx,
		cancel:          cancel,
	}
	
	// Start processing loop
	go manager.processFrames()
	
	return manager
}

// SetCustomVAD sets a custom VAD implementation
func (vm *VADManager) SetCustomVAD(customVAD CustomVAD) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	vm.customVAD = customVAD
	log.Printf("[VAD] Custom VAD implementation set")
}

// ProcessFrame processes an audio frame for voice activity
func (vm *VADManager) ProcessFrame(frame AudioFrame) error {
	select {
	case vm.inputChan <- frame:
		return nil
	case <-vm.ctx.Done():
		return fmt.Errorf("VAD manager stopped")
	}
}

// GetResult retrieves the next VAD result
func (vm *VADManager) GetResult() (VADResult, error) {
	select {
	case result := <-vm.outputChan:
		return result, nil
	case <-vm.ctx.Done():
		return VADResult{}, fmt.Errorf("VAD manager stopped")
	}
}

// processFrames runs the main VAD processing loop
func (vm *VADManager) processFrames() {
	for {
		select {
		case <-vm.ctx.Done():
			return
		case frame := <-vm.inputChan:
			result := vm.analyzeFrame(frame)
			
			// Update statistics
			vm.mu.Lock()
			vm.totalFrames++
			if result.IsSpeech {
				vm.speechFrames++
			} else {
				vm.silenceFrames++
			}
			vm.mu.Unlock()
			
			// Send result
			select {
			case vm.outputChan <- result:
				// Success
			default:
				log.Printf("[VAD] Output buffer full, dropping result")
			}
		}
	}
}

// analyzeFrame performs VAD analysis on a single frame
func (vm *VADManager) analyzeFrame(frame AudioFrame) VADResult {
	result := VADResult{
		Frame:    frame,
		Features: make(map[string]float32),
	}
	
	// If custom VAD is available, use it
	if vm.customVAD != nil {
		isSpeech, confidence := vm.customVAD.ProcessFrame(frame)
		result.IsSpeech = isSpeech
		result.Confidence = confidence
		result.State = vm.updateState(isSpeech)
		return result
	}
	
	// Calculate features
	if vm.config.UseEnergyDetection {
		result.Energy = vm.calculateEnergy(frame.Data)
		result.Features["energy"] = result.Energy
	}
	
	if vm.config.UseZeroCrossing {
		result.ZeroCrossings = vm.calculateZeroCrossings(frame.Data)
		result.Features["zero_crossings"] = float32(result.ZeroCrossings)
	}
	
	if vm.config.UseSpectralAnalysis {
		result.SpectralFlux = vm.calculateSpectralFlux(frame.Data)
		result.Features["spectral_flux"] = result.SpectralFlux
	}
	
	// Update buffers for smoothing
	vm.updateBuffers(result)
	
	// Make decision
	isSpeech, confidence := vm.makeDecision(result)
	result.IsSpeech = isSpeech
	result.Confidence = confidence
	result.State = vm.updateState(isSpeech)
	
	return result
}

// calculateEnergy computes the energy of an audio frame
func (vm *VADManager) calculateEnergy(data []float32) float32 {
	if len(data) == 0 {
		return 0
	}
	
	energy := float32(0)
	for _, sample := range data {
		energy += sample * sample
	}
	
	return energy / float32(len(data))
}

// calculateZeroCrossings counts zero crossings in the frame
func (vm *VADManager) calculateZeroCrossings(data []float32) int {
	if len(data) < 2 {
		return 0
	}
	
	crossings := 0
	for i := 1; i < len(data); i++ {
		if (data[i-1] >= 0 && data[i] < 0) || (data[i-1] < 0 && data[i] >= 0) {
			crossings++
		}
	}
	
	return crossings
}

// calculateSpectralFlux computes spectral flux (simplified)
func (vm *VADManager) calculateSpectralFlux(data []float32) float32 {
	// Simplified spectral flux calculation
	// In a real implementation, this would use FFT
	// TODO: Implement proper FFT-based spectral analysis
	// TODO: Add windowing functions (Hamming, Hann, Blackman)
	// TODO: Calculate magnitude spectrum from FFT
	// TODO: Implement proper spectral flux calculation
	// TODO: Add spectral centroid and spread calculations
	// TODO: Implement mel-frequency cepstral coefficients (MFCC)
	
	flux := float32(0)
	for i := 1; i < len(data); i++ {
		diff := data[i] - data[i-1]
		if diff > 0 {
			flux += diff
		}
	}
	
	return flux / float32(len(data))
}

// updateBuffers updates the smoothing buffers
func (vm *VADManager) updateBuffers(result VADResult) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	// Update energy buffer
	vm.energyBuffer = append(vm.energyBuffer, result.Energy)
	if len(vm.energyBuffer) > vm.config.SmoothingWindow {
		vm.energyBuffer = vm.energyBuffer[1:]
	}
	
	// Update zero crossing buffer
	vm.zeroCrossBuffer = append(vm.zeroCrossBuffer, result.ZeroCrossings)
	if len(vm.zeroCrossBuffer) > vm.config.SmoothingWindow {
		vm.zeroCrossBuffer = vm.zeroCrossBuffer[1:]
	}
}

// makeDecision determines if the frame contains speech
func (vm *VADManager) makeDecision(result VADResult) (bool, float32) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	scores := []float32{}
	
	// Energy-based decision
	if vm.config.UseEnergyDetection && len(vm.energyBuffer) > 0 {
		avgEnergy := vm.average(vm.energyBuffer)
		threshold := vm.config.EnergyThreshold
		
		if vm.config.AdaptiveThreshold {
			threshold = vm.adaptThreshold(avgEnergy, vm.config.EnergyThreshold)
		}
		
		if avgEnergy > threshold {
			scores = append(scores, 1.0)
		} else {
			scores = append(scores, avgEnergy/threshold)
		}
	}
	
	// Zero crossing-based decision
	if vm.config.UseZeroCrossing && len(vm.zeroCrossBuffer) > 0 {
		avgCrossings := vm.averageInt(vm.zeroCrossBuffer)
		if avgCrossings > vm.config.ZeroCrossThreshold {
			scores = append(scores, 0.3) // Lower weight for zero crossings
		} else {
			scores = append(scores, float32(avgCrossings)/float32(vm.config.ZeroCrossThreshold)*0.3)
		}
	}
	
	// Spectral-based decision
	if vm.config.UseSpectralAnalysis && result.SpectralFlux > vm.config.SpectralThreshold {
		scores = append(scores, 1.0)
	} else if vm.config.UseSpectralAnalysis {
		scores = append(scores, result.SpectralFlux/vm.config.SpectralThreshold)
	}
	
	// Combine scores
	if len(scores) == 0 {
		return false, 0.0
	}
	
	confidence := vm.average(scores)
	isSpeech := confidence > 0.5
	
	return isSpeech, confidence
}

// updateState updates the VAD state machine
func (vm *VADManager) updateState(isSpeech bool) VADState {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	now := time.Now()
	timeSinceTransition := now.Sub(vm.lastTransition)
	
	newState := vm.currentState
	
	switch vm.currentState {
	case VADSilence:
		if isSpeech {
			newState = VADSpeechOnset
			vm.lastTransition = now
		}
		
	case VADSpeechOnset:
		if isSpeech && timeSinceTransition >= vm.config.MinSpeechDuration {
			newState = VADSpeech
		} else if !isSpeech {
			newState = VADSilence
			vm.lastTransition = now
		}
		
	case VADSpeech:
		if !isSpeech {
			newState = VADSpeechOffset
			vm.lastTransition = now
		}
		
	case VADSpeechOffset:
		if isSpeech {
			newState = VADSpeech
			vm.lastTransition = now
		} else if timeSinceTransition >= vm.config.HangoverTime {
			newState = VADSilence
		}
	}
	
	vm.currentState = newState
	return newState
}

// average calculates the average of a float32 slice
func (vm *VADManager) average(values []float32) float32 {
	if len(values) == 0 {
		return 0
	}
	
	sum := float32(0)
	for _, v := range values {
		sum += v
	}
	
	return sum / float32(len(values))
}

// averageInt calculates the average of an int slice
func (vm *VADManager) averageInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	
	sum := 0
	for _, v := range values {
		sum += v
	}
	
	return sum / len(values)
}

// adaptThreshold adapts the threshold based on noise level
func (vm *VADManager) adaptThreshold(currentLevel, baseThreshold float32) float32 {
	// Simple adaptive threshold
	// TODO: Implement proper noise estimation algorithm
	// TODO: Add long-term and short-term noise tracking
	// TODO: Implement minimum statistics noise estimation
	// TODO: Add spectral subtraction for noise reduction
	// TODO: Implement Wiener filtering for adaptive threshold
	// TODO: Add support for multi-band adaptive thresholds
	noiseLevel := vm.config.NoiseFloor
	adaptedThreshold := baseThreshold + (currentLevel * 0.1)
	
	return float32(math.Max(float64(adaptedThreshold), float64(noiseLevel*2)))
}

// GetState returns the current VAD state
func (vm *VADManager) GetState() VADState {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.currentState
}

// GetStatistics returns VAD statistics
func (vm *VADManager) GetStatistics() (speechFrames, silenceFrames, totalFrames int64) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.speechFrames, vm.silenceFrames, vm.totalFrames
}

// Reset resets the VAD state
func (vm *VADManager) Reset() {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	vm.currentState = VADSilence
	vm.lastTransition = time.Now()
	vm.energyBuffer = vm.energyBuffer[:0]
	vm.zeroCrossBuffer = vm.zeroCrossBuffer[:0]
	vm.spectralBuffer = vm.spectralBuffer[:0]
	
	if vm.customVAD != nil {
		vm.customVAD.Reset()
	}
	
	log.Printf("[VAD] State reset")
}

// Stop gracefully stops the VAD manager
func (vm *VADManager) Stop() {
	vm.cancel()
	close(vm.inputChan)
	close(vm.outputChan)
	
	vm.mu.RLock()
	speechRatio := float64(vm.speechFrames) / float64(vm.totalFrames) * 100
	vm.mu.RUnlock()
	
	log.Printf("[VAD] Manager stopped. Speech ratio: %.2f%%", speechRatio)
}