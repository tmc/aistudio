package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// SessionManager manages persistent sessions for long-running conversations
type SessionManager struct {
	// Configuration
	config            SessionConfig
	isActive          bool
	
	// Session management
	activeSessions    map[string]*Session
	sessionsMutex     sync.RWMutex
	
	// Persistence
	storageProvider   StorageProvider
	sessionStore      SessionStore
	
	// Context and lifecycle
	ctx               context.Context
	cancel            context.CancelFunc
	
	// Channels
	sessionUpdateChan chan SessionUpdate
	uiUpdateChan      chan tea.Msg
	
	// Metrics
	totalSessions     int64
	activeSessionCount int64
	
	// Cleanup
	cleanupTicker     *time.Ticker
	cleanupInterval   time.Duration
}

// SessionConfig contains configuration for session management
type SessionConfig struct {
	// Storage settings
	StorageType       string        // "file", "database", "memory"
	StorageLocation   string        // Directory path or connection string
	MaxSessionAge     time.Duration // Maximum session age before cleanup
	MaxSessionSize    int64         // Maximum session size in bytes
	
	// Persistence settings
	AutoSave          bool          // Enable automatic session saving
	SaveInterval      time.Duration // Interval for automatic saves
	BackupEnabled     bool          // Enable session backups
	BackupInterval    time.Duration // Backup interval
	BackupRetention   int           // Number of backups to retain
	
	// Session settings
	SessionTimeout    time.Duration // Session timeout for inactive sessions
	MaxSessions       int           // Maximum concurrent sessions
	EnableCompression bool          // Enable session data compression
	EncryptionEnabled bool          // Enable session encryption
	EncryptionKey     string        // Encryption key for sessions
	
	// Cleanup settings
	CleanupInterval   time.Duration // Interval for cleanup operations
	EnableAutoCleanup bool          // Enable automatic cleanup
	
	// Feature flags
	EnableMetrics     bool          // Enable session metrics
	EnableEvents      bool          // Enable session events
	EnableAuditLog    bool          // Enable audit logging
}

// Session represents a persistent conversation session
type Session struct {
	// Identity
	ID                string                 `json:"id"`
	UserID            string                 `json:"user_id"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	
	// Timestamps
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	LastAccessedAt    time.Time              `json:"last_accessed_at"`
	ExpiresAt         time.Time              `json:"expires_at"`
	
	// Conversation state
	Messages          []SessionMessage       `json:"messages"`
	Context           map[string]interface{} `json:"context"`
	SystemPrompt      string                 `json:"system_prompt"`
	ModelConfig       ModelConfiguration     `json:"model_config"`
	
	// Streaming state
	StreamingState    *StreamingState        `json:"streaming_state,omitempty"`
	AudioContext      *AudioContext          `json:"audio_context,omitempty"`
	VideoContext      *VideoContext          `json:"video_context,omitempty"`
	
	// Multimodal data
	Attachments       []SessionAttachment    `json:"attachments"`
	SharedFiles       []SharedFile           `json:"shared_files"`
	GeneratedContent  []GeneratedContent     `json:"generated_content"`
	
	// Session metadata
	Tags              []string               `json:"tags"`
	Metadata          map[string]interface{} `json:"metadata"`
	Size              int64                  `json:"size"`
	MessageCount      int64                  `json:"message_count"`
	
	// State management
	IsActive          bool                   `json:"is_active"`
	IsPersisted       bool                   `json:"is_persisted"`
	Version           int64                  `json:"version"`
	
	// Synchronization
	mutex             sync.RWMutex
	lastSaved         time.Time
	isDirty           bool
}

// SessionMessage represents a message in a session
type SessionMessage struct {
	ID                string                 `json:"id"`
	Role              string                 `json:"role"` // "user", "assistant", "system"
	Content           string                 `json:"content"`
	Timestamp         time.Time              `json:"timestamp"`
	MessageType       string                 `json:"message_type"` // "text", "audio", "video", "image", "tool"
	
	// Multimodal content
	AudioData         []byte                 `json:"audio_data,omitempty"`
	VideoData         []byte                 `json:"video_data,omitempty"`
	ImageData         []byte                 `json:"image_data,omitempty"`
	
	// Tool interactions
	ToolCalls         []ToolCall             `json:"tool_calls,omitempty"`
	ToolResults       []ToolResult           `json:"tool_results,omitempty"`
	
	// Metadata
	Metadata          map[string]interface{} `json:"metadata"`
	ProcessingTime    time.Duration          `json:"processing_time"`
	TokenCount        int                    `json:"token_count"`
	
	// State
	IsStreaming       bool                   `json:"is_streaming"`
	IsComplete        bool                   `json:"is_complete"`
	IsError           bool                   `json:"is_error"`
	ErrorMessage      string                 `json:"error_message,omitempty"`
}

// ModelConfiguration represents model configuration for a session
type ModelConfiguration struct {
	ModelName         string                 `json:"model_name"`
	Temperature       float32                `json:"temperature"`
	TopP              float32                `json:"top_p"`
	TopK              int                    `json:"top_k"`
	MaxTokens         int                    `json:"max_tokens"`
	SystemPrompt      string                 `json:"system_prompt"`
	CustomParameters  map[string]interface{} `json:"custom_parameters"`
}

// StreamingState represents the current streaming state
type StreamingState struct {
	IsStreaming       bool                   `json:"is_streaming"`
	StreamType        string                 `json:"stream_type"` // "text", "audio", "video"
	StartedAt         time.Time              `json:"started_at"`
	LastActivity      time.Time              `json:"last_activity"`
	CurrentMessage    *SessionMessage        `json:"current_message,omitempty"`
	StreamingContext  map[string]interface{} `json:"streaming_context"`
}

// AudioContext represents audio-specific session context
type AudioContext struct {
	VoiceID           string                 `json:"voice_id"`
	AudioFormat       string                 `json:"audio_format"`
	SampleRate        int                    `json:"sample_rate"`
	LastAudioTimestamp time.Time             `json:"last_audio_timestamp"`
	AudioSettings     map[string]interface{} `json:"audio_settings"`
	TranscriptionHistory []string            `json:"transcription_history"`
}

// VideoContext represents video-specific session context
type VideoContext struct {
	VideoFormat       string                 `json:"video_format"`
	Resolution        string                 `json:"resolution"`
	FrameRate         int                    `json:"frame_rate"`
	LastFrameTimestamp time.Time             `json:"last_frame_timestamp"`
	VideoSettings     map[string]interface{} `json:"video_settings"`
	AnalysisHistory   []string               `json:"analysis_history"`
}

// SessionAttachment represents a file attachment in a session
type SessionAttachment struct {
	ID                string                 `json:"id"`
	FileName          string                 `json:"file_name"`
	MimeType          string                 `json:"mime_type"`
	Size              int64                  `json:"size"`
	Data              []byte                 `json:"data"`
	UploadedAt        time.Time              `json:"uploaded_at"`
	Metadata          map[string]interface{} `json:"metadata"`
}

// SharedFile represents a file shared in the session
type SharedFile struct {
	ID                string                 `json:"id"`
	Path              string                 `json:"path"`
	Name              string                 `json:"name"`
	Size              int64                  `json:"size"`
	ModifiedAt        time.Time              `json:"modified_at"`
	SharedAt          time.Time              `json:"shared_at"`
	Permissions       []string               `json:"permissions"`
}

// GeneratedContent represents AI-generated content in a session
type GeneratedContent struct {
	ID                string                 `json:"id"`
	Type              string                 `json:"type"` // "text", "image", "audio", "video", "code"
	Content           string                 `json:"content"`
	BinaryData        []byte                 `json:"binary_data,omitempty"`
	GeneratedAt       time.Time              `json:"generated_at"`
	ModelUsed         string                 `json:"model_used"`
	Prompt            string                 `json:"prompt"`
	Metadata          map[string]interface{} `json:"metadata"`
}

// ToolCall represents a function/tool call in a session
type ToolCall struct {
	ID                string                 `json:"id"`
	ToolName          string                 `json:"tool_name"`
	Arguments         map[string]interface{} `json:"arguments"`
	CalledAt          time.Time              `json:"called_at"`
}

// ToolResult represents the result of a tool call
type ToolResult struct {
	ID                string                 `json:"id"`
	ToolCallID        string                 `json:"tool_call_id"`
	Result            interface{}            `json:"result"`
	Success           bool                   `json:"success"`
	Error             string                 `json:"error,omitempty"`
	CompletedAt       time.Time              `json:"completed_at"`
	ExecutionTime     time.Duration          `json:"execution_time"`
}

// SessionUpdate represents an update to a session
type SessionUpdate struct {
	SessionID         string                 `json:"session_id"`
	UpdateType        string                 `json:"update_type"` // "message", "context", "state", "metadata"
	Data              interface{}            `json:"data"`
	Timestamp         time.Time              `json:"timestamp"`
}

// StorageProvider interface for session storage
type StorageProvider interface {
	SaveSession(session *Session) error
	LoadSession(sessionID string) (*Session, error)
	DeleteSession(sessionID string) error
	ListSessions(userID string) ([]*Session, error)
	BackupSession(sessionID string) error
	RestoreSession(sessionID string, backupID string) error
}

// SessionStore interface for session storage operations
type SessionStore interface {
	Save(session *Session) error
	Load(sessionID string) (*Session, error)
	Delete(sessionID string) error
	List(userID string) ([]string, error)
	Exists(sessionID string) bool
}

// DefaultSessionConfig returns default session configuration
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		StorageType:       "file",
		StorageLocation:   "sessions",
		MaxSessionAge:     30 * 24 * time.Hour, // 30 days
		MaxSessionSize:    100 * 1024 * 1024,   // 100MB
		
		AutoSave:          true,
		SaveInterval:      5 * time.Minute,
		BackupEnabled:     true,
		BackupInterval:    1 * time.Hour,
		BackupRetention:   5,
		
		SessionTimeout:    2 * time.Hour,
		MaxSessions:       100,
		EnableCompression: true,
		EncryptionEnabled: false,
		
		CleanupInterval:   1 * time.Hour,
		EnableAutoCleanup: true,
		
		EnableMetrics:     true,
		EnableEvents:      true,
		EnableAuditLog:    false,
	}
}

// NewSessionManager creates a new session manager
func NewSessionManager(config SessionConfig, uiUpdateChan chan tea.Msg) *SessionManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	manager := &SessionManager{
		config:            config,
		activeSessions:    make(map[string]*Session),
		sessionUpdateChan: make(chan SessionUpdate, 100),
		uiUpdateChan:      uiUpdateChan,
		ctx:               ctx,
		cancel:            cancel,
		cleanupInterval:   config.CleanupInterval,
	}
	
	// Initialize storage provider
	if err := manager.initializeStorage(); err != nil {
		log.Printf("[SESSION] Failed to initialize storage: %v", err)
	}
	
	// Start background processes
	go manager.updateProcessor()
	go manager.autoSaveProcessor()
	
	if config.EnableAutoCleanup {
		go manager.cleanupProcessor()
	}
	
	return manager
}

// initializeStorage initializes the storage provider
func (sm *SessionManager) initializeStorage() error {
	switch sm.config.StorageType {
	case "file":
		provider, err := NewFileStorageProvider(sm.config.StorageLocation)
		if err != nil {
			return fmt.Errorf("failed to create file storage provider: %v", err)
		}
		sm.storageProvider = provider
		sm.sessionStore = provider
		
	case "memory":
		provider := NewMemoryStorageProvider()
		sm.storageProvider = provider
		sm.sessionStore = provider
		
	default:
		return fmt.Errorf("unsupported storage type: %s", sm.config.StorageType)
	}
	
	return nil
}

// CreateSession creates a new session
func (sm *SessionManager) CreateSession(userID, name, description string) (*Session, error) {
	sm.sessionsMutex.Lock()
	defer sm.sessionsMutex.Unlock()
	
	// Check session limit
	if len(sm.activeSessions) >= sm.config.MaxSessions {
		return nil, fmt.Errorf("maximum number of sessions reached")
	}
	
	// Generate session ID
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %v", err)
	}
	
	// Create session
	session := &Session{
		ID:                sessionID,
		UserID:            userID,
		Name:              name,
		Description:       description,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		LastAccessedAt:    time.Now(),
		ExpiresAt:         time.Now().Add(sm.config.MaxSessionAge),
		Messages:          make([]SessionMessage, 0),
		Context:           make(map[string]interface{}),
		Attachments:       make([]SessionAttachment, 0),
		SharedFiles:       make([]SharedFile, 0),
		GeneratedContent:  make([]GeneratedContent, 0),
		Tags:              make([]string, 0),
		Metadata:          make(map[string]interface{}),
		IsActive:          true,
		Version:           1,
		isDirty:           true,
	}
	
	// Add to active sessions
	sm.activeSessions[sessionID] = session
	sm.totalSessions++
	sm.activeSessionCount++
	
	// Save session
	if err := sm.saveSession(session); err != nil {
		log.Printf("[SESSION] Failed to save new session: %v", err)
	}
	
	// Send UI update
	if sm.uiUpdateChan != nil {
		sm.uiUpdateChan <- SessionCreatedMsg{Session: session}
	}
	
	log.Printf("[SESSION] Created session %s for user %s", sessionID, userID)
	return session, nil
}

// LoadSession loads a session by ID
func (sm *SessionManager) LoadSession(sessionID string) (*Session, error) {
	sm.sessionsMutex.RLock()
	
	// Check if session is already active
	if session, exists := sm.activeSessions[sessionID]; exists {
		session.LastAccessedAt = time.Now()
		sm.sessionsMutex.RUnlock()
		return session, nil
	}
	sm.sessionsMutex.RUnlock()
	
	// Load from storage
	session, err := sm.storageProvider.LoadSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %v", err)
	}
	
	// Add to active sessions
	sm.sessionsMutex.Lock()
	session.IsActive = true
	session.LastAccessedAt = time.Now()
	sm.activeSessions[sessionID] = session
	sm.activeSessionCount++
	sm.sessionsMutex.Unlock()
	
	// Send UI update
	if sm.uiUpdateChan != nil {
		sm.uiUpdateChan <- SessionLoadedMsg{Session: session}
	}
	
	log.Printf("[SESSION] Loaded session %s", sessionID)
	return session, nil
}

// SaveSession saves a session
func (sm *SessionManager) SaveSession(sessionID string) error {
	sm.sessionsMutex.RLock()
	session, exists := sm.activeSessions[sessionID]
	sm.sessionsMutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	
	return sm.saveSession(session)
}

// saveSession saves a session to storage
func (sm *SessionManager) saveSession(session *Session) error {
	session.mutex.Lock()
	session.UpdatedAt = time.Now()
	session.IsPersisted = true
	session.lastSaved = time.Now()
	session.isDirty = false
	session.mutex.Unlock()
	
	if err := sm.storageProvider.SaveSession(session); err != nil {
		return fmt.Errorf("failed to save session: %v", err)
	}
	
	// Send UI update
	if sm.uiUpdateChan != nil {
		sm.uiUpdateChan <- SessionSavedMsg{SessionID: session.ID}
	}
	
	return nil
}

// DeleteSession deletes a session
func (sm *SessionManager) DeleteSession(sessionID string) error {
	sm.sessionsMutex.Lock()
	defer sm.sessionsMutex.Unlock()
	
	// Remove from active sessions
	if session, exists := sm.activeSessions[sessionID]; exists {
		session.IsActive = false
		delete(sm.activeSessions, sessionID)
		sm.activeSessionCount--
	}
	
	// Delete from storage
	if err := sm.storageProvider.DeleteSession(sessionID); err != nil {
		return fmt.Errorf("failed to delete session: %v", err)
	}
	
	// Send UI update
	if sm.uiUpdateChan != nil {
		sm.uiUpdateChan <- SessionDeletedMsg{SessionID: sessionID}
	}
	
	log.Printf("[SESSION] Deleted session %s", sessionID)
	return nil
}

// ListSessions lists all sessions for a user
func (sm *SessionManager) ListSessions(userID string) ([]*Session, error) {
	return sm.storageProvider.ListSessions(userID)
}

// AddMessage adds a message to a session
func (sm *SessionManager) AddMessage(sessionID string, message SessionMessage) error {
	sm.sessionsMutex.RLock()
	session, exists := sm.activeSessions[sessionID]
	sm.sessionsMutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	
	// Generate message ID if not provided
	if message.ID == "" {
		messageID, err := generateMessageID()
		if err != nil {
			return fmt.Errorf("failed to generate message ID: %v", err)
		}
		message.ID = messageID
	}
	
	// Set timestamp
	message.Timestamp = time.Now()
	
	// Add to session
	session.mutex.Lock()
	session.Messages = append(session.Messages, message)
	session.MessageCount++
	session.LastAccessedAt = time.Now()
	session.isDirty = true
	session.mutex.Unlock()
	
	// Send update
	sm.sessionUpdateChan <- SessionUpdate{
		SessionID:  sessionID,
		UpdateType: "message",
		Data:       message,
		Timestamp:  time.Now(),
	}
	
	return nil
}

// UpdateContext updates session context
func (sm *SessionManager) UpdateContext(sessionID string, key string, value interface{}) error {
	sm.sessionsMutex.RLock()
	session, exists := sm.activeSessions[sessionID]
	sm.sessionsMutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	
	session.mutex.Lock()
	session.Context[key] = value
	session.LastAccessedAt = time.Now()
	session.isDirty = true
	session.mutex.Unlock()
	
	// Send update
	sm.sessionUpdateChan <- SessionUpdate{
		SessionID:  sessionID,
		UpdateType: "context",
		Data:       map[string]interface{}{key: value},
		Timestamp:  time.Now(),
	}
	
	return nil
}

// GetActiveSessionCount returns the number of active sessions
func (sm *SessionManager) GetActiveSessionCount() int64 {
	sm.sessionsMutex.RLock()
	defer sm.sessionsMutex.RUnlock()
	return sm.activeSessionCount
}

// GetSessionMetrics returns session metrics
func (sm *SessionManager) GetSessionMetrics() SessionMetrics {
	sm.sessionsMutex.RLock()
	defer sm.sessionsMutex.RUnlock()
	
	return SessionMetrics{
		TotalSessions:   sm.totalSessions,
		ActiveSessions:  sm.activeSessionCount,
		StorageType:     sm.config.StorageType,
		StorageLocation: sm.config.StorageLocation,
	}
}

// updateProcessor processes session updates
func (sm *SessionManager) updateProcessor() {
	for {
		select {
		case <-sm.ctx.Done():
			return
		case update := <-sm.sessionUpdateChan:
			sm.handleSessionUpdate(update)
		}
	}
}

// handleSessionUpdate handles a session update
func (sm *SessionManager) handleSessionUpdate(update SessionUpdate) {
	if sm.uiUpdateChan != nil {
		sm.uiUpdateChan <- SessionUpdateMsg{Update: update}
	}
}

// autoSaveProcessor automatically saves dirty sessions
func (sm *SessionManager) autoSaveProcessor() {
	if !sm.config.AutoSave {
		return
	}
	
	ticker := time.NewTicker(sm.config.SaveInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-ticker.C:
			sm.autoSaveSessions()
		}
	}
}

// autoSaveSessions automatically saves all dirty sessions
func (sm *SessionManager) autoSaveSessions() {
	sm.sessionsMutex.RLock()
	sessionsToSave := make([]*Session, 0)
	
	for _, session := range sm.activeSessions {
		session.mutex.RLock()
		if session.isDirty {
			sessionsToSave = append(sessionsToSave, session)
		}
		session.mutex.RUnlock()
	}
	sm.sessionsMutex.RUnlock()
	
	// Save dirty sessions
	for _, session := range sessionsToSave {
		if err := sm.saveSession(session); err != nil {
			log.Printf("[SESSION] Failed to auto-save session %s: %v", session.ID, err)
		}
	}
	
	if len(sessionsToSave) > 0 {
		log.Printf("[SESSION] Auto-saved %d sessions", len(sessionsToSave))
	}
}

// cleanupProcessor performs periodic cleanup
func (sm *SessionManager) cleanupProcessor() {
	sm.cleanupTicker = time.NewTicker(sm.cleanupInterval)
	defer sm.cleanupTicker.Stop()
	
	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-sm.cleanupTicker.C:
			sm.performCleanup()
		}
	}
}

// performCleanup performs session cleanup
func (sm *SessionManager) performCleanup() {
	sm.sessionsMutex.Lock()
	defer sm.sessionsMutex.Unlock()
	
	now := time.Now()
	expiredSessions := make([]string, 0)
	
	// Check for expired sessions
	for sessionID, session := range sm.activeSessions {
		session.mutex.RLock()
		
		// Check if session is expired
		if now.After(session.ExpiresAt) {
			expiredSessions = append(expiredSessions, sessionID)
		} else if now.Sub(session.LastAccessedAt) > sm.config.SessionTimeout {
			// Session is inactive
			session.IsActive = false
			if err := sm.saveSession(session); err != nil {
				log.Printf("[SESSION] Failed to save inactive session %s: %v", sessionID, err)
			}
			delete(sm.activeSessions, sessionID)
			sm.activeSessionCount--
		}
		
		session.mutex.RUnlock()
	}
	
	// Remove expired sessions
	for _, sessionID := range expiredSessions {
		delete(sm.activeSessions, sessionID)
		sm.activeSessionCount--
		
		if err := sm.storageProvider.DeleteSession(sessionID); err != nil {
			log.Printf("[SESSION] Failed to delete expired session %s: %v", sessionID, err)
		}
	}
	
	if len(expiredSessions) > 0 {
		log.Printf("[SESSION] Cleaned up %d expired sessions", len(expiredSessions))
	}
}

// Stop gracefully stops the session manager
func (sm *SessionManager) Stop() error {
	sm.sessionsMutex.Lock()
	defer sm.sessionsMutex.Unlock()
	
	if !sm.isActive {
		return nil
	}
	
	// Save all active sessions
	for _, session := range sm.activeSessions {
		if err := sm.saveSession(session); err != nil {
			log.Printf("[SESSION] Failed to save session %s during shutdown: %v", session.ID, err)
		}
	}
	
	// Cancel context
	sm.cancel()
	
	// Stop cleanup ticker
	if sm.cleanupTicker != nil {
		sm.cleanupTicker.Stop()
	}
	
	sm.isActive = false
	
	log.Printf("[SESSION] Session manager stopped. Saved %d sessions", len(sm.activeSessions))
	return nil
}

// generateSessionID generates a unique session ID
func generateSessionID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// generateMessageID generates a unique message ID
func generateMessageID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// SessionMetrics contains session metrics
type SessionMetrics struct {
	TotalSessions   int64  `json:"total_sessions"`
	ActiveSessions  int64  `json:"active_sessions"`
	StorageType     string `json:"storage_type"`
	StorageLocation string `json:"storage_location"`
}

// UI Messages
type SessionCreatedMsg struct {
	Session *Session
}

type SessionLoadedMsg struct {
	Session *Session
}

type SessionSavedMsg struct {
	SessionID string
}

type SessionDeletedMsg struct {
	SessionID string
}

type SessionUpdateMsg struct {
	Update SessionUpdate
}