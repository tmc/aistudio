package audio

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"
)

// ProactiveAudioManager handles intelligent audio response triggering
type ProactiveAudioManager struct {
	// Configuration
	enabled             bool
	relevanceThreshold  float32
	deviceKeywords      []string
	contextWindow       time.Duration
	
	// State tracking
	isListening         bool
	lastRelevantTime    time.Time
	conversationContext []ContextEntry
	
	// Audio analysis
	audioBuffer         []AudioSegment
	bufferDuration      time.Duration
	
	// Synchronization
	mu                  sync.RWMutex
	
	// Channels
	inputChan           chan AudioInput
	relevanceChan       chan RelevanceResult
	
	// Context management
	ctx                 context.Context
	cancel              context.CancelFunc
}

// AudioInput represents incoming audio with metadata
type AudioInput struct {
	Data        []byte
	Timestamp   time.Time
	Transcript  string
	Energy      float32
	IsSpeech    bool
}

// AudioSegment represents a segment of audio with analysis
type AudioSegment struct {
	Input          AudioInput
	IsRelevant     bool
	RelevanceScore float32
	Keywords       []string
	Intent         string
}

// ContextEntry represents a piece of conversation context
type ContextEntry struct {
	Timestamp   time.Time
	Type        string // "user", "assistant", "system"
	Content     string
	IsRelevant  bool
}

// RelevanceResult represents the result of relevance analysis
type RelevanceResult struct {
	IsRelevant     bool
	Score          float32
	Reason         string
	ShouldRespond  bool
	ResponseType   string // "immediate", "deferred", "none"
}

// ProactiveConfig holds configuration for proactive audio
type ProactiveConfig struct {
	Enabled            bool
	RelevanceThreshold float32
	DeviceKeywords     []string
	ContextWindow      time.Duration
	BufferDuration     time.Duration
}

// DefaultProactiveConfig returns default configuration
func DefaultProactiveConfig() ProactiveConfig {
	return ProactiveConfig{
		Enabled:            true,
		RelevanceThreshold: 0.7,
		DeviceKeywords:     []string{"hey", "assistant", "help", "please", "can you", "could you", "would you"},
		ContextWindow:      30 * time.Second,
		BufferDuration:     5 * time.Second,
	}
}

// NewProactiveAudioManager creates a new proactive audio manager
func NewProactiveAudioManager(config ProactiveConfig) *ProactiveAudioManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	manager := &ProactiveAudioManager{
		enabled:            config.Enabled,
		relevanceThreshold: config.RelevanceThreshold,
		deviceKeywords:     config.DeviceKeywords,
		contextWindow:      config.ContextWindow,
		bufferDuration:     config.BufferDuration,
		audioBuffer:        make([]AudioSegment, 0),
		conversationContext: make([]ContextEntry, 0),
		inputChan:          make(chan AudioInput, 100),
		relevanceChan:      make(chan RelevanceResult, 10),
		ctx:                ctx,
		cancel:             cancel,
	}
	
	// Start processing loops
	go manager.processAudioInput()
	go manager.cleanupOldContext()
	
	return manager
}

// ProcessAudioInput analyzes incoming audio for relevance
func (pam *ProactiveAudioManager) ProcessAudioInput(input AudioInput) error {
	if !pam.enabled {
		// If proactive audio is disabled, always consider relevant
		pam.relevanceChan <- RelevanceResult{
			IsRelevant:    true,
			Score:         1.0,
			ShouldRespond: true,
			ResponseType:  "immediate",
		}
		return nil
	}
	
	select {
	case pam.inputChan <- input:
		return nil
	case <-pam.ctx.Done():
		return fmt.Errorf("proactive audio manager stopped")
	}
}

// GetRelevanceResult returns the next relevance analysis result
func (pam *ProactiveAudioManager) GetRelevanceResult() (RelevanceResult, error) {
	select {
	case result := <-pam.relevanceChan:
		return result, nil
	case <-pam.ctx.Done():
		return RelevanceResult{}, fmt.Errorf("proactive audio manager stopped")
	}
}

// processAudioInput runs the main audio analysis loop
func (pam *ProactiveAudioManager) processAudioInput() {
	for {
		select {
		case <-pam.ctx.Done():
			return
		case input := <-pam.inputChan:
			result := pam.analyzeRelevance(input)
			
			// Store in buffer
			pam.mu.Lock()
			segment := AudioSegment{
				Input:          input,
				IsRelevant:     result.IsRelevant,
				RelevanceScore: result.Score,
			}
			pam.audioBuffer = append(pam.audioBuffer, segment)
			
			// Trim old segments
			pam.trimAudioBuffer()
			
			// Update context if relevant
			if result.IsRelevant {
				pam.lastRelevantTime = time.Now()
				pam.addToContext("user", input.Transcript, true)
			}
			pam.mu.Unlock()
			
			// Send result
			select {
			case pam.relevanceChan <- result:
				// Success
			default:
				log.Printf("[PROACTIVE_AUDIO] Relevance channel full, dropping result")
			}
		}
	}
}

// analyzeRelevance determines if audio input is relevant
func (pam *ProactiveAudioManager) analyzeRelevance(input AudioInput) RelevanceResult {
	// If no speech detected, not relevant
	if !input.IsSpeech {
		return RelevanceResult{
			IsRelevant:    false,
			Score:         0.0,
			Reason:        "No speech detected",
			ShouldRespond: false,
			ResponseType:  "none",
		}
	}
	
	// If no transcript, can't analyze
	if input.Transcript == "" {
		return RelevanceResult{
			IsRelevant:    false,
			Score:         0.1,
			Reason:        "No transcript available",
			ShouldRespond: false,
			ResponseType:  "none",
		}
	}
	
	// Calculate relevance score
	score := pam.calculateRelevanceScore(input)
	
	// Check if within conversation context window
	pam.mu.RLock()
	inContextWindow := time.Since(pam.lastRelevantTime) < pam.contextWindow
	pam.mu.RUnlock()
	
	// Determine response type
	var responseType string
	shouldRespond := false
	
	if score >= pam.relevanceThreshold {
		shouldRespond = true
		responseType = "immediate"
	} else if score >= pam.relevanceThreshold*0.7 && inContextWindow {
		// Lower threshold if in active conversation
		shouldRespond = true
		responseType = "immediate"
	} else if score >= pam.relevanceThreshold*0.5 {
		// Might be relevant, defer decision
		shouldRespond = false
		responseType = "deferred"
	} else {
		shouldRespond = false
		responseType = "none"
	}
	
	reason := pam.getRelevanceReason(score, input)
	
	return RelevanceResult{
		IsRelevant:    score >= pam.relevanceThreshold,
		Score:         score,
		Reason:        reason,
		ShouldRespond: shouldRespond,
		ResponseType:  responseType,
	}
}

// calculateRelevanceScore computes a relevance score for the input
func (pam *ProactiveAudioManager) calculateRelevanceScore(input AudioInput) float32 {
	score := float32(0.0)
	transcript := strings.ToLower(input.Transcript)
	
	// Check for device keywords
	keywordScore := float32(0.0)
	keywordCount := 0
	for _, keyword := range pam.deviceKeywords {
		if strings.Contains(transcript, keyword) {
			keywordCount++
			keywordScore += 0.3
		}
	}
	if keywordCount > 0 {
		score += math.Min(float64(keywordScore), 0.6)
	}
	
	// Check for question patterns
	if pam.isQuestion(transcript) {
		score += 0.2
	}
	
	// Check for command patterns
	if pam.isCommand(transcript) {
		score += 0.2
	}
	
	// Check energy level (louder = more likely directed at device)
	if input.Energy > 0.7 {
		score += 0.1
	}
	
	// Check context continuity
	pam.mu.RLock()
	if len(pam.conversationContext) > 0 && time.Since(pam.lastRelevantTime) < 10*time.Second {
		score += 0.2
	}
	pam.mu.RUnlock()
	
	// Clamp score between 0 and 1
	return float32(math.Min(math.Max(float64(score), 0.0), 1.0))
}

// isQuestion checks if the transcript appears to be a question
func (pam *ProactiveAudioManager) isQuestion(transcript string) bool {
	questionWords := []string{"what", "when", "where", "who", "why", "how", "can", "could", "would", "should", "is", "are", "do", "does"}
	questionMarks := strings.Contains(transcript, "?")
	
	if questionMarks {
		return true
	}
	
	for _, word := range questionWords {
		if strings.HasPrefix(transcript, word+" ") {
			return true
		}
	}
	
	return false
}

// isCommand checks if the transcript appears to be a command
func (pam *ProactiveAudioManager) isCommand(transcript string) bool {
	commandWords := []string{"show", "tell", "explain", "help", "start", "stop", "open", "close", "play", "pause", "find", "search"}
	
	for _, word := range commandWords {
		if strings.Contains(transcript, word) {
			return true
		}
	}
	
	return false
}

// getRelevanceReason provides a human-readable reason for the relevance score
func (pam *ProactiveAudioManager) getRelevanceReason(score float32, input AudioInput) string {
	reasons := []string{}
	
	transcript := strings.ToLower(input.Transcript)
	
	// Check what contributed to the score
	for _, keyword := range pam.deviceKeywords {
		if strings.Contains(transcript, keyword) {
			reasons = append(reasons, fmt.Sprintf("contains keyword '%s'", keyword))
			break
		}
	}
	
	if pam.isQuestion(transcript) {
		reasons = append(reasons, "appears to be a question")
	}
	
	if pam.isCommand(transcript) {
		reasons = append(reasons, "appears to be a command")
	}
	
	if len(reasons) == 0 {
		if score < 0.3 {
			return "No device-directed indicators found"
		}
		return "Weak relevance signals"
	}
	
	return strings.Join(reasons, "; ")
}

// addToContext adds an entry to the conversation context
func (pam *ProactiveAudioManager) addToContext(entryType, content string, isRelevant bool) {
	entry := ContextEntry{
		Timestamp:  time.Now(),
		Type:       entryType,
		Content:    content,
		IsRelevant: isRelevant,
	}
	
	pam.conversationContext = append(pam.conversationContext, entry)
	
	// Keep only recent context
	maxEntries := 50
	if len(pam.conversationContext) > maxEntries {
		pam.conversationContext = pam.conversationContext[len(pam.conversationContext)-maxEntries:]
	}
}

// trimAudioBuffer removes old audio segments
func (pam *ProactiveAudioManager) trimAudioBuffer() {
	cutoff := time.Now().Add(-pam.bufferDuration)
	
	newBuffer := []AudioSegment{}
	for _, segment := range pam.audioBuffer {
		if segment.Input.Timestamp.After(cutoff) {
			newBuffer = append(newBuffer, segment)
		}
	}
	
	pam.audioBuffer = newBuffer
}

// cleanupOldContext periodically removes old context entries
func (pam *ProactiveAudioManager) cleanupOldContext() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-pam.ctx.Done():
			return
		case <-ticker.C:
			pam.mu.Lock()
			cutoff := time.Now().Add(-pam.contextWindow)
			
			newContext := []ContextEntry{}
			for _, entry := range pam.conversationContext {
				if entry.Timestamp.After(cutoff) {
					newContext = append(newContext, entry)
				}
			}
			
			pam.conversationContext = newContext
			pam.mu.Unlock()
		}
	}
}

// GetRecentContext returns recent conversation context
func (pam *ProactiveAudioManager) GetRecentContext(duration time.Duration) []ContextEntry {
	pam.mu.RLock()
	defer pam.mu.RUnlock()
	
	cutoff := time.Now().Add(-duration)
	recent := []ContextEntry{}
	
	for _, entry := range pam.conversationContext {
		if entry.Timestamp.After(cutoff) {
			recent = append(recent, entry)
		}
	}
	
	return recent
}

// SetEnabled enables or disables proactive audio
func (pam *ProactiveAudioManager) SetEnabled(enabled bool) {
	pam.mu.Lock()
	defer pam.mu.Unlock()
	
	pam.enabled = enabled
	log.Printf("[PROACTIVE_AUDIO] Enabled: %v", enabled)
}

// UpdateKeywords updates the device keywords
func (pam *ProactiveAudioManager) UpdateKeywords(keywords []string) {
	pam.mu.Lock()
	defer pam.mu.Unlock()
	
	pam.deviceKeywords = keywords
	log.Printf("[PROACTIVE_AUDIO] Updated keywords: %v", keywords)
}

// SetRelevanceThreshold updates the relevance threshold
func (pam *ProactiveAudioManager) SetRelevanceThreshold(threshold float32) {
	pam.mu.Lock()
	defer pam.mu.Unlock()
	
	pam.relevanceThreshold = threshold
	log.Printf("[PROACTIVE_AUDIO] Relevance threshold: %.2f", threshold)
}

// Stop gracefully stops the proactive audio manager
func (pam *ProactiveAudioManager) Stop() {
	pam.cancel()
	close(pam.inputChan)
	close(pam.relevanceChan)
	log.Printf("[PROACTIVE_AUDIO] Manager stopped")
}