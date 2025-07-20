package aistudio

import (
	"context"
	"testing"
)

func TestMCPIntegrationStub(t *testing.T) {
	// Test basic MCP integration functionality
	config := &MCPConfig{
		Enabled: false,
		Server: MCPServerConfig{
			Enabled:    false,
			Port:       8080,
			Transports: []string{"http", "stdio"},
			Tools:      true,
			Resources:  true,
			Prompts:    true,
		},
		Clients: []MCPClientConfig{
			{
				Name:      "test-client",
				Transport: "stdio",
				Enabled:   false,
			},
		},
		Streaming: MCPStreamingConfig{
			Voice: MCPVoiceStreamingConfig{
				Enabled:               false,
				ExposeAsTools:         false,
				ExposeAsResources:     false,
				StreamingMode:         MCPVoiceStreamingModeDisabled,
				RealTimeTranscription: false,
			},
			Video: MCPVideoStreamingConfig{
				Enabled:           false,
				ExposeAsTools:     false,
				ExposeAsResources: false,
				StreamingMode:     MCPVideoStreamingModeDisabled,
				FrameAnalysis:     false,
				ObjectDetection:   false,
			},
		},
	}

	// Test constructor
	mcp := NewMCPIntegration(config)
	if mcp == nil {
		t.Fatal("NewMCPIntegration returned nil")
	}

	if mcp.enabled != config.Enabled {
		t.Errorf("Expected enabled=%v, got %v", config.Enabled, mcp.enabled)
	}

	// Test with nil config
	mcpNil := NewMCPIntegration(nil)
	if mcpNil == nil {
		t.Fatal("NewMCPIntegration with nil config returned nil")
	}

	if mcpNil.enabled != false {
		t.Errorf("Expected enabled=false for nil config, got %v", mcpNil.enabled)
	}

	// Test initialization
	ctx := context.Background()
	tm := NewToolManager()
	
	err := mcp.Initialize(ctx, tm)
	if err != nil {
		t.Errorf("Initialize failed: %v", err)
	}

	// Test methods don't panic
	mcp.SetVoiceStreamer(nil)  // Should not panic
	mcp.SetVideoStreamer(nil) // Should not panic

	// Test server and client getters
	server := mcp.GetMCPServer()
	if server != nil {
		t.Error("Expected GetMCPServer to return nil in stub implementation")
	}

	clients := mcp.GetMCPClients()
	if clients == nil {
		t.Error("Expected GetMCPClients to return empty map, got nil")
	}
	if len(clients) != 0 {
		t.Errorf("Expected empty clients map, got %d clients", len(clients))
	}

	// Test shutdown
	err = mcp.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestMCPStreamingModes(t *testing.T) {
	// Test voice streaming modes
	voiceModes := []MCPVoiceStreamingMode{
		MCPVoiceStreamingModeDisabled,
		MCPVoiceStreamingModeTools,
		MCPVoiceStreamingModeResources,
		MCPVoiceStreamingModeBidirectional,
	}

	expectedVoiceModes := []string{
		"disabled",
		"tools", 
		"resources",
		"bidirectional",
	}

	for i, mode := range voiceModes {
		if string(mode) != expectedVoiceModes[i] {
			t.Errorf("Voice mode %d: expected %s, got %s", i, expectedVoiceModes[i], string(mode))
		}
	}

	// Test video streaming modes
	videoModes := []MCPVideoStreamingMode{
		MCPVideoStreamingModeDisabled,
		MCPVideoStreamingModeTools,
		MCPVideoStreamingModeResources,
		MCPVideoStreamingModeLive,
	}

	expectedVideoModes := []string{
		"disabled",
		"tools",
		"resources", 
		"live",
	}

	for i, mode := range videoModes {
		if string(mode) != expectedVideoModes[i] {
			t.Errorf("Video mode %d: expected %s, got %s", i, expectedVideoModes[i], string(mode))
		}
	}
}

func TestMCPConfigValidation(t *testing.T) {
	// Test valid configuration
	config := &MCPConfig{
		Enabled: true,
		Server: MCPServerConfig{
			Enabled:    true,
			Port:       8080,
			Transports: []string{"http", "websocket", "stdio"},
			Tools:      true,
			Resources:  true,
			Prompts:    true,
		},
		Clients: []MCPClientConfig{
			{
				Name:      "filesystem-server",
				Command:   []string{"npx", "@modelcontextprotocol/server-filesystem"},
				Transport: "stdio",
				Enabled:   true,
			},
			{
				Name:      "remote-api",
				URL:       "http://localhost:9000",
				Transport: "http",
				Enabled:   true,
			},
		},
		Streaming: MCPStreamingConfig{
			Voice: MCPVoiceStreamingConfig{
				Enabled:               true,
				ExposeAsTools:         true,  
				ExposeAsResources:     true,
				StreamingMode:         MCPVoiceStreamingModeBidirectional,
				RealTimeTranscription: true,
				VoiceCloning:          false,
			},
			Video: MCPVideoStreamingConfig{
				Enabled:           true,
				ExposeAsTools:     true,
				ExposeAsResources: true,
				StreamingMode:     MCPVideoStreamingModeLive,
				FrameAnalysis:     true,
				ObjectDetection:   true,
				LiveAnnotations:   false,
			},
		},
	}

	mcp := NewMCPIntegration(config)
	if mcp == nil {
		t.Fatal("NewMCPIntegration failed with valid config")
	}

	// Config should be preserved
	if mcp.config.Enabled != config.Enabled {
		t.Error("Config enabled flag not preserved")
	}

	if mcp.config.Server.Port != config.Server.Port {
		t.Error("Server port not preserved")
	}

	if len(mcp.config.Clients) != len(config.Clients) {
		t.Error("Client configs not preserved")
	}
}