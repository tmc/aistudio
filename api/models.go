package api

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// ListModels returns a list of available Gemini models, optionally filtered by substring
func (c *Client) ListModels(filter string) ([]string, error) {
	// First, initialize the client if not already initialized
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.InitClient(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize client: %w", err)
	}

	log.Println("Returning list of standard Gemini models")

	// Define the standard Gemini models
	standardModels := []string{
		"gemini-1.0-pro",
		"gemini-1.0-pro-latest",
		"gemini-1.0-pro-vision",
		"gemini-1.0-pro-vision-latest",
		"gemini-1.5-pro",
		"gemini-1.5-flash",
		"gemini-1.5-pro-latest",
		"gemini-1.5-flash-latest",
	}

	// Filter models if a filter is provided
	if filter != "" {
		var filteredModels []string
		for _, model := range standardModels {
			if strings.Contains(strings.ToLower(model), strings.ToLower(filter)) {
				filteredModels = append(filteredModels, model)
			}
		}
		return filteredModels, nil
	}

	return standardModels, nil
}
