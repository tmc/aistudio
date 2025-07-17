package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestSessionManager tests the session manager functionality
func TestSessionManager(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "session_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create test configuration
	config := DefaultSessionConfig()
	config.StorageLocation = tempDir
	config.AutoSave = false // Disable auto-save for testing
	
	// Create UI update channel
	uiUpdateChan := make(chan tea.Msg, 100)
	
	// Create session manager
	manager := NewSessionManager(config, uiUpdateChan)
	
	// Test creating a session
	session, err := manager.CreateSession("test_user", "Test Session", "A test session")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	
	if session.ID == "" {
		t.Error("Session ID should not be empty")
	}
	
	if session.UserID != "test_user" {
		t.Errorf("Expected UserID to be 'test_user', got '%s'", session.UserID)
	}
	
	if session.Name != "Test Session" {
		t.Errorf("Expected Name to be 'Test Session', got '%s'", session.Name)
	}
	
	// Test loading a session
	loadedSession, err := manager.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to load session: %v", err)
	}
	
	if loadedSession.ID != session.ID {
		t.Errorf("Expected loaded session ID to be '%s', got '%s'", session.ID, loadedSession.ID)
	}
	
	// Test adding a message
	message := SessionMessage{
		Role:        "user",
		Content:     "Hello, world!",
		MessageType: "text",
		IsComplete:  true,
	}
	
	err = manager.AddMessage(session.ID, message)
	if err != nil {
		t.Fatalf("Failed to add message: %v", err)
	}
	
	// Verify message was added
	updatedSession, err := manager.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to load updated session: %v", err)
	}
	
	if len(updatedSession.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(updatedSession.Messages))
	}
	
	if updatedSession.Messages[0].Content != "Hello, world!" {
		t.Errorf("Expected message content to be 'Hello, world!', got '%s'", updatedSession.Messages[0].Content)
	}
	
	// Test updating context
	err = manager.UpdateContext(session.ID, "test_key", "test_value")
	if err != nil {
		t.Fatalf("Failed to update context: %v", err)
	}
	
	// Verify context was updated
	updatedSession, err = manager.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to load updated session: %v", err)
	}
	
	if updatedSession.Context["test_key"] != "test_value" {
		t.Errorf("Expected context value to be 'test_value', got '%v'", updatedSession.Context["test_key"])
	}
	
	// Test saving session
	err = manager.SaveSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to save session: %v", err)
	}
	
	// Test listing sessions
	sessions, err := manager.ListSessions("test_user")
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}
	
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}
	
	// Test deleting session
	err = manager.DeleteSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}
	
	// Verify session was deleted
	sessions, err = manager.ListSessions("test_user")
	if err != nil {
		t.Fatalf("Failed to list sessions after deletion: %v", err)
	}
	
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions after deletion, got %d", len(sessions))
	}
	
	// Test metrics
	metrics := manager.GetSessionMetrics()
	if metrics.TotalSessions != 1 {
		t.Errorf("Expected total sessions to be 1, got %d", metrics.TotalSessions)
	}
	
	// Stop manager
	err = manager.Stop()
	if err != nil {
		t.Fatalf("Failed to stop session manager: %v", err)
	}
}

// TestFileStorageProvider tests the file storage provider
func TestFileStorageProvider(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "storage_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create storage provider
	provider, err := NewFileStorageProvider(tempDir)
	if err != nil {
		t.Fatalf("Failed to create file storage provider: %v", err)
	}
	
	// Create test session
	session := &Session{
		ID:          "test_session_123",
		UserID:      "test_user",
		Name:        "Test Session",
		Description: "A test session",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Messages:    []SessionMessage{},
		Context:     make(map[string]interface{}),
		IsActive:    true,
		Version:     1,
	}
	
	// Test saving session
	err = provider.SaveSession(session)
	if err != nil {
		t.Fatalf("Failed to save session: %v", err)
	}
	
	// Verify file was created
	expectedPath := filepath.Join(tempDir, "test_user", "test_session_123.json.gz")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected session file to be created at %s", expectedPath)
	}
	
	// Test loading session
	loadedSession, err := provider.LoadSession("test_session_123")
	if err != nil {
		t.Fatalf("Failed to load session: %v", err)
	}
	
	if loadedSession.ID != session.ID {
		t.Errorf("Expected loaded session ID to be '%s', got '%s'", session.ID, loadedSession.ID)
	}
	
	if loadedSession.Name != session.Name {
		t.Errorf("Expected loaded session name to be '%s', got '%s'", session.Name, loadedSession.Name)
	}
	
	// Test listing sessions
	sessions, err := provider.ListSessions("test_user")
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}
	
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}
	
	// Test backup
	err = provider.BackupSession("test_session_123")
	if err != nil {
		t.Fatalf("Failed to backup session: %v", err)
	}
	
	// Verify backup was created
	backupDir := filepath.Join(tempDir, ".backups", "test_session_123")
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		t.Errorf("Expected backup directory to be created at %s", backupDir)
	}
	
	// Test deleting session
	err = provider.DeleteSession("test_session_123")
	if err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}
	
	// Verify file was deleted
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Errorf("Expected session file to be deleted")
	}
	
	// Test SessionStore interface
	store := SessionStore(provider)
	
	// Test saving through interface
	err = store.Save(session)
	if err != nil {
		t.Fatalf("Failed to save session through store interface: %v", err)
	}
	
	// Test loading through interface
	loadedSession2, err := store.Load("test_session_123")
	if err != nil {
		t.Fatalf("Failed to load session through store interface: %v", err)
	}
	
	if loadedSession2.ID != session.ID {
		t.Errorf("Expected loaded session ID to be '%s', got '%s'", session.ID, loadedSession2.ID)
	}
	
	// Test existence check
	if !store.Exists("test_session_123") {
		t.Error("Expected session to exist")
	}
	
	// Test listing through interface
	sessionIDs, err := store.List("test_user")
	if err != nil {
		t.Fatalf("Failed to list sessions through store interface: %v", err)
	}
	
	if len(sessionIDs) != 1 {
		t.Errorf("Expected 1 session ID, got %d", len(sessionIDs))
	}
	
	// Test deleting through interface
	err = store.Delete("test_session_123")
	if err != nil {
		t.Fatalf("Failed to delete session through store interface: %v", err)
	}
	
	if store.Exists("test_session_123") {
		t.Error("Expected session to not exist after deletion")
	}
}

// TestMemoryStorageProvider tests the memory storage provider
func TestMemoryStorageProvider(t *testing.T) {
	// Create storage provider
	provider := NewMemoryStorageProvider()
	
	// Create test session
	session := &Session{
		ID:          "test_session_123",
		UserID:      "test_user",
		Name:        "Test Session",
		Description: "A test session",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Messages:    []SessionMessage{},
		Context:     make(map[string]interface{}),
		IsActive:    true,
		Version:     1,
	}
	
	// Test saving session
	err := provider.SaveSession(session)
	if err != nil {
		t.Fatalf("Failed to save session: %v", err)
	}
	
	// Test loading session
	loadedSession, err := provider.LoadSession("test_session_123")
	if err != nil {
		t.Fatalf("Failed to load session: %v", err)
	}
	
	if loadedSession.ID != session.ID {
		t.Errorf("Expected loaded session ID to be '%s', got '%s'", session.ID, loadedSession.ID)
	}
	
	// Test listing sessions
	sessions, err := provider.ListSessions("test_user")
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}
	
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}
	
	// Test deleting session
	err = provider.DeleteSession("test_session_123")
	if err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}
	
	// Verify session was deleted
	sessions, err = provider.ListSessions("test_user")
	if err != nil {
		t.Fatalf("Failed to list sessions after deletion: %v", err)
	}
	
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions after deletion, got %d", len(sessions))
	}
	
	// Test SessionStore interface
	store := SessionStore(provider)
	
	// Test existence check
	if store.Exists("test_session_123") {
		t.Error("Expected session to not exist")
	}
	
	// Test saving through interface
	err = store.Save(session)
	if err != nil {
		t.Fatalf("Failed to save session through store interface: %v", err)
	}
	
	if !store.Exists("test_session_123") {
		t.Error("Expected session to exist after saving")
	}
}

// TestSessionExporter tests the session exporter
func TestSessionExporter(t *testing.T) {
	// Create session manager
	config := DefaultSessionConfig()
	config.StorageType = "memory"
	uiUpdateChan := make(chan tea.Msg, 100)
	manager := NewSessionManager(config, uiUpdateChan)
	
	// Create test session
	session, err := manager.CreateSession("test_user", "Test Session", "A test session")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	
	// Add test message
	message := SessionMessage{
		Role:        "user",
		Content:     "Hello, world!",
		MessageType: "text",
		IsComplete:  true,
	}
	
	err = manager.AddMessage(session.ID, message)
	if err != nil {
		t.Fatalf("Failed to add message: %v", err)
	}
	
	// Create exporter
	exporter := NewSessionExporter(manager)
	
	// Test JSON export
	jsonData, err := exporter.ExportSession(session.ID, "json")
	if err != nil {
		t.Fatalf("Failed to export session as JSON: %v", err)
	}
	
	if len(jsonData) == 0 {
		t.Error("Expected JSON export to have data")
	}
	
	// Test Markdown export
	markdownData, err := exporter.ExportSession(session.ID, "markdown")
	if err != nil {
		t.Fatalf("Failed to export session as Markdown: %v", err)
	}
	
	if len(markdownData) == 0 {
		t.Error("Expected Markdown export to have data")
	}
	
	// Test text export
	textData, err := exporter.ExportSession(session.ID, "text")
	if err != nil {
		t.Fatalf("Failed to export session as text: %v", err)
	}
	
	if len(textData) == 0 {
		t.Error("Expected text export to have data")
	}
	
	// Test invalid format
	_, err = exporter.ExportSession(session.ID, "invalid")
	if err == nil {
		t.Error("Expected error for invalid export format")
	}
}

// TestSessionImporter tests the session importer
func TestSessionImporter(t *testing.T) {
	// Create session manager
	config := DefaultSessionConfig()
	config.StorageType = "memory"
	uiUpdateChan := make(chan tea.Msg, 100)
	manager := NewSessionManager(config, uiUpdateChan)
	
	// Create test session
	originalSession, err := manager.CreateSession("test_user", "Test Session", "A test session")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	
	// Add test message
	message := SessionMessage{
		Role:        "user",
		Content:     "Hello, world!",
		MessageType: "text",
		IsComplete:  true,
	}
	
	err = manager.AddMessage(originalSession.ID, message)
	if err != nil {
		t.Fatalf("Failed to add message: %v", err)
	}
	
	// Export session
	exporter := NewSessionExporter(manager)
	jsonData, err := exporter.ExportSession(originalSession.ID, "json")
	if err != nil {
		t.Fatalf("Failed to export session: %v", err)
	}
	
	// Create importer
	importer := NewSessionImporter(manager)
	
	// Test JSON import
	importedSession, err := importer.ImportSession(jsonData, "json", "test_user_2")
	if err != nil {
		t.Fatalf("Failed to import session: %v", err)
	}
	
	if importedSession.ID == originalSession.ID {
		t.Error("Expected imported session to have different ID")
	}
	
	if importedSession.UserID != "test_user_2" {
		t.Errorf("Expected imported session user ID to be 'test_user_2', got '%s'", importedSession.UserID)
	}
	
	if importedSession.Name != originalSession.Name {
		t.Errorf("Expected imported session name to be '%s', got '%s'", originalSession.Name, importedSession.Name)
	}
	
	if len(importedSession.Messages) != 1 {
		t.Errorf("Expected 1 message in imported session, got %d", len(importedSession.Messages))
	}
	
	// Test invalid format
	_, err = importer.ImportSession(jsonData, "invalid", "test_user")
	if err == nil {
		t.Error("Expected error for invalid import format")
	}
}

// TestSessionAnalyzer tests the session analyzer
func TestSessionAnalyzer(t *testing.T) {
	// Create session manager
	config := DefaultSessionConfig()
	config.StorageType = "memory"
	uiUpdateChan := make(chan tea.Msg, 100)
	manager := NewSessionManager(config, uiUpdateChan)
	
	// Create test session
	session, err := manager.CreateSession("test_user", "Test Session", "A test session")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	
	// Add test messages
	messages := []SessionMessage{
		{
			Role:        "user",
			Content:     "Hello, I need help with code!",
			MessageType: "text",
			IsComplete:  true,
		},
		{
			Role:        "assistant",
			Content:     "I'd be happy to help you with code!",
			MessageType: "text",
			IsComplete:  true,
		},
		{
			Role:        "user",
			Content:     "Can you analyze this data?",
			MessageType: "text",
			IsComplete:  true,
		},
	}
	
	for _, message := range messages {
		err = manager.AddMessage(session.ID, message)
		if err != nil {
			t.Fatalf("Failed to add message: %v", err)
		}
	}
	
	// Create analyzer
	analyzer := NewSessionAnalyzer(manager)
	
	// Test analyzing session
	analysis, err := analyzer.AnalyzeSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to analyze session: %v", err)
	}
	
	if analysis.SessionID != session.ID {
		t.Errorf("Expected analysis session ID to be '%s', got '%s'", session.ID, analysis.SessionID)
	}
	
	if analysis.MessageCount != 3 {
		t.Errorf("Expected message count to be 3, got %d", analysis.MessageCount)
	}
	
	if analysis.UserMessages != 2 {
		t.Errorf("Expected user messages to be 2, got %d", analysis.UserMessages)
	}
	
	if analysis.AssistantMessages != 1 {
		t.Errorf("Expected assistant messages to be 1, got %d", analysis.AssistantMessages)
	}
	
	if len(analysis.Keywords) == 0 {
		t.Error("Expected keywords to be extracted")
	}
	
	if len(analysis.Topics) == 0 {
		t.Error("Expected topics to be extracted")
	}
}

// TestSessionValidator tests the session validator
func TestSessionValidator(t *testing.T) {
	validator := NewSessionValidator()
	
	// Test valid session
	validSession := &Session{
		ID:        "test_session_123",
		UserID:    "test_user",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Messages: []SessionMessage{
			{
				ID:        "msg_1",
				Role:      "user",
				Content:   "Hello",
				Timestamp: time.Now(),
			},
		},
	}
	
	errors := validator.ValidateSession(validSession)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors for valid session, got %d", len(errors))
	}
	
	// Test invalid session
	invalidSession := &Session{
		ID:        "", // Missing ID
		UserID:    "", // Missing UserID
		CreatedAt: time.Now(),
		UpdatedAt: time.Now().Add(-1 * time.Hour), // UpdatedAt before CreatedAt
		ExpiresAt: time.Now().Add(-1 * time.Hour), // ExpiresAt before CreatedAt
		Messages: []SessionMessage{
			{
				ID:        "", // Missing message ID
				Role:      "", // Missing role
				Content:   "Hello",
				Timestamp: time.Time{}, // Missing timestamp
			},
		},
	}
	
	errors = validator.ValidateSession(invalidSession)
	if len(errors) == 0 {
		t.Error("Expected validation errors for invalid session")
	}
	
	// Check specific errors
	expectedErrors := []string{"ID", "UserID", "UpdatedAt", "ExpiresAt", "Messages[0].ID", "Messages[0].Role", "Messages[0].Timestamp"}
	for _, expectedError := range expectedErrors {
		found := false
		for _, error := range errors {
			if error.Field == expectedError {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected validation error for field '%s'", expectedError)
		}
	}
}

// TestSessionConcurrency tests session manager concurrency
func TestSessionConcurrency(t *testing.T) {
	// Create session manager
	config := DefaultSessionConfig()
	config.StorageType = "memory"
	uiUpdateChan := make(chan tea.Msg, 100)
	manager := NewSessionManager(config, uiUpdateChan)
	
	// Create test session
	session, err := manager.CreateSession("test_user", "Test Session", "A test session")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	
	// Test concurrent message additions
	messageCount := 100
	done := make(chan bool, messageCount)
	
	for i := 0; i < messageCount; i++ {
		go func(i int) {
			message := SessionMessage{
				Role:        "user",
				Content:     fmt.Sprintf("Message %d", i),
				MessageType: "text",
				IsComplete:  true,
			}
			
			err := manager.AddMessage(session.ID, message)
			if err != nil {
				t.Errorf("Failed to add message %d: %v", i, err)
			}
			
			done <- true
		}(i)
	}
	
	// Wait for all messages to be added
	for i := 0; i < messageCount; i++ {
		<-done
	}
	
	// Verify all messages were added
	loadedSession, err := manager.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to load session: %v", err)
	}
	
	if len(loadedSession.Messages) != messageCount {
		t.Errorf("Expected %d messages, got %d", messageCount, len(loadedSession.Messages))
	}
}