package integration

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestExternalAPIConfig_Basic(t *testing.T) {
	config := ExternalAPIConfig{
		MaxRetries:     3,
		RetryDelay:     1 * time.Second,
		RequestTimeout: 30 * time.Second,
	}

	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries to be 3, got %d", config.MaxRetries)
	}

	if config.RetryDelay != 1*time.Second {
		t.Errorf("Expected RetryDelay to be 1s, got %v", config.RetryDelay)
	}

	if config.RequestTimeout != 30*time.Second {
		t.Errorf("Expected RequestTimeout to be 30s, got %v", config.RequestTimeout)
	}
}

func TestNewExternalAPIManager_Basic(t *testing.T) {
	config := ExternalAPIConfig{
		MaxRetries:     3,
		RetryDelay:     1 * time.Second,
		RequestTimeout: 30 * time.Second,
	}
	updateChan := make(chan tea.Msg, 1)

	manager := NewExternalAPIManager(config, updateChan)

	if manager == nil {
		t.Fatal("Expected ExternalAPIManager to be created")
	}

	if manager.uiUpdateChan == nil {
		t.Error("Expected uiUpdateChan to be set")
	}

	if manager.httpClient == nil {
		t.Error("Expected httpClient to be initialized")
	}
}

func TestAPIEndpoint_Basic(t *testing.T) {
	endpoint := APIEndpoint{
		ID:          "test-api",
		Name:        "Test API",
		BaseURL:     "https://api.example.com",
		Description: "Test API endpoint",
		AuthType:    "bearer",
		Timeout:     30 * time.Second,
	}

	if endpoint.ID != "test-api" {
		t.Errorf("Expected ID to be 'test-api', got %s", endpoint.ID)
	}

	if endpoint.Name != "Test API" {
		t.Errorf("Expected Name to be 'Test API', got %s", endpoint.Name)
	}

	if endpoint.BaseURL != "https://api.example.com" {
		t.Errorf("Expected BaseURL to be 'https://api.example.com', got %s", endpoint.BaseURL)
	}

	if endpoint.AuthType != "bearer" {
		t.Errorf("Expected AuthType to be 'bearer', got %s", endpoint.AuthType)
	}
}

func TestExecutionContext_Basic(t *testing.T) {
	// Create a basic ExecutionContext
	ctx := &ExecutionContext{
		ID:        "ctx-123",
		StartTime: time.Now(),
	}

	if ctx.ID != "ctx-123" {
		t.Errorf("Expected ID to be 'ctx-123', got %s", ctx.ID)
	}

	if ctx.StartTime.IsZero() {
		t.Error("Expected StartTime to be set")
	}
}