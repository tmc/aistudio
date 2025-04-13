package aistudio

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ChatSession represents a complete chat session with messages and metadata
type ChatSession struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Messages    []Message `json:"messages"`
	ModelName   string    `json:"model_name,omitempty"`
}

// HistoryManager handles chat history storage and retrieval
type HistoryManager struct {
	HistoryDir     string         // Directory where history files are stored
	CurrentSession *ChatSession   // Currently active session
	Sessions       []*ChatSession // List of available sessions
}

// Message types for history operations
type historyLoadedMsg struct {
	session *ChatSession
}

type historyLoadFailedMsg struct {
	err error
}

type historySavedMsg struct{}

type historySaveFailedMsg struct {
	err error
}

// NewHistoryManager creates a new history manager
func NewHistoryManager(historyDir string) (*HistoryManager, error) {
	// Ensure history directory exists
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create history directory: %w", err)
	}

	return &HistoryManager{
		HistoryDir: historyDir,
		Sessions:   make([]*ChatSession, 0),
	}, nil
}

// LoadSessions loads available sessions from the history directory
func (hm *HistoryManager) LoadSessions() error {
	files, err := ioutil.ReadDir(hm.HistoryDir)
	if err != nil {
		return fmt.Errorf("failed to read history directory: %w", err)
	}

	hm.Sessions = make([]*ChatSession, 0)
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(hm.HistoryDir, file.Name())
		session, err := hm.LoadSessionFromFile(filePath)
		if err != nil {
			log.Printf("Error loading session from %s: %v", filePath, err)
			continue
		}

		hm.Sessions = append(hm.Sessions, session)
	}

	return nil
}

// LoadSessionFromFile loads a chat session from a file
func (hm *HistoryManager) LoadSessionFromFile(filePath string) (*ChatSession, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session ChatSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session file: %w", err)
	}

	return &session, nil
}

// SaveSession saves the current session to a file
func (hm *HistoryManager) SaveSession(session *ChatSession) error {
	if session == nil {
		return fmt.Errorf("no session to save")
	}

	// Update timestamps
	session.UpdatedAt = time.Now()

	// Convert to JSON
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize session: %w", err)
	}

	// Create filename from ID
	fileName := fmt.Sprintf("%s.json", session.ID)
	filePath := filepath.Join(hm.HistoryDir, fileName)

	// Write to file
	if err := ioutil.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// NewSession creates a new chat session
func (hm *HistoryManager) NewSession(title, modelName string) *ChatSession {
	session := &ChatSession{
		ID:        fmt.Sprintf("session_%d", time.Now().Unix()),
		Title:     title,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  make([]Message, 0),
		ModelName: modelName,
	}

	hm.CurrentSession = session
	return session
}

// AddMessage adds a message to the current session
func (hm *HistoryManager) AddMessage(message Message) {
	if hm.CurrentSession == nil {
		// Create a new default session if none exists
		hm.NewSession("Untitled Chat", "")
	}

	hm.CurrentSession.Messages = append(hm.CurrentSession.Messages, message)
	hm.CurrentSession.UpdatedAt = time.Now()
}

// loadSessionCmd returns a command to load a session in the background
func (m *Model) loadSessionCmd(filePath string) tea.Cmd {
	return func() tea.Msg {
		if m.historyManager == nil {
			return historyLoadFailedMsg{err: fmt.Errorf("history manager not initialized")}
		}

		session, err := m.historyManager.LoadSessionFromFile(filePath)
		if err != nil {
			return historyLoadFailedMsg{err: err}
		}

		return historyLoadedMsg{session: session}
	}
}

// saveSessionCmd returns a command to save the current session
func (m *Model) saveSessionCmd() tea.Cmd {
	return func() tea.Msg {
		if m.historyManager == nil || m.historyManager.CurrentSession == nil {
			return historySaveFailedMsg{err: fmt.Errorf("no active session to save")}
		}

		// Update the session with current messages
		m.historyManager.CurrentSession.Messages = m.messages
		m.historyManager.CurrentSession.UpdatedAt = time.Now()

		// Save the session
		err := m.historyManager.SaveSession(m.historyManager.CurrentSession)
		if err != nil {
			return historySaveFailedMsg{err: err}
		}

		return historySavedMsg{}
	}
}

// loadMessagesFromSession loads messages from a session into the model
func (m *Model) loadMessagesFromSession(session *ChatSession) {
	m.messages = session.Messages
	if session.ModelName != "" && session.ModelName != m.modelName {
		m.modelName = session.ModelName
	}
	m.viewport.SetContent(m.formatAllMessages())
	m.viewport.GotoBottom()
}
