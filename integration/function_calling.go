package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
	tea "github.com/charmbracelet/bubbletea"
)

// FunctionCallingManager manages live function calling during streams
type FunctionCallingManager struct {
	// Configuration
	config           FunctionCallingConfig
	enabledFunctions map[string]bool
	
	// Execution state
	activeCallsCount int
	totalCallsCount  int64
	
	// Function registry
	functions        map[string]*FunctionHandler
	functionsMutex   sync.RWMutex
	
	// Execution channels
	callRequestChan  chan FunctionCallRequest
	callResultChan   chan FunctionCallResult
	
	// Context management
	streamContext    map[string]interface{}
	contextMutex     sync.RWMutex
	
	// Lifecycle
	ctx              context.Context
	cancel           context.CancelFunc
	
	// UI updates
	uiUpdateChan     chan tea.Msg
}

// FunctionCallingConfig holds configuration for function calling
type FunctionCallingConfig struct {
	Enabled              bool
	MaxConcurrentCalls   int
	CallTimeout          time.Duration
	EnableParallelCalls  bool
	EnableContextAware   bool
	MaxRetries           int
	RetryDelay           time.Duration
	EnableSecurityScan   bool
	AllowedFunctions     []string
	RestrictedFunctions  []string
}

// FunctionHandler represents a callable function
type FunctionHandler struct {
	Name         string
	Description  string
	Parameters   json.RawMessage
	Handler      func(context.Context, json.RawMessage) (json.RawMessage, error)
	Timeout      time.Duration
	RequireAuth  bool
	ContextAware bool
	Parallel     bool
}

// FunctionCallRequest represents a request to call a function
type FunctionCallRequest struct {
	ID           string
	FunctionName string
	Arguments    json.RawMessage
	Context      map[string]interface{}
	Timestamp    time.Time
	StreamID     string
	UserID       string
}

// FunctionCallResult represents the result of a function call
type FunctionCallResult struct {
	ID           string
	FunctionName string
	Success      bool
	Result       json.RawMessage
	Error        error
	Duration     time.Duration
	Timestamp    time.Time
	Context      map[string]interface{}
}

// StreamingContext holds context information for streaming
type StreamingContext struct {
	AudioTranscript string
	VideoObjects    []string
	ScreenContent   string
	UserQuery       string
	SessionID       string
	Timestamp       time.Time
}

// DefaultFunctionCallingConfig returns default configuration
func DefaultFunctionCallingConfig() FunctionCallingConfig {
	return FunctionCallingConfig{
		Enabled:              true,
		MaxConcurrentCalls:   5,
		CallTimeout:          30 * time.Second,
		EnableParallelCalls:  true,
		EnableContextAware:   true,
		MaxRetries:           3,
		RetryDelay:           1 * time.Second,
		EnableSecurityScan:   true,
		AllowedFunctions:     []string{},
		RestrictedFunctions:  []string{},
	}
}

// NewFunctionCallingManager creates a new function calling manager
func NewFunctionCallingManager(config FunctionCallingConfig, uiUpdateChan chan tea.Msg) *FunctionCallingManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	manager := &FunctionCallingManager{
		config:           config,
		enabledFunctions: make(map[string]bool),
		functions:        make(map[string]*FunctionHandler),
		callRequestChan:  make(chan FunctionCallRequest, 100),
		callResultChan:   make(chan FunctionCallResult, 100),
		streamContext:    make(map[string]interface{}),
		ctx:              ctx,
		cancel:           cancel,
		uiUpdateChan:     uiUpdateChan,
	}
	
	// Initialize with default functions
	manager.registerDefaultFunctions()
	
	// Start processing loops
	go manager.processFunctionCalls()
	go manager.processResults()
	
	return manager
}

// RegisterFunction registers a new function for calling
func (fcm *FunctionCallingManager) RegisterFunction(handler *FunctionHandler) error {
	if handler.Name == "" {
		return fmt.Errorf("function name is required")
	}
	
	fcm.functionsMutex.Lock()
	defer fcm.functionsMutex.Unlock()
	
	// Check if function is allowed
	if !fcm.isFunctionAllowed(handler.Name) {
		return fmt.Errorf("function %s is not allowed", handler.Name)
	}
	
	fcm.functions[handler.Name] = handler
	fcm.enabledFunctions[handler.Name] = true
	
	log.Printf("[FUNCTION_CALLING] Registered function: %s", handler.Name)
	return nil
}

// CallFunction initiates a function call
func (fcm *FunctionCallingManager) CallFunction(request FunctionCallRequest) error {
	if !fcm.config.Enabled {
		return fmt.Errorf("function calling is disabled")
	}
	
	// Check if function exists
	fcm.functionsMutex.RLock()
	_, exists := fcm.functions[request.FunctionName]
	fcm.functionsMutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("function %s not found", request.FunctionName)
	}
	
	// Check concurrent calls limit
	if fcm.activeCallsCount >= fcm.config.MaxConcurrentCalls {
		return fmt.Errorf("maximum concurrent calls reached")
	}
	
	// Add request ID if not provided
	if request.ID == "" {
		request.ID = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), fcm.totalCallsCount)
	}
	
	select {
	case fcm.callRequestChan <- request:
		fcm.activeCallsCount++
		fcm.totalCallsCount++
		return nil
	case <-fcm.ctx.Done():
		return fmt.Errorf("function calling manager stopped")
	}
}

// processFunctionCalls processes incoming function call requests
func (fcm *FunctionCallingManager) processFunctionCalls() {
	for {
		select {
		case <-fcm.ctx.Done():
			return
		case request := <-fcm.callRequestChan:
			if fcm.config.EnableParallelCalls {
				go fcm.executeFunctionCall(request)
			} else {
				fcm.executeFunctionCall(request)
			}
		}
	}
}

// executeFunctionCall executes a single function call
func (fcm *FunctionCallingManager) executeFunctionCall(request FunctionCallRequest) {
	defer func() {
		fcm.activeCallsCount--
	}()
	
	startTime := time.Now()
	
	// Get function handler
	fcm.functionsMutex.RLock()
	handler, exists := fcm.functions[request.FunctionName]
	fcm.functionsMutex.RUnlock()
	
	if !exists {
		result := FunctionCallResult{
			ID:           request.ID,
			FunctionName: request.FunctionName,
			Success:      false,
			Error:        fmt.Errorf("function not found"),
			Duration:     time.Since(startTime),
			Timestamp:    time.Now(),
		}
		fcm.sendResult(result)
		return
	}
	
	// Create execution context
	execCtx := request.Context
	if execCtx == nil {
		execCtx = make(map[string]interface{})
	}
	
	// Add streaming context if context-aware
	if handler.ContextAware {
		fcm.contextMutex.RLock()
		for k, v := range fcm.streamContext {
			execCtx[k] = v
		}
		fcm.contextMutex.RUnlock()
	}
	
	// Set timeout
	timeout := handler.Timeout
	if timeout == 0 {
		timeout = fcm.config.CallTimeout
	}
	
	callCtx, cancel := context.WithTimeout(fcm.ctx, timeout)
	defer cancel()
	
	// Execute with retries
	var result json.RawMessage
	var err error
	
	for attempt := 0; attempt <= fcm.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(fcm.config.RetryDelay):
				// Continue with retry
			case <-callCtx.Done():
				err = callCtx.Err()
				break
			}
		}
		
		// Execute function
		result, err = handler.Handler(callCtx, request.Arguments)
		if err == nil {
			break
		}
		
		log.Printf("[FUNCTION_CALLING] Attempt %d failed for %s: %v", attempt+1, request.FunctionName, err)
	}
	
	// Send result
	callResult := FunctionCallResult{
		ID:           request.ID,
		FunctionName: request.FunctionName,
		Success:      err == nil,
		Result:       result,
		Error:        err,
		Duration:     time.Since(startTime),
		Timestamp:    time.Now(),
		Context:      execCtx,
	}
	
	fcm.sendResult(callResult)
}

// processResults processes function call results
func (fcm *FunctionCallingManager) processResults() {
	for {
		select {
		case <-fcm.ctx.Done():
			return
		case result := <-fcm.callResultChan:
			// Log result
			if result.Success {
				log.Printf("[FUNCTION_CALLING] Function %s completed in %v", result.FunctionName, result.Duration)
			} else {
				log.Printf("[FUNCTION_CALLING] Function %s failed: %v", result.FunctionName, result.Error)
			}
			
			// Send UI update
			if fcm.uiUpdateChan != nil {
				fcm.uiUpdateChan <- FunctionCallResultMsg{Result: result}
			}
		}
	}
}

// sendResult sends a function call result
func (fcm *FunctionCallingManager) sendResult(result FunctionCallResult) {
	select {
	case fcm.callResultChan <- result:
		// Success
	default:
		log.Printf("[FUNCTION_CALLING] Result channel full, dropping result for %s", result.FunctionName)
	}
}

// UpdateStreamContext updates the streaming context
func (fcm *FunctionCallingManager) UpdateStreamContext(context map[string]interface{}) {
	fcm.contextMutex.Lock()
	defer fcm.contextMutex.Unlock()
	
	for k, v := range context {
		fcm.streamContext[k] = v
	}
}

// GetStreamContext returns the current streaming context
func (fcm *FunctionCallingManager) GetStreamContext() map[string]interface{} {
	fcm.contextMutex.RLock()
	defer fcm.contextMutex.RUnlock()
	
	context := make(map[string]interface{})
	for k, v := range fcm.streamContext {
		context[k] = v
	}
	
	return context
}

// isFunctionAllowed checks if a function is allowed to be called
func (fcm *FunctionCallingManager) isFunctionAllowed(functionName string) bool {
	// Check restricted functions
	for _, restricted := range fcm.config.RestrictedFunctions {
		if restricted == functionName {
			return false
		}
	}
	
	// Check allowed functions (if specified)
	if len(fcm.config.AllowedFunctions) > 0 {
		for _, allowed := range fcm.config.AllowedFunctions {
			if allowed == functionName {
				return true
			}
		}
		return false
	}
	
	return true
}

// registerDefaultFunctions registers default functions
func (fcm *FunctionCallingManager) registerDefaultFunctions() {
	// Time function
	fcm.RegisterFunction(&FunctionHandler{
		Name:        "get_current_time",
		Description: "Get the current time",
		Handler: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			result := map[string]interface{}{
				"time":      time.Now().Format(time.RFC3339),
				"unix":      time.Now().Unix(),
				"timezone":  time.Now().Location().String(),
			}
			return json.Marshal(result)
		},
		Timeout: 5 * time.Second,
	})
	
	// Screen analysis function
	fcm.RegisterFunction(&FunctionHandler{
		Name:         "analyze_screen_content",
		Description:  "Analyze current screen content",
		ContextAware: true,
		Handler: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			// This would integrate with screen capture
			result := map[string]interface{}{
				"analysis": "Screen content analysis would go here",
				"objects":  []string{"window", "text", "image"},
			}
			return json.Marshal(result)
		},
		Timeout: 10 * time.Second,
	})
	
	// Audio analysis function
	fcm.RegisterFunction(&FunctionHandler{
		Name:         "analyze_audio_transcript",
		Description:  "Analyze audio transcript for context",
		ContextAware: true,
		Handler: func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			// This would integrate with audio processing
			result := map[string]interface{}{
				"sentiment": "positive",
				"keywords":  []string{"help", "question", "request"},
			}
			return json.Marshal(result)
		},
		Timeout: 5 * time.Second,
	})
}

// GetStatistics returns function calling statistics
func (fcm *FunctionCallingManager) GetStatistics() map[string]interface{} {
	fcm.functionsMutex.RLock()
	defer fcm.functionsMutex.RUnlock()
	
	return map[string]interface{}{
		"active_calls":      fcm.activeCallsCount,
		"total_calls":       fcm.totalCallsCount,
		"registered_functions": len(fcm.functions),
		"enabled_functions":    len(fcm.enabledFunctions),
	}
}

// Stop gracefully stops the function calling manager
func (fcm *FunctionCallingManager) Stop() {
	fcm.cancel()
	close(fcm.callRequestChan)
	close(fcm.callResultChan)
	
	log.Printf("[FUNCTION_CALLING] Manager stopped. Total calls: %d", fcm.totalCallsCount)
}

// UI Messages
type FunctionCallResultMsg struct {
	Result FunctionCallResult
}

type FunctionCallStartedMsg struct {
	Request FunctionCallRequest
}

type FunctionCallErrorMsg struct {
	Error error
}