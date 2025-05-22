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

	// Ensure we're creating a deep copy to avoid reference issues
	messageCopy := message

	// Zero out runtime-specific flags that shouldn't be persisted
	messageCopy.IsPlaying = false

	// Append the message to the current session
	hm.CurrentSession.Messages = append(hm.CurrentSession.Messages, messageCopy)
	hm.CurrentSession.UpdatedAt = time.Now()
}

// saveSessionCmd returns a command to save the current session
func (m *Model) saveSessionCmd() tea.Cmd {
	return func() tea.Msg {
		if m.historyManager == nil || m.historyManager.CurrentSession == nil {
			return historySaveFailedMsg{err: fmt.Errorf("no active session to save")}
		}

		// Make a clean copy of the messages for persistence
		persistedMessages := make([]Message, len(m.messages))
		for i, msg := range m.messages {
			// Create a deep copy of each message
			msgCopy := msg

			// Reset runtime state flags that shouldn't be persisted
			msgCopy.IsPlaying = false
			msgCopy.IsPlayed = msg.IsPlayed // We want to keep this flag for UI purposes

			// Make deep copies of pointer fields to avoid reference issues
			if msg.ToolCall != nil {
				toolCallCopy := *msg.ToolCall
				msgCopy.ToolCall = &toolCallCopy
			}
			if msg.ToolResponse != nil {
				toolResponseCopy := *msg.ToolResponse
				msgCopy.ToolResponse = &toolResponseCopy
			}
			if msg.ExecutableCode != nil {
				execCodeCopy := *msg.ExecutableCode
				msgCopy.ExecutableCode = &execCodeCopy
			}

			// Audio data is large, so we store only a small hint if it exists
			if len(msg.AudioData) > 1024 && msgCopy.HasAudio {
				// Store just a hint that there was audio (first 1K)
				msgCopy.AudioData = msg.AudioData[:1024]
			}

			// Add the message to the persist array
			persistedMessages[i] = msgCopy
		}

		// Update the session with processed messages
		m.historyManager.CurrentSession.Messages = persistedMessages
		m.historyManager.CurrentSession.UpdatedAt = time.Now()
		m.historyManager.CurrentSession.ModelName = m.modelName

		// Save the session
		err := m.historyManager.SaveSession(m.historyManager.CurrentSession)
		if err != nil {
			return historySaveFailedMsg{err: err}
		}

		log.Printf("Saved %d messages to session %s", len(persistedMessages), m.historyManager.CurrentSession.ID)
		return historySavedMsg{}
	}
}

// loadMessagesFromSession loads messages from a session into the model
func (m *Model) loadMessagesFromSession(session *ChatSession) {
	// Create a copy of the messages to avoid modifying the original session
	m.messages = make([]Message, len(session.Messages))
	for i, msg := range session.Messages {
		// Create a deep copy of each message
		messageCopy := msg

		// Ensure pointers to complex types are also copied properly
		if msg.ToolCall != nil {
			toolCallCopy := *msg.ToolCall
			messageCopy.ToolCall = &toolCallCopy
		}
		if msg.ToolResponse != nil {
			toolResponseCopy := *msg.ToolResponse
			messageCopy.ToolResponse = &toolResponseCopy
		}
		if msg.ExecutableCode != nil {
			execCodeCopy := *msg.ExecutableCode
			messageCopy.ExecutableCode = &execCodeCopy
		}

		// Add the copied message to the model
		m.messages[i] = messageCopy
	}

	// Update model configuration if needed
	if session.ModelName != "" && session.ModelName != m.modelName {
		m.modelName = session.ModelName
	}

	// Update the viewport content
	formattedContent := m.renderAllMessages()
	m.viewport.SetContent(formattedContent)

	// Reset viewport position to allow scrolling from the beginning
	m.viewport.GotoTop()

	// Log the number of loaded messages
	log.Printf("Loaded %d messages from session %s", len(m.messages), session.ID)
}
