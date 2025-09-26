package session

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileStorageProvider implements file-based session storage
type FileStorageProvider struct {
	baseDir     string
	compression bool
	mutex       sync.RWMutex
}

// NewFileStorageProvider creates a new file storage provider
func NewFileStorageProvider(baseDir string) (*FileStorageProvider, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %v", err)
	}
	
	return &FileStorageProvider{
		baseDir:     baseDir,
		compression: true,
	}, nil
}

// SaveSession saves a session to a file
func (fsp *FileStorageProvider) SaveSession(session *Session) error {
	fsp.mutex.Lock()
	defer fsp.mutex.Unlock()
	
	// Create user directory
	userDir := filepath.Join(fsp.baseDir, session.UserID)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return fmt.Errorf("failed to create user directory: %v", err)
	}
	
	// Determine file path
	filename := fmt.Sprintf("%s.json", session.ID)
	if fsp.compression {
		filename += ".gz"
	}
	filePath := filepath.Join(userDir, filename)
	
	// Create temporary file
	tempPath := filePath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer file.Close()
	
	// Write session data
	var writer io.Writer = file
	if fsp.compression {
		gzWriter := gzip.NewWriter(file)
		defer gzWriter.Close()
		writer = gzWriter
	}
	
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(session); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to encode session: %v", err)
	}
	
	// Close writers
	if fsp.compression {
		if gzWriter, ok := writer.(*gzip.Writer); ok {
			gzWriter.Close()
		}
	}
	file.Close()
	
	// Atomic rename
	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temporary file: %v", err)
	}
	
	// Create backup if enabled
	if err := fsp.createBackup(session.ID, filePath); err != nil {
		// Log error but don't fail the save
		fmt.Printf("[SESSION] Failed to create backup for session %s: %v\n", session.ID, err)
	}
	
	return nil
}

// LoadSession loads a session from a file
func (fsp *FileStorageProvider) LoadSession(sessionID string) (*Session, error) {
	fsp.mutex.RLock()
	defer fsp.mutex.RUnlock()
	
	// Find session file
	filePath, err := fsp.findSessionFile(sessionID)
	if err != nil {
		return nil, err
	}
	
	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open session file: %v", err)
	}
	defer file.Close()
	
	// Create reader
	var reader io.Reader = file
	if strings.HasSuffix(filePath, ".gz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %v", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}
	
	// Decode session
	var session Session
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&session); err != nil {
		return nil, fmt.Errorf("failed to decode session: %v", err)
	}
	
	return &session, nil
}

// DeleteSession deletes a session file
func (fsp *FileStorageProvider) DeleteSession(sessionID string) error {
	fsp.mutex.Lock()
	defer fsp.mutex.Unlock()
	
	// Find session file
	filePath, err := fsp.findSessionFile(sessionID)
	if err != nil {
		return err
	}
	
	// Delete file
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete session file: %v", err)
	}
	
	// Delete backups
	fsp.deleteBackups(sessionID)
	
	return nil
}

// ListSessions lists all sessions for a user
func (fsp *FileStorageProvider) ListSessions(userID string) ([]*Session, error) {
	fsp.mutex.RLock()
	defer fsp.mutex.RUnlock()
	
	userDir := filepath.Join(fsp.baseDir, userID)
	if _, err := os.Stat(userDir); os.IsNotExist(err) {
		return []*Session{}, nil
	}
	
	files, err := os.ReadDir(userDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read user directory: %v", err)
	}
	
	sessions := make([]*Session, 0)
	for _, file := range files {
		if file.IsDir() || strings.HasPrefix(file.Name(), ".") {
			continue
		}
		
		// Skip if the filename doesn't match expected pattern
		if !strings.HasSuffix(file.Name(), ".json") && !strings.HasSuffix(file.Name(), ".json.gz") {
			continue
		}
		
		// Extract session ID from filename
		sessionID := file.Name()
		if strings.HasSuffix(sessionID, ".json.gz") {
			sessionID = strings.TrimSuffix(sessionID, ".json.gz")
		} else if strings.HasSuffix(sessionID, ".json") {
			sessionID = strings.TrimSuffix(sessionID, ".json")
		}
		
		// Load session
		session, err := fsp.LoadSession(sessionID)
		if err != nil {
			fmt.Printf("[SESSION] Failed to load session %s: %v\n", sessionID, err)
			continue
		}
		
		sessions = append(sessions, session)
	}
	
	return sessions, nil
}

// BackupSession creates a backup of a session
func (fsp *FileStorageProvider) BackupSession(sessionID string) error {
	fsp.mutex.RLock()
	defer fsp.mutex.RUnlock()
	
	// Find session file
	filePath, err := fsp.findSessionFile(sessionID)
	if err != nil {
		return err
	}
	
	return fsp.createBackup(sessionID, filePath)
}

// RestoreSession restores a session from a backup
func (fsp *FileStorageProvider) RestoreSession(sessionID string, backupID string) error {
	fsp.mutex.Lock()
	defer fsp.mutex.Unlock()
	
	// Find backup file
	backupPath := fsp.getBackupPath(sessionID, backupID)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup not found: %s", backupID)
	}
	
	// Find current session file location
	filePath, err := fsp.findSessionFile(sessionID)
	if err != nil {
		// If session doesn't exist, determine where it should be
		session, err := fsp.loadSessionFromFile(backupPath)
		if err != nil {
			return fmt.Errorf("failed to load backup: %v", err)
		}
		
		userDir := filepath.Join(fsp.baseDir, session.UserID)
		filename := fmt.Sprintf("%s.json", sessionID)
		if fsp.compression {
			filename += ".gz"
		}
		filePath = filepath.Join(userDir, filename)
	}
	
	// Copy backup to session location
	if err := fsp.copyFile(backupPath, filePath); err != nil {
		return fmt.Errorf("failed to restore backup: %v", err)
	}
	
	return nil
}

// Save implements SessionStore interface
func (fsp *FileStorageProvider) Save(session *Session) error {
	return fsp.SaveSession(session)
}

// Load implements SessionStore interface
func (fsp *FileStorageProvider) Load(sessionID string) (*Session, error) {
	return fsp.LoadSession(sessionID)
}

// Delete implements SessionStore interface
func (fsp *FileStorageProvider) Delete(sessionID string) error {
	return fsp.DeleteSession(sessionID)
}

// List implements SessionStore interface
func (fsp *FileStorageProvider) List(userID string) ([]string, error) {
	sessions, err := fsp.ListSessions(userID)
	if err != nil {
		return nil, err
	}
	
	sessionIDs := make([]string, len(sessions))
	for i, session := range sessions {
		sessionIDs[i] = session.ID
	}
	
	return sessionIDs, nil
}

// Exists implements SessionStore interface
func (fsp *FileStorageProvider) Exists(sessionID string) bool {
	_, err := fsp.findSessionFile(sessionID)
	return err == nil
}

// findSessionFile finds the file path for a session
func (fsp *FileStorageProvider) findSessionFile(sessionID string) (string, error) {
	// Search all user directories
	userDirs, err := os.ReadDir(fsp.baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to read base directory: %v", err)
	}
	
	for _, userDir := range userDirs {
		if !userDir.IsDir() {
			continue
		}
		
		userPath := filepath.Join(fsp.baseDir, userDir.Name())
		
		// Try both compressed and uncompressed versions
		candidates := []string{
			filepath.Join(userPath, sessionID+".json.gz"),
			filepath.Join(userPath, sessionID+".json"),
		}
		
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	
	return "", fmt.Errorf("session file not found: %s", sessionID)
}

// createBackup creates a backup of a session file
func (fsp *FileStorageProvider) createBackup(sessionID string, filePath string) error {
	// Create backup directory
	backupDir := filepath.Join(fsp.baseDir, ".backups", sessionID)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %v", err)
	}
	
	// Create backup filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	backupFilename := fmt.Sprintf("%s_%s.json", sessionID, timestamp)
	if strings.HasSuffix(filePath, ".gz") {
		backupFilename += ".gz"
	}
	backupPath := filepath.Join(backupDir, backupFilename)
	
	// Copy file
	if err := fsp.copyFile(filePath, backupPath); err != nil {
		return fmt.Errorf("failed to copy session file: %v", err)
	}
	
	// Clean up old backups (keep only last 5)
	fsp.cleanupBackups(sessionID, 5)
	
	return nil
}

// getBackupPath returns the path for a specific backup
func (fsp *FileStorageProvider) getBackupPath(sessionID string, backupID string) string {
	backupDir := filepath.Join(fsp.baseDir, ".backups", sessionID)
	return filepath.Join(backupDir, backupID)
}

// deleteBackups deletes all backups for a session
func (fsp *FileStorageProvider) deleteBackups(sessionID string) {
	backupDir := filepath.Join(fsp.baseDir, ".backups", sessionID)
	os.RemoveAll(backupDir)
}

// cleanupBackups removes old backups, keeping only the specified number
func (fsp *FileStorageProvider) cleanupBackups(sessionID string, keep int) {
	backupDir := filepath.Join(fsp.baseDir, ".backups", sessionID)
	
	files, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}
	
	// Sort files by modification time (newest first)
	if len(files) <= keep {
		return
	}
	
	// Remove oldest files
	for i := keep; i < len(files); i++ {
		filePath := filepath.Join(backupDir, files[i].Name())
		os.Remove(filePath)
	}
}

// copyFile copies a file from source to destination
func (fsp *FileStorageProvider) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()
	
	_, err = io.Copy(destFile, sourceFile)
	return err
}

// loadSessionFromFile loads a session from a specific file path
func (fsp *FileStorageProvider) loadSessionFromFile(filePath string) (*Session, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()
	
	var reader io.Reader = file
	if strings.HasSuffix(filePath, ".gz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %v", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}
	
	var session Session
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&session); err != nil {
		return nil, fmt.Errorf("failed to decode session: %v", err)
	}
	
	return &session, nil
}

// MemoryStorageProvider implements in-memory session storage
type MemoryStorageProvider struct {
	sessions map[string]*Session
	mutex    sync.RWMutex
}

// NewMemoryStorageProvider creates a new memory storage provider
func NewMemoryStorageProvider() *MemoryStorageProvider {
	return &MemoryStorageProvider{
		sessions: make(map[string]*Session),
	}
}

// SaveSession saves a session to memory
func (msp *MemoryStorageProvider) SaveSession(session *Session) error {
	msp.mutex.Lock()
	defer msp.mutex.Unlock()
	
	// Store the session pointer directly
	msp.sessions[session.ID] = session
	return nil
}

// LoadSession loads a session from memory
func (msp *MemoryStorageProvider) LoadSession(sessionID string) (*Session, error) {
	msp.mutex.RLock()
	defer msp.mutex.RUnlock()
	
	session, exists := msp.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	
	// Return the session pointer directly
	return session, nil
}

// DeleteSession deletes a session from memory
func (msp *MemoryStorageProvider) DeleteSession(sessionID string) error {
	msp.mutex.Lock()
	defer msp.mutex.Unlock()
	
	delete(msp.sessions, sessionID)
	return nil
}

// ListSessions lists all sessions for a user
func (msp *MemoryStorageProvider) ListSessions(userID string) ([]*Session, error) {
	msp.mutex.RLock()
	defer msp.mutex.RUnlock()
	
	sessions := make([]*Session, 0)
	for _, session := range msp.sessions {
		if session.UserID == userID {
			sessions = append(sessions, session)
		}
	}
	
	return sessions, nil
}

// BackupSession creates a backup of a session (no-op for memory)
func (msp *MemoryStorageProvider) BackupSession(sessionID string) error {
	return nil // No-op for memory storage
}

// RestoreSession restores a session from a backup (no-op for memory)
func (msp *MemoryStorageProvider) RestoreSession(sessionID string, backupID string) error {
	return fmt.Errorf("restore not supported for memory storage")
}

// Save implements SessionStore interface
func (msp *MemoryStorageProvider) Save(session *Session) error {
	return msp.SaveSession(session)
}

// Load implements SessionStore interface
func (msp *MemoryStorageProvider) Load(sessionID string) (*Session, error) {
	return msp.LoadSession(sessionID)
}

// Delete implements SessionStore interface
func (msp *MemoryStorageProvider) Delete(sessionID string) error {
	return msp.DeleteSession(sessionID)
}

// List implements SessionStore interface
func (msp *MemoryStorageProvider) List(userID string) ([]string, error) {
	sessions, err := msp.ListSessions(userID)
	if err != nil {
		return nil, err
	}
	
	sessionIDs := make([]string, len(sessions))
	for i, session := range sessions {
		sessionIDs[i] = session.ID
	}
	
	return sessionIDs, nil
}

// Exists implements SessionStore interface
func (msp *MemoryStorageProvider) Exists(sessionID string) bool {
	msp.mutex.RLock()
	defer msp.mutex.RUnlock()
	
	_, exists := msp.sessions[sessionID]
	return exists
}