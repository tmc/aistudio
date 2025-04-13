package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ModelInfo represents a model from the Aider MCP server
type ModelInfo struct {
	Name string `json:"name"`
}

// ModelsResponse represents the response from the Aider MCP server
type ModelsResponse struct {
	Models []string `json:"models"`
}

// ListModels returns a list of available models, optionally filtered by substring
func (c *Client) ListModels(filter string) ([]string, error) {
	// First, initialize the client if not already initialized
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.InitClient(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize client: %w", err)
	}

	// We'll use the Aider MCP server to list models, which requires a special URL and auth
	log.Println("Fetching models from Aider MCP server")
	
	// Create HTTP client
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	
	// Create request to Aider MCP server
	// Note: This is a simplified version - in a real implementation, you would need
	// proper authentication and the correct endpoint URL for the Aider MCP server
	url := "http://localhost:8000/aider/list-models"
	if filter != "" {
		url += "?substring=" + filter
	}
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Add authorization header if API key is provided
	if c.APIKey != "" {
		req.Header.Add("Authorization", "Bearer "+c.APIKey)
	}
	
	// Send request
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	
	// Parse response
	var modelsResp ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	// Filter models if a filter is provided (double check, even though we filtered in the request)
	if filter != "" {
		var filteredModels []string
		for _, model := range modelsResp.Models {
			if strings.Contains(strings.ToLower(model), strings.ToLower(filter)) {
				filteredModels = append(filteredModels, model)
			}
		}
		return filteredModels, nil
	}
	
	return modelsResp.Models, nil
}