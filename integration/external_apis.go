package integration

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
	"github.com/tmc/aistudio/api"
)

// ExternalAPIManager manages external API integrations during live streaming
type ExternalAPIManager struct {
	// Core components
	uiUpdateChan       chan tea.Msg
	httpClient         *http.Client
	
	// Context and state
	ctx                context.Context
	cancel             context.CancelFunc
	isActive           bool
	
	// Configuration
	config             ExternalAPIConfig
	
	// API management
	registeredAPIs     map[string]*APIEndpoint
	activeRequests     map[string]*APIRequest
	requestQueue       chan *APIRequest
	responseQueue      chan *APIResponse
	
	// Rate limiting
	rateLimiters       map[string]*RateLimiter
	globalRateLimit    *RateLimiter
	
	// WebSocket connections
	wsConnections      map[string]*WebSocketConnection
	wsUpgrader         websocket.Upgrader
	
	// Authentication
	authProviders      map[string]AuthProvider
	tokenCache         map[string]*AuthToken
	
	// Synchronization
	mu                 sync.RWMutex
	requestCounter     int64
	
	// Performance metrics
	totalRequests      int64
	successfulRequests int64
	failedRequests     int64
	averageResponseTime time.Duration
	
	// Response processing
	responseProcessors map[string]ResponseProcessor
	middlewares        []APIMiddleware
}

// ExternalAPIConfig contains configuration for external API integration
type ExternalAPIConfig struct {
	// HTTP client settings
	RequestTimeout       time.Duration
	MaxConcurrentRequests int
	MaxRetries           int
	RetryDelay           time.Duration
	
	// Rate limiting
	GlobalRateLimit      int     // requests per second
	DefaultAPIRateLimit  int     // per API rate limit
	BurstSize            int     // burst capacity
	
	// Security settings
	AllowHTTP            bool
	VerifySSL            bool
	MaxResponseSize      int64
	AllowedDomains       []string
	BlockedDomains       []string
	
	// WebSocket settings
	WSReadBufferSize     int
	WSWriteBufferSize    int
	WSHandshakeTimeout   time.Duration
	WSPingInterval       time.Duration
	WSPongTimeout        time.Duration
	
	// Authentication
	DefaultAuthType      string
	AuthCacheTTL         time.Duration
	
	// Response processing
	StreamResponses      bool
	StreamingInterval    time.Duration
	MaxStreamingSize     int64
	
	// Proxy settings
	ProxyURL             string
	ProxyAuth            *ProxyAuth
	
	// Circuit breaker
	CircuitBreakerEnabled bool
	FailureThreshold      int
	RecoveryTimeout       time.Duration
}

// APIEndpoint represents a registered external API endpoint
type APIEndpoint struct {
	ID                   string
	Name                 string
	BaseURL              string
	AuthType             string
	AuthConfig           map[string]interface{}
	RateLimit            int
	Headers              map[string]string
	Timeout              time.Duration
	RetryPolicy          *RetryPolicy
	CircuitBreaker       *CircuitBreaker
	ResponseProcessor    ResponseProcessor
	Middleware           []APIMiddleware
	
	// WebSocket specific
	IsWebSocket          bool
	WSProtocols          []string
	WSOrigin             string
	
	// Documentation
	Description          string
	Documentation        string
	Examples             []APIExample
}

// APIRequest represents an API request
type APIRequest struct {
	ID                   string
	EndpointID           string
	Method               string
	URL                  string
	Headers              map[string]string
	Body                 []byte
	QueryParams          map[string]string
	Context              *ExecutionContext
	StartTime            time.Time
	Timeout              time.Duration
	RetryCount           int
	
	// Response handling
	ResponseChan         chan *APIResponse
	StreamingChan        chan *APIStreamingData
	
	// Authentication
	AuthToken            *AuthToken
	
	// Request metadata
	UserAgent            string
	ContentType          string
	Accept               string
	
	// WebSocket specific
	IsWebSocket          bool
	WSSubprotocols       []string
	WSOrigin             string
}

// APIResponse represents an API response
type APIResponse struct {
	RequestID            string
	StatusCode           int
	Headers              map[string]string
	Body                 []byte
	ContentType          string
	ResponseTime         time.Duration
	Success              bool
	Error                error
	
	// Streaming data
	IsStreaming          bool
	StreamingComplete    bool
	StreamingProgress    float64
	
	// Parsed data
	JSONData             map[string]interface{}
	XMLData              interface{}
	TextData             string
	
	// Response metadata
	ContentLength        int64
	Timestamp            time.Time
	CacheHit             bool
	RetryCount           int
	
	// Rate limiting info
	RateLimitRemaining   int
	RateLimitReset       time.Time
}

// APIStreamingData represents streaming response data
type APIStreamingData struct {
	RequestID            string
	Data                 []byte
	IsComplete           bool
	Progress             float64
	Timestamp            time.Time
	Metadata             map[string]interface{}
}

// WebSocketConnection represents a WebSocket connection
type WebSocketConnection struct {
	ID                   string
	EndpointID           string
	Conn                 *websocket.Conn
	URL                  string
	IsActive             bool
	LastPing             time.Time
	LastPong             time.Time
	
	// Message handling
	MessageChan          chan *WSMessage
	ErrorChan            chan error
	
	// Connection state
	ConnectedAt          time.Time
	LastActivity         time.Time
	ReconnectAttempts    int
	MaxReconnectAttempts int
}

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type                 int
	Data                 []byte
	Timestamp            time.Time
	ConnectionID         string
}

// AuthProvider interface for authentication providers
type AuthProvider interface {
	GetToken(ctx context.Context, config map[string]interface{}) (*AuthToken, error)
	RefreshToken(ctx context.Context, token *AuthToken) (*AuthToken, error)
	ValidateToken(token *AuthToken) bool
}

// AuthToken represents an authentication token
type AuthToken struct {
	Type                 string // "Bearer", "Basic", "API-Key", etc.
	Token                string
	RefreshToken         string
	ExpiresAt            time.Time
	Scopes               []string
	Metadata             map[string]interface{}
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	tokens               int
	maxTokens            int
	refillRate           time.Duration
	lastRefill           time.Time
	mu                   sync.Mutex
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxRetries           int
	InitialDelay         time.Duration
	MaxDelay             time.Duration
	BackoffMultiplier    float64
	RetryableStatusCodes []int
	RetryableErrors      []string
}

// CircuitBreaker implements circuit breaker pattern
type CircuitBreaker struct {
	mu                   sync.RWMutex
	state                CircuitState
	failures             int
	lastFailureTime      time.Time
	failureThreshold     int
	recoveryTimeout      time.Duration
	onStateChange        func(from, to CircuitState)
}

// CircuitState represents circuit breaker state
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// ResponseProcessor processes API responses
type ResponseProcessor func(*APIResponse) error

// APIMiddleware represents middleware for API requests
type APIMiddleware func(*APIRequest, func(*APIRequest) (*APIResponse, error)) (*APIResponse, error)

// ProxyAuth represents proxy authentication
type ProxyAuth struct {
	Username string
	Password string
}

// APIExample represents an API usage example
type APIExample struct {
	Name        string
	Description string
	Request     *APIRequest
	Response    *APIResponse
}

// DefaultExternalAPIConfig returns default configuration
func DefaultExternalAPIConfig() ExternalAPIConfig {
	return ExternalAPIConfig{
		RequestTimeout:        30 * time.Second,
		MaxConcurrentRequests: 10,
		MaxRetries:            3,
		RetryDelay:            1 * time.Second,
		
		GlobalRateLimit:       100,
		DefaultAPIRateLimit:   50,
		BurstSize:             10,
		
		AllowHTTP:             false,
		VerifySSL:             true,
		MaxResponseSize:       10 * 1024 * 1024, // 10MB
		AllowedDomains:        []string{},
		BlockedDomains:        []string{},
		
		WSReadBufferSize:      1024,
		WSWriteBufferSize:     1024,
		WSHandshakeTimeout:    10 * time.Second,
		WSPingInterval:        30 * time.Second,
		WSPongTimeout:         10 * time.Second,
		
		DefaultAuthType:       "Bearer",
		AuthCacheTTL:          1 * time.Hour,
		
		StreamResponses:       true,
		StreamingInterval:     100 * time.Millisecond,
		MaxStreamingSize:      50 * 1024 * 1024, // 50MB
		
		CircuitBreakerEnabled: true,
		FailureThreshold:      5,
		RecoveryTimeout:       1 * time.Minute,
	}
}

// NewExternalAPIManager creates a new external API manager
func NewExternalAPIManager(config ExternalAPIConfig, uiUpdateChan chan tea.Msg) *ExternalAPIManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Create HTTP client with configuration
	httpClient := &http.Client{
		Timeout: config.RequestTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: !config.VerifySSL,
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	
	// Set up proxy if configured
	if config.ProxyURL != "" {
		proxyURL, err := url.Parse(config.ProxyURL)
		if err == nil {
			httpClient.Transport.(*http.Transport).Proxy = http.ProxyURL(proxyURL)
		}
	}
	
	return &ExternalAPIManager{
		uiUpdateChan:       uiUpdateChan,
		httpClient:         httpClient,
		ctx:                ctx,
		cancel:             cancel,
		config:             config,
		registeredAPIs:     make(map[string]*APIEndpoint),
		activeRequests:     make(map[string]*APIRequest),
		requestQueue:       make(chan *APIRequest, config.MaxConcurrentRequests*2),
		responseQueue:      make(chan *APIResponse, config.MaxConcurrentRequests*2),
		rateLimiters:       make(map[string]*RateLimiter),
		wsConnections:      make(map[string]*WebSocketConnection),
		authProviders:      make(map[string]AuthProvider),
		tokenCache:         make(map[string]*AuthToken),
		responseProcessors: make(map[string]ResponseProcessor),
		middlewares:        make([]APIMiddleware, 0),
		wsUpgrader: websocket.Upgrader{
			ReadBufferSize:  config.WSReadBufferSize,
			WriteBufferSize: config.WSWriteBufferSize,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now
			},
		},
	}
}

// Start initializes the external API manager
func (eam *ExternalAPIManager) Start() error {
	eam.mu.Lock()
	defer eam.mu.Unlock()
	
	if eam.isActive {
		return fmt.Errorf("external API manager already active")
	}
	
	// Initialize global rate limiter
	eam.globalRateLimit = NewRateLimiter(eam.config.GlobalRateLimit, eam.config.BurstSize)
	
	// Register default auth providers
	eam.registerDefaultAuthProviders()
	
	// Start worker goroutines
	for i := 0; i < eam.config.MaxConcurrentRequests; i++ {
		go eam.requestWorker(i)
	}
	
	// Start response processor
	go eam.responseProcessor()
	
	// Start WebSocket connection manager
	go eam.websocketManager()
	
	// Start token refresh manager
	go eam.tokenRefreshManager()
	
	eam.isActive = true
	
	log.Printf("[EXTERNAL_API] Started external API manager with %d workers", eam.config.MaxConcurrentRequests)
	
	if eam.uiUpdateChan != nil {
		eam.uiUpdateChan <- ExternalAPIManagerStartedMsg{}
	}
	
	return nil
}

// Stop shuts down the external API manager
func (eam *ExternalAPIManager) Stop() error {
	eam.mu.Lock()
	defer eam.mu.Unlock()
	
	if !eam.isActive {
		return fmt.Errorf("external API manager not active")
	}
	
	eam.cancel()
	eam.isActive = false
	
	// Close all WebSocket connections
	for _, conn := range eam.wsConnections {
		if conn.Conn != nil {
			conn.Conn.Close()
		}
	}
	
	// Clear active requests
	eam.activeRequests = make(map[string]*APIRequest)
	
	log.Printf("[EXTERNAL_API] Stopped external API manager")
	
	if eam.uiUpdateChan != nil {
		eam.uiUpdateChan <- ExternalAPIManagerStoppedMsg{
			TotalRequests:      eam.totalRequests,
			SuccessfulRequests: eam.successfulRequests,
			FailedRequests:     eam.failedRequests,
		}
	}
	
	return nil
}

// RegisterAPI registers an external API endpoint
func (eam *ExternalAPIManager) RegisterAPI(endpoint *APIEndpoint) error {
	eam.mu.Lock()
	defer eam.mu.Unlock()
	
	// Validate endpoint
	if endpoint.ID == "" {
		return fmt.Errorf("endpoint ID cannot be empty")
	}
	
	if endpoint.BaseURL == "" {
		return fmt.Errorf("endpoint base URL cannot be empty")
	}
	
	// Set defaults
	if endpoint.RateLimit == 0 {
		endpoint.RateLimit = eam.config.DefaultAPIRateLimit
	}
	
	if endpoint.Timeout == 0 {
		endpoint.Timeout = eam.config.RequestTimeout
	}
	
	if endpoint.RetryPolicy == nil {
		endpoint.RetryPolicy = &RetryPolicy{
			MaxRetries:           eam.config.MaxRetries,
			InitialDelay:         eam.config.RetryDelay,
			MaxDelay:             30 * time.Second,
			BackoffMultiplier:    2.0,
			RetryableStatusCodes: []int{500, 502, 503, 504},
		}
	}
	
	// Create circuit breaker if enabled
	if eam.config.CircuitBreakerEnabled {
		endpoint.CircuitBreaker = &CircuitBreaker{
			failureThreshold: eam.config.FailureThreshold,
			recoveryTimeout:  eam.config.RecoveryTimeout,
			state:            CircuitClosed,
		}
	}
	
	// Register the endpoint
	eam.registeredAPIs[endpoint.ID] = endpoint
	
	// Create rate limiter for this API
	eam.rateLimiters[endpoint.ID] = NewRateLimiter(endpoint.RateLimit, eam.config.BurstSize)
	
	log.Printf("[EXTERNAL_API] Registered API endpoint: %s (%s)", endpoint.Name, endpoint.ID)
	
	return nil
}

// MakeRequest makes an API request
func (eam *ExternalAPIManager) MakeRequest(endpointID, method, path string, headers map[string]string, body []byte, context *ExecutionContext) (*APIResponse, error) {
	eam.mu.Lock()
	endpoint, exists := eam.registeredAPIs[endpointID]
	if !exists {
		eam.mu.Unlock()
		return nil, fmt.Errorf("API endpoint not found: %s", endpointID)
	}
	
	eam.requestCounter++
	requestID := fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), eam.requestCounter)
	eam.mu.Unlock()
	
	// Build full URL
	fullURL := endpoint.BaseURL + path
	
	// Create request
	request := &APIRequest{
		ID:           requestID,
		EndpointID:   endpointID,
		Method:       method,
		URL:          fullURL,
		Headers:      headers,
		Body:         body,
		Context:      context,
		StartTime:    time.Now(),
		Timeout:      endpoint.Timeout,
		ResponseChan: make(chan *APIResponse, 1),
	}
	
	// Merge endpoint headers
	if request.Headers == nil {
		request.Headers = make(map[string]string)
	}
	for key, value := range endpoint.Headers {
		if _, exists := request.Headers[key]; !exists {
			request.Headers[key] = value
		}
	}
	
	// Add authentication if required
	if err := eam.addAuthentication(request, endpoint); err != nil {
		return nil, fmt.Errorf("failed to add authentication: %w", err)
	}
	
	// Check rate limits
	if !eam.checkRateLimit(endpointID) {
		return nil, fmt.Errorf("rate limit exceeded for API: %s", endpointID)
	}
	
	// Check circuit breaker
	if endpoint.CircuitBreaker != nil && !endpoint.CircuitBreaker.Allow() {
		return nil, fmt.Errorf("circuit breaker open for API: %s", endpointID)
	}
	
	// Add to active requests
	eam.mu.Lock()
	eam.activeRequests[requestID] = request
	eam.totalRequests++
	eam.mu.Unlock()
	
	// Queue for execution
	select {
	case eam.requestQueue <- request:
		log.Printf("[EXTERNAL_API] Queued request: %s %s (ID: %s)", method, path, requestID)
	case <-eam.ctx.Done():
		return nil, fmt.Errorf("external API manager shutting down")
	default:
		return nil, fmt.Errorf("request queue full")
	}
	
	// Wait for response
	select {
	case response := <-request.ResponseChan:
		eam.mu.Lock()
		delete(eam.activeRequests, requestID)
		if response.Success {
			eam.successfulRequests++
		} else {
			eam.failedRequests++
		}
		eam.mu.Unlock()
		
		return response, nil
	case <-eam.ctx.Done():
		return nil, fmt.Errorf("external API manager shutting down")
	case <-time.After(request.Timeout):
		eam.mu.Lock()
		delete(eam.activeRequests, requestID)
		eam.failedRequests++
		eam.mu.Unlock()
		
		return nil, fmt.Errorf("request timeout")
	}
}

// ConnectWebSocket establishes a WebSocket connection
func (eam *ExternalAPIManager) ConnectWebSocket(endpointID string, headers map[string]string, context *ExecutionContext) (*WebSocketConnection, error) {
	eam.mu.Lock()
	endpoint, exists := eam.registeredAPIs[endpointID]
	if !exists {
		eam.mu.Unlock()
		return nil, fmt.Errorf("API endpoint not found: %s", endpointID)
	}
	
	if !endpoint.IsWebSocket {
		eam.mu.Unlock()
		return nil, fmt.Errorf("endpoint is not configured for WebSocket: %s", endpointID)
	}
	
	connID := fmt.Sprintf("ws_%s_%d", endpointID, time.Now().UnixNano())
	eam.mu.Unlock()
	
	// Create WebSocket dialer
	dialer := &websocket.Dialer{
		HandshakeTimeout: eam.config.WSHandshakeTimeout,
		ReadBufferSize:   eam.config.WSReadBufferSize,
		WriteBufferSize:  eam.config.WSWriteBufferSize,
		Subprotocols:     endpoint.WSProtocols,
	}
	
	// Set headers
	if headers == nil {
		headers = make(map[string]string)
	}
	if endpoint.WSOrigin != "" {
		headers["Origin"] = endpoint.WSOrigin
	}
	
	// Add authentication
	if err := eam.addWebSocketAuth(headers, endpoint); err != nil {
		return nil, fmt.Errorf("failed to add WebSocket authentication: %w", err)
	}
	
	// Connect
	conn, _, err := dialer.Dial(endpoint.BaseURL, http.Header(headers))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
	}
	
	// Create connection object
	wsConn := &WebSocketConnection{
		ID:                   connID,
		EndpointID:           endpointID,
		Conn:                 conn,
		URL:                  endpoint.BaseURL,
		IsActive:             true,
		ConnectedAt:          time.Now(),
		LastActivity:         time.Now(),
		MessageChan:          make(chan *WSMessage, 100),
		ErrorChan:            make(chan error, 10),
		MaxReconnectAttempts: 5,
	}
	
	// Store connection
	eam.mu.Lock()
	eam.wsConnections[connID] = wsConn
	eam.mu.Unlock()
	
	// Start message handlers
	go eam.handleWebSocketMessages(wsConn)
	go eam.handleWebSocketPing(wsConn)
	
	log.Printf("[EXTERNAL_API] Established WebSocket connection: %s", connID)
	
	return wsConn, nil
}

// requestWorker processes API requests from the queue
func (eam *ExternalAPIManager) requestWorker(workerID int) {
	log.Printf("[EXTERNAL_API] Started request worker %d", workerID)
	
	for {
		select {
		case <-eam.ctx.Done():
			log.Printf("[EXTERNAL_API] Request worker %d shutting down", workerID)
			return
		case request := <-eam.requestQueue:
			eam.processRequest(request, workerID)
		}
	}
}

// processRequest processes a single API request
func (eam *ExternalAPIManager) processRequest(request *APIRequest, workerID int) {
	startTime := time.Now()
	
	log.Printf("[EXTERNAL_API] Worker %d processing: %s %s (ID: %s)", 
		workerID, request.Method, request.URL, request.ID)
	
	// Get endpoint
	eam.mu.RLock()
	endpoint := eam.registeredAPIs[request.EndpointID]
	eam.mu.RUnlock()
	
	// Apply middleware
	response, err := eam.applyMiddleware(request, endpoint, func(req *APIRequest) (*APIResponse, error) {
		return eam.executeRequest(req, endpoint)
	})
	
	if err != nil {
		response = &APIResponse{
			RequestID:    request.ID,
			Success:      false,
			Error:        err,
			ResponseTime: time.Since(startTime),
			Timestamp:    time.Now(),
		}
	}
	
	// Update circuit breaker
	if endpoint.CircuitBreaker != nil {
		if response.Success {
			endpoint.CircuitBreaker.OnSuccess()
		} else {
			endpoint.CircuitBreaker.OnFailure()
		}
	}
	
	// Send response
	eam.sendResponse(request, response)
	
	log.Printf("[EXTERNAL_API] Worker %d completed: %s (ID: %s) in %v", 
		workerID, request.URL, request.ID, time.Since(startTime))
}

// executeRequest executes an HTTP request
func (eam *ExternalAPIManager) executeRequest(request *APIRequest, endpoint *APIEndpoint) (*APIResponse, error) {
	// Create HTTP request
	var bodyReader io.Reader
	if len(request.Body) > 0 {
		bodyReader = bytes.NewReader(request.Body)
	}
	
	httpReq, err := http.NewRequestWithContext(eam.ctx, request.Method, request.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	
	// Set headers
	for key, value := range request.Headers {
		httpReq.Header.Set(key, value)
	}
	
	// Set default headers
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", "AIStudio/1.0")
	}
	
	// Execute request
	startTime := time.Now()
	resp, err := eam.httpClient.Do(httpReq)
	responseTime := time.Since(startTime)
	
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	// Check response size limit
	if int64(len(body)) > eam.config.MaxResponseSize {
		return nil, fmt.Errorf("response size exceeds limit: %d bytes", len(body))
	}
	
	// Create response
	response := &APIResponse{
		RequestID:     request.ID,
		StatusCode:    resp.StatusCode,
		Headers:       make(map[string]string),
		Body:          body,
		ContentType:   resp.Header.Get("Content-Type"),
		ResponseTime:  responseTime,
		Success:       resp.StatusCode >= 200 && resp.StatusCode < 300,
		ContentLength: resp.ContentLength,
		Timestamp:     time.Now(),
	}
	
	// Copy headers
	for key, values := range resp.Header {
		if len(values) > 0 {
			response.Headers[key] = values[0]
		}
	}
	
	// Parse response body
	eam.parseResponseBody(response)
	
	// Extract rate limit information
	eam.extractRateLimitInfo(response)
	
	return response, nil
}

// parseResponseBody parses response body based on content type
func (eam *ExternalAPIManager) parseResponseBody(response *APIResponse) {
	contentType := strings.ToLower(response.ContentType)
	
	switch {
	case strings.Contains(contentType, "application/json"):
		var jsonData map[string]interface{}
		if err := json.Unmarshal(response.Body, &jsonData); err == nil {
			response.JSONData = jsonData
		}
	case strings.Contains(contentType, "text/"):
		response.TextData = string(response.Body)
	}
}

// extractRateLimitInfo extracts rate limit information from response headers
func (eam *ExternalAPIManager) extractRateLimitInfo(response *APIResponse) {
	if remaining := response.Headers["X-RateLimit-Remaining"]; remaining != "" {
		// Parse remaining requests
		// Implementation would depend on specific API format
	}
	
	if reset := response.Headers["X-RateLimit-Reset"]; reset != "" {
		// Parse reset time
		// Implementation would depend on specific API format
	}
}

// applyMiddleware applies middleware to the request
func (eam *ExternalAPIManager) applyMiddleware(request *APIRequest, endpoint *APIEndpoint, handler func(*APIRequest) (*APIResponse, error)) (*APIResponse, error) {
	// Apply endpoint-specific middleware
	currentHandler := handler
	for i := len(endpoint.Middleware) - 1; i >= 0; i-- {
		middleware := endpoint.Middleware[i]
		nextHandler := currentHandler
		currentHandler = func(req *APIRequest) (*APIResponse, error) {
			return middleware(req, nextHandler)
		}
	}
	
	// Apply global middleware
	for i := len(eam.middlewares) - 1; i >= 0; i-- {
		middleware := eam.middlewares[i]
		nextHandler := currentHandler
		currentHandler = func(req *APIRequest) (*APIResponse, error) {
			return middleware(req, nextHandler)
		}
	}
	
	return currentHandler(request)
}

// sendResponse sends the response to the request
func (eam *ExternalAPIManager) sendResponse(request *APIRequest, response *APIResponse) {
	select {
	case request.ResponseChan <- response:
		// Response sent successfully
	case <-eam.ctx.Done():
		// Manager shutting down
	case <-time.After(1 * time.Second):
		log.Printf("[EXTERNAL_API] Warning: Response channel blocked for request %s", request.ID)
	}
	
	// Send to response queue for processing
	select {
	case eam.responseQueue <- response:
	default:
		log.Printf("[EXTERNAL_API] Warning: Response queue full, dropping response for request %s", request.ID)
	}
}

// responseProcessor processes responses and sends UI updates
func (eam *ExternalAPIManager) responseProcessor() {
	for {
		select {
		case <-eam.ctx.Done():
			return
		case response := <-eam.responseQueue:
			eam.processResponseForUI(response)
		}
	}
}

// processResponseForUI processes responses for UI updates
func (eam *ExternalAPIManager) processResponseForUI(response *APIResponse) {
	if eam.uiUpdateChan != nil {
		eam.uiUpdateChan <- APIResponseMsg{
			Response: response,
		}
	}
	
	// Log the response
	if response.Success {
		log.Printf("[EXTERNAL_API] Success: %s (ID: %s) in %v - %d", 
			response.RequestID, response.RequestID, response.ResponseTime, response.StatusCode)
	} else {
		log.Printf("[EXTERNAL_API] Failed: %s (ID: %s) in %v - %v", 
			response.RequestID, response.RequestID, response.ResponseTime, response.Error)
	}
}

// websocketManager manages WebSocket connections
func (eam *ExternalAPIManager) websocketManager() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-eam.ctx.Done():
			return
		case <-ticker.C:
			eam.maintainWebSocketConnections()
		}
	}
}

// maintainWebSocketConnections maintains active WebSocket connections
func (eam *ExternalAPIManager) maintainWebSocketConnections() {
	eam.mu.Lock()
	defer eam.mu.Unlock()
	
	for id, conn := range eam.wsConnections {
		if !conn.IsActive {
			continue
		}
		
		// Check if connection is still alive
		if time.Since(conn.LastActivity) > 5*time.Minute {
			log.Printf("[EXTERNAL_API] Closing inactive WebSocket connection: %s", id)
			conn.Conn.Close()
			conn.IsActive = false
			delete(eam.wsConnections, id)
		}
	}
}

// handleWebSocketMessages handles incoming WebSocket messages
func (eam *ExternalAPIManager) handleWebSocketMessages(conn *WebSocketConnection) {
	for {
		messageType, data, err := conn.Conn.ReadMessage()
		if err != nil {
			log.Printf("[EXTERNAL_API] WebSocket read error: %v", err)
			conn.ErrorChan <- err
			break
		}
		
		conn.LastActivity = time.Now()
		
		message := &WSMessage{
			Type:         messageType,
			Data:         data,
			Timestamp:    time.Now(),
			ConnectionID: conn.ID,
		}
		
		select {
		case conn.MessageChan <- message:
		default:
			log.Printf("[EXTERNAL_API] WebSocket message buffer full, dropping message")
		}
		
		// Send to UI
		if eam.uiUpdateChan != nil {
			eam.uiUpdateChan <- WebSocketMessageMsg{
				Message: message,
			}
		}
	}
}

// handleWebSocketPing handles WebSocket ping/pong
func (eam *ExternalAPIManager) handleWebSocketPing(conn *WebSocketConnection) {
	ticker := time.NewTicker(eam.config.WSPingInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-eam.ctx.Done():
			return
		case <-ticker.C:
			if !conn.IsActive {
				return
			}
			
			conn.LastPing = time.Now()
			if err := conn.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("[EXTERNAL_API] WebSocket ping error: %v", err)
				conn.ErrorChan <- err
				return
			}
		}
	}
}

// tokenRefreshManager manages token refresh
func (eam *ExternalAPIManager) tokenRefreshManager() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-eam.ctx.Done():
			return
		case <-ticker.C:
			eam.refreshExpiredTokens()
		}
	}
}

// refreshExpiredTokens refreshes expired authentication tokens
func (eam *ExternalAPIManager) refreshExpiredTokens() {
	eam.mu.Lock()
	defer eam.mu.Unlock()
	
	for key, token := range eam.tokenCache {
		if time.Now().Add(5*time.Minute).After(token.ExpiresAt) {
			// Token will expire soon, refresh it
			log.Printf("[EXTERNAL_API] Refreshing token for: %s", key)
			// Implementation would depend on specific auth provider
		}
	}
}

// addAuthentication adds authentication to the request
func (eam *ExternalAPIManager) addAuthentication(request *APIRequest, endpoint *APIEndpoint) error {
	if endpoint.AuthType == "" {
		return nil
	}
	
	// Get cached token
	token, exists := eam.tokenCache[endpoint.ID]
	if !exists || time.Now().After(token.ExpiresAt) {
		// Get fresh token
		provider, exists := eam.authProviders[endpoint.AuthType]
		if !exists {
			return fmt.Errorf("auth provider not found: %s", endpoint.AuthType)
		}
		
		var err error
		token, err = provider.GetToken(eam.ctx, endpoint.AuthConfig)
		if err != nil {
			return fmt.Errorf("failed to get auth token: %w", err)
		}
		
		eam.tokenCache[endpoint.ID] = token
	}
	
	// Add token to request
	switch token.Type {
	case "Bearer":
		request.Headers["Authorization"] = fmt.Sprintf("Bearer %s", token.Token)
	case "Basic":
		request.Headers["Authorization"] = fmt.Sprintf("Basic %s", token.Token)
	case "API-Key":
		request.Headers["X-API-Key"] = token.Token
	default:
		return fmt.Errorf("unsupported auth type: %s", token.Type)
	}
	
	return nil
}

// addWebSocketAuth adds authentication for WebSocket connections
func (eam *ExternalAPIManager) addWebSocketAuth(headers map[string]string, endpoint *APIEndpoint) error {
	if endpoint.AuthType == "" {
		return nil
	}
	
	// Similar to addAuthentication but for WebSocket headers
	token, exists := eam.tokenCache[endpoint.ID]
	if !exists || time.Now().After(token.ExpiresAt) {
		provider, exists := eam.authProviders[endpoint.AuthType]
		if !exists {
			return fmt.Errorf("auth provider not found: %s", endpoint.AuthType)
		}
		
		var err error
		token, err = provider.GetToken(eam.ctx, endpoint.AuthConfig)
		if err != nil {
			return fmt.Errorf("failed to get auth token: %w", err)
		}
		
		eam.tokenCache[endpoint.ID] = token
	}
	
	switch token.Type {
	case "Bearer":
		headers["Authorization"] = fmt.Sprintf("Bearer %s", token.Token)
	case "Basic":
		headers["Authorization"] = fmt.Sprintf("Basic %s", token.Token)
	case "API-Key":
		headers["X-API-Key"] = token.Token
	}
	
	return nil
}

// checkRateLimit checks if the request is within rate limits
func (eam *ExternalAPIManager) checkRateLimit(endpointID string) bool {
	// Check global rate limit
	if !eam.globalRateLimit.Allow() {
		return false
	}
	
	// Check endpoint-specific rate limit
	if limiter, exists := eam.rateLimiters[endpointID]; exists {
		return limiter.Allow()
	}
	
	return true
}

// registerDefaultAuthProviders registers default authentication providers
func (eam *ExternalAPIManager) registerDefaultAuthProviders() {
	// Register Bearer token provider
	eam.authProviders["Bearer"] = &BearerAuthProvider{}
	
	// Register API key provider
	eam.authProviders["API-Key"] = &APIKeyAuthProvider{}
	
	// Register OAuth provider
	eam.authProviders["OAuth"] = &OAuthProvider{}
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(tokensPerSecond, maxTokens int) *RateLimiter {
	return &RateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: time.Second / time.Duration(tokensPerSecond),
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed under rate limiting
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	
	// Refill tokens based on elapsed time
	tokensToAdd := int(elapsed / rl.refillRate)
	if tokensToAdd > 0 {
		rl.tokens = min(rl.maxTokens, rl.tokens+tokensToAdd)
		rl.lastRefill = now
	}
	
	// Check if we have tokens available
	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	
	return false
}

// Allow checks if a request is allowed through the circuit breaker
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		return time.Since(cb.lastFailureTime) > cb.recoveryTimeout
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

// OnSuccess records a successful operation
func (cb *CircuitBreaker) OnSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
		cb.failures = 0
		if cb.onStateChange != nil {
			cb.onStateChange(CircuitHalfOpen, CircuitClosed)
		}
	}
}

// OnFailure records a failed operation
func (cb *CircuitBreaker) OnFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.failures++
	cb.lastFailureTime = time.Now()
	
	if cb.failures >= cb.failureThreshold {
		if cb.state == CircuitClosed {
			cb.state = CircuitOpen
			if cb.onStateChange != nil {
				cb.onStateChange(CircuitClosed, CircuitOpen)
			}
		} else if cb.state == CircuitHalfOpen {
			cb.state = CircuitOpen
			if cb.onStateChange != nil {
				cb.onStateChange(CircuitHalfOpen, CircuitOpen)
			}
		}
	}
}

// Default auth provider implementations
type BearerAuthProvider struct{}

func (p *BearerAuthProvider) GetToken(ctx context.Context, config map[string]interface{}) (*AuthToken, error) {
	token, ok := config["token"].(string)
	if !ok {
		return nil, fmt.Errorf("token not found in config")
	}
	
	return &AuthToken{
		Type:      "Bearer",
		Token:     token,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}, nil
}

func (p *BearerAuthProvider) RefreshToken(ctx context.Context, token *AuthToken) (*AuthToken, error) {
	return token, nil // Bearer tokens typically don't refresh
}

func (p *BearerAuthProvider) ValidateToken(token *AuthToken) bool {
	return time.Now().Before(token.ExpiresAt)
}

type APIKeyAuthProvider struct{}

func (p *APIKeyAuthProvider) GetToken(ctx context.Context, config map[string]interface{}) (*AuthToken, error) {
	key, ok := config["api_key"].(string)
	if !ok {
		return nil, fmt.Errorf("api_key not found in config")
	}
	
	return &AuthToken{
		Type:      "API-Key",
		Token:     key,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}, nil
}

func (p *APIKeyAuthProvider) RefreshToken(ctx context.Context, token *AuthToken) (*AuthToken, error) {
	return token, nil // API keys typically don't refresh
}

func (p *APIKeyAuthProvider) ValidateToken(token *AuthToken) bool {
	return time.Now().Before(token.ExpiresAt)
}

type OAuthProvider struct{}

func (p *OAuthProvider) GetToken(ctx context.Context, config map[string]interface{}) (*AuthToken, error) {
	// OAuth implementation would be more complex
	return nil, fmt.Errorf("OAuth provider not implemented")
}

func (p *OAuthProvider) RefreshToken(ctx context.Context, token *AuthToken) (*AuthToken, error) {
	return nil, fmt.Errorf("OAuth provider not implemented")
}

func (p *OAuthProvider) ValidateToken(token *AuthToken) bool {
	return false
}

// Utility function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetActiveRequestsCount returns the number of active API requests
func (eam *ExternalAPIManager) GetActiveRequestsCount() int {
	eam.mu.RLock()
	defer eam.mu.RUnlock()
	return len(eam.activeRequests)
}

// GetActiveWebSocketsCount returns the number of active WebSocket connections
func (eam *ExternalAPIManager) GetActiveWebSocketsCount() int {
	eam.mu.RLock()
	defer eam.mu.RUnlock()
	
	active := 0
	for _, conn := range eam.wsConnections {
		if conn.IsActive {
			active++
		}
	}
	return active
}

// GetMetrics returns performance metrics
func (eam *ExternalAPIManager) GetMetrics() ExternalAPIMetrics {
	eam.mu.RLock()
	defer eam.mu.RUnlock()
	
	return ExternalAPIMetrics{
		TotalRequests:       eam.totalRequests,
		SuccessfulRequests:  eam.successfulRequests,
		FailedRequests:      eam.failedRequests,
		ActiveRequests:      int64(len(eam.activeRequests)),
		ActiveWebSockets:    int64(eam.GetActiveWebSocketsCount()),
		AverageResponseTime: eam.averageResponseTime,
		RegisteredAPIs:      int64(len(eam.registeredAPIs)),
	}
}

// AddMiddleware adds global middleware
func (eam *ExternalAPIManager) AddMiddleware(middleware APIMiddleware) {
	eam.mu.Lock()
	defer eam.mu.Unlock()
	
	eam.middlewares = append(eam.middlewares, middleware)
}

// UpdateConfig updates the external API configuration
func (eam *ExternalAPIManager) UpdateConfig(config ExternalAPIConfig) error {
	eam.mu.Lock()
	defer eam.mu.Unlock()
	
	eam.config = config
	
	log.Printf("[EXTERNAL_API] Updated configuration: timeout=%v, max_concurrent=%d", 
		config.RequestTimeout, config.MaxConcurrentRequests)
	
	return nil
}

// Message types for UI integration
type ExternalAPIManagerStartedMsg struct{}
type ExternalAPIManagerStoppedMsg struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
}
type APIResponseMsg struct {
	Response *APIResponse
}
type WebSocketMessageMsg struct {
	Message *WSMessage
}

// ExternalAPIMetrics contains performance metrics
type ExternalAPIMetrics struct {
	TotalRequests       int64
	SuccessfulRequests  int64
	FailedRequests      int64
	ActiveRequests      int64
	ActiveWebSockets    int64
	AverageResponseTime time.Duration
	RegisteredAPIs      int64
}