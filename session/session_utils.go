package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SessionExporter provides session export functionality
type SessionExporter struct {
	manager *SessionManager
}

// NewSessionExporter creates a new session exporter
func NewSessionExporter(manager *SessionManager) *SessionExporter {
	return &SessionExporter{
		manager: manager,
	}
}

// ExportSession exports a session to various formats
func (se *SessionExporter) ExportSession(sessionID string, format string) ([]byte, error) {
	// Load session
	session, err := se.manager.LoadSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %v", err)
	}
	
	switch format {
	case "json":
		return se.exportAsJSON(session)
	case "markdown":
		return se.exportAsMarkdown(session)
	case "text":
		return se.exportAsText(session)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// exportAsJSON exports session as JSON
func (se *SessionExporter) exportAsJSON(session *Session) ([]byte, error) {
	// Create export structure
	export := struct {
		Session   *Session  `json:"session"`
		ExportedAt time.Time `json:"exported_at"`
		Version   string    `json:"version"`
	}{
		Session:   session,
		ExportedAt: time.Now(),
		Version:   "1.0",
	}
	
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %v", err)
	}
	
	return data, nil
}

// exportAsMarkdown exports session as Markdown
func (se *SessionExporter) exportAsMarkdown(session *Session) ([]byte, error) {
	var output strings.Builder
	
	// Header
	output.WriteString(fmt.Sprintf("# %s\n\n", session.Name))
	output.WriteString(fmt.Sprintf("**Description:** %s\n\n", session.Description))
	output.WriteString(fmt.Sprintf("**Created:** %s\n", session.CreatedAt.Format("2006-01-02 15:04:05")))
	output.WriteString(fmt.Sprintf("**Updated:** %s\n", session.UpdatedAt.Format("2006-01-02 15:04:05")))
	output.WriteString(fmt.Sprintf("**Messages:** %d\n\n", len(session.Messages)))
	
	// Messages
	output.WriteString("## Messages\n\n")
	for _, message := range session.Messages {
		output.WriteString(fmt.Sprintf("### %s (%s)\n", 
			capitalizeFirst(message.Role), 
			message.Timestamp.Format("2006-01-02 15:04:05")))
		output.WriteString(fmt.Sprintf("*%s*\n\n", message.MessageType))
		output.WriteString(message.Content)
		output.WriteString("\n\n")
		
		// Add tool calls if present
		if len(message.ToolCalls) > 0 {
			output.WriteString("**Tool Calls:**\n")
			for _, toolCall := range message.ToolCalls {
				output.WriteString(fmt.Sprintf("- %s\n", toolCall.ToolName))
			}
			output.WriteString("\n")
		}
		
		// Add tool results if present
		if len(message.ToolResults) > 0 {
			output.WriteString("**Tool Results:**\n")
			for _, result := range message.ToolResults {
				status := "✓"
				if !result.Success {
					status = "✗"
				}
				output.WriteString(fmt.Sprintf("- %s %s\n", status, result.ToolCallID))
			}
			output.WriteString("\n")
		}
		
		output.WriteString("---\n\n")
	}
	
	// Attachments
	if len(session.Attachments) > 0 {
		output.WriteString("## Attachments\n\n")
		for _, attachment := range session.Attachments {
			output.WriteString(fmt.Sprintf("- **%s** (%s, %s)\n", 
				attachment.FileName, 
				attachment.MimeType, 
				formatBytes(attachment.Size)))
		}
		output.WriteString("\n")
	}
	
	// Generated Content
	if len(session.GeneratedContent) > 0 {
		output.WriteString("## Generated Content\n\n")
		for _, content := range session.GeneratedContent {
			output.WriteString(fmt.Sprintf("- **%s** (%s) - %s\n", 
				content.Type, 
				content.ModelUsed, 
				content.GeneratedAt.Format("2006-01-02 15:04:05")))
		}
		output.WriteString("\n")
	}
	
	// Tags
	if len(session.Tags) > 0 {
		output.WriteString("## Tags\n\n")
		for _, tag := range session.Tags {
			output.WriteString(fmt.Sprintf("- %s\n", tag))
		}
		output.WriteString("\n")
	}
	
	return []byte(output.String()), nil
}

// exportAsText exports session as plain text
func (se *SessionExporter) exportAsText(session *Session) ([]byte, error) {
	var output strings.Builder
	
	// Header
	output.WriteString(fmt.Sprintf("Session: %s\n", session.Name))
	output.WriteString(fmt.Sprintf("Description: %s\n", session.Description))
	output.WriteString(fmt.Sprintf("Created: %s\n", session.CreatedAt.Format("2006-01-02 15:04:05")))
	output.WriteString(fmt.Sprintf("Updated: %s\n", session.UpdatedAt.Format("2006-01-02 15:04:05")))
	output.WriteString(fmt.Sprintf("Messages: %d\n\n", len(session.Messages)))
	
	// Messages
	output.WriteString("Messages:\n")
	output.WriteString("========\n\n")
	
	for _, message := range session.Messages {
		output.WriteString(fmt.Sprintf("[%s] %s (%s)\n", 
			message.Timestamp.Format("15:04:05"), 
			capitalizeFirst(message.Role),
			message.MessageType))
		output.WriteString(message.Content)
		output.WriteString("\n\n")
		
		if len(message.ToolCalls) > 0 {
			output.WriteString("Tools used: ")
			for i, toolCall := range message.ToolCalls {
				if i > 0 {
					output.WriteString(", ")
				}
				output.WriteString(toolCall.ToolName)
			}
			output.WriteString("\n\n")
		}
		
		output.WriteString("---\n\n")
	}
	
	return []byte(output.String()), nil
}

// SessionImporter provides session import functionality
type SessionImporter struct {
	manager *SessionManager
}

// NewSessionImporter creates a new session importer
func NewSessionImporter(manager *SessionManager) *SessionImporter {
	return &SessionImporter{
		manager: manager,
	}
}

// ImportSession imports a session from various formats
func (si *SessionImporter) ImportSession(data []byte, format string, userID string) (*Session, error) {
	switch format {
	case "json":
		return si.importFromJSON(data, userID)
	default:
		return nil, fmt.Errorf("unsupported import format: %s", format)
	}
}

// importFromJSON imports session from JSON
func (si *SessionImporter) importFromJSON(data []byte, userID string) (*Session, error) {
	// Parse JSON
	var importData struct {
		Session   *Session  `json:"session"`
		ExportedAt time.Time `json:"exported_at"`
		Version   string    `json:"version"`
	}
	
	if err := json.Unmarshal(data, &importData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}
	
	if importData.Session == nil {
		return nil, fmt.Errorf("no session data found in import")
	}
	
	// Update session for import
	session := importData.Session
	session.UserID = userID
	session.CreatedAt = time.Now()
	session.UpdatedAt = time.Now()
	session.LastAccessedAt = time.Now()
	session.ExpiresAt = time.Now().Add(si.manager.config.MaxSessionAge)
	session.IsActive = true
	session.IsPersisted = false
	session.Version = 1
	
	// Generate new session ID
	newID, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate new session ID: %v", err)
	}
	session.ID = newID
	
	// Save session
	if err := si.manager.saveSession(session); err != nil {
		return nil, fmt.Errorf("failed to save imported session: %v", err)
	}
	
	return session, nil
}

// SessionAnalyzer provides session analysis functionality
type SessionAnalyzer struct {
	manager *SessionManager
}

// NewSessionAnalyzer creates a new session analyzer
func NewSessionAnalyzer(manager *SessionManager) *SessionAnalyzer {
	return &SessionAnalyzer{
		manager: manager,
	}
}

// AnalyzeSession analyzes a session and returns insights
func (sa *SessionAnalyzer) AnalyzeSession(sessionID string) (*SessionAnalysis, error) {
	session, err := sa.manager.LoadSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %v", err)
	}
	
	analysis := &SessionAnalysis{
		SessionID:     sessionID,
		AnalyzedAt:    time.Now(),
		MessageCount:  len(session.Messages),
		Duration:      session.UpdatedAt.Sub(session.CreatedAt),
		UserMessages:  0,
		AssistantMessages: 0,
		ToolCalls:     0,
		ToolResults:   0,
		Attachments:   len(session.Attachments),
		GeneratedContent: len(session.GeneratedContent),
		Topics:        make([]string, 0),
		Keywords:      make([]string, 0),
	}
	
	// Analyze messages
	for _, message := range session.Messages {
		switch message.Role {
		case "user":
			analysis.UserMessages++
		case "assistant":
			analysis.AssistantMessages++
		}
		
		analysis.ToolCalls += len(message.ToolCalls)
		analysis.ToolResults += len(message.ToolResults)
		
		// Simple keyword extraction (in a real implementation, use NLP)
		analysis.Keywords = append(analysis.Keywords, extractKeywords(message.Content)...)
	}
	
	// Calculate averages
	if analysis.MessageCount > 0 {
		analysis.AvgMessageLength = float64(sa.getTotalContentLength(session)) / float64(analysis.MessageCount)
	}
	
	// Extract topics (simplified)
	analysis.Topics = extractTopics(session.Messages)
	
	return analysis, nil
}

// SessionAnalysis contains analysis results for a session
type SessionAnalysis struct {
	SessionID         string        `json:"session_id"`
	AnalyzedAt        time.Time     `json:"analyzed_at"`
	MessageCount      int           `json:"message_count"`
	Duration          time.Duration `json:"duration"`
	UserMessages      int           `json:"user_messages"`
	AssistantMessages int           `json:"assistant_messages"`
	ToolCalls         int           `json:"tool_calls"`
	ToolResults       int           `json:"tool_results"`
	Attachments       int           `json:"attachments"`
	GeneratedContent  int           `json:"generated_content"`
	AvgMessageLength  float64       `json:"avg_message_length"`
	Topics            []string      `json:"topics"`
	Keywords          []string      `json:"keywords"`
}

// SessionValidator provides session validation functionality
type SessionValidator struct{}

// NewSessionValidator creates a new session validator
func NewSessionValidator() *SessionValidator {
	return &SessionValidator{}
}

// ValidateSession validates a session
func (sv *SessionValidator) ValidateSession(session *Session) []ValidationError {
	var errors []ValidationError
	
	// Check required fields
	if session.ID == "" {
		errors = append(errors, ValidationError{
			Field:   "ID",
			Message: "session ID is required",
		})
	}
	
	if session.UserID == "" {
		errors = append(errors, ValidationError{
			Field:   "UserID",
			Message: "user ID is required",
		})
	}
	
	if session.CreatedAt.IsZero() {
		errors = append(errors, ValidationError{
			Field:   "CreatedAt",
			Message: "created timestamp is required",
		})
	}
	
	// Check timestamps
	if session.UpdatedAt.Before(session.CreatedAt) {
		errors = append(errors, ValidationError{
			Field:   "UpdatedAt",
			Message: "updated timestamp cannot be before created timestamp",
		})
	}
	
	if session.ExpiresAt.Before(session.CreatedAt) {
		errors = append(errors, ValidationError{
			Field:   "ExpiresAt",
			Message: "expiry timestamp cannot be before created timestamp",
		})
	}
	
	// Validate messages
	for i, message := range session.Messages {
		if message.ID == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("Messages[%d].ID", i),
				Message: "message ID is required",
			})
		}
		
		if message.Role == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("Messages[%d].Role", i),
				Message: "message role is required",
			})
		}
		
		if message.Timestamp.IsZero() {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("Messages[%d].Timestamp", i),
				Message: "message timestamp is required",
			})
		}
	}
	
	return errors
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Helper functions

// getTotalContentLength calculates total content length in a session
func (sa *SessionAnalyzer) getTotalContentLength(session *Session) int {
	total := 0
	for _, message := range session.Messages {
		total += len(message.Content)
	}
	return total
}

// extractKeywords extracts keywords from text (simplified)
func extractKeywords(text string) []string {
	// This is a simplified keyword extraction
	// In a real implementation, use proper NLP techniques
	words := strings.Fields(strings.ToLower(text))
	keywords := make([]string, 0)
	
	for _, word := range words {
		if len(word) > 3 && !isStopWord(word) {
			keywords = append(keywords, word)
		}
	}
	
	return keywords
}

// extractTopics extracts topics from messages (simplified)
func extractTopics(messages []SessionMessage) []string {
	// This is a simplified topic extraction
	// In a real implementation, use topic modeling techniques
	topics := make([]string, 0)
	
	// Simple heuristic: look for common patterns
	for _, message := range messages {
		if strings.Contains(strings.ToLower(message.Content), "code") {
			topics = append(topics, "Programming")
		}
		if strings.Contains(strings.ToLower(message.Content), "data") {
			topics = append(topics, "Data Analysis")
		}
		if strings.Contains(strings.ToLower(message.Content), "help") {
			topics = append(topics, "Help & Support")
		}
	}
	
	return removeDuplicates(topics)
}

// isStopWord checks if a word is a stop word
func isStopWord(word string) bool {
	stopWords := []string{"the", "and", "or", "but", "in", "on", "at", "to", "for", "of", "with", "by", "from", "is", "are", "was", "were", "be", "been", "being", "have", "has", "had", "do", "does", "did", "will", "would", "should", "could", "can", "may", "might", "must", "shall", "this", "that", "these", "those", "a", "an", "as", "if", "then", "than", "when", "where", "while", "how", "why", "what", "which", "who", "whom", "whose", "i", "you", "he", "she", "it", "we", "they", "me", "him", "her", "us", "them", "my", "your", "his", "her", "its", "our", "their", "mine", "yours", "hers", "ours", "theirs"}
	
	for _, stopWord := range stopWords {
		if word == stopWord {
			return true
		}
	}
	
	return false
}

// removeDuplicates removes duplicate strings from a slice
func removeDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	result := make([]string, 0)
	
	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}
	
	return result
}

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// formatBytes formats bytes into human readable string
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	
	if bytes < KB {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < MB {
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	} else if bytes < GB {
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	} else {
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	}
}