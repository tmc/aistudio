package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	langalpha "cloud.google.com/go/ai/generativelanguage/apiv1alpha"
	langalphapb "cloud.google.com/go/ai/generativelanguage/apiv1alpha/generativelanguagepb"
	langalphabeta "cloud.google.com/go/ai/generativelanguage/apiv1beta"
	langbetapb "cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// APIVersion represents different versions of the Gemini API
type APIVersion string

const (
	APIVersionAlpha APIVersion = "alpha"
	APIVersionBeta  APIVersion = "beta"
	APIVersionV1    APIVersion = "v1"
)

// ModelInfo contains information about a model
type ModelInfo struct {
	Name        string     // Full model name including prefix
	DisplayName string     // Display name
	Description string     // Model description
	Version     APIVersion // API version this model works with
	SupportsSSE bool       // Whether the model supports server-side events
}

// ListModelsOptions provides options for model listing
type ListModelsOptions struct {
	Filter      string       // Filter string to limit results
	APIVersions []APIVersion // Specific API versions to query (empty means all)
	Raw         bool         // Return raw ModelInfo objects instead of strings
}

// DefaultListModelsOptions returns the default options for listing models
func DefaultListModelsOptions() ListModelsOptions {
	return ListModelsOptions{
		Filter:      "",
		APIVersions: []APIVersion{APIVersionBeta}, // Default to Beta API only
		Raw:         false,
	}
}

// ListModels returns a list of available Gemini models, optionally filtered by substring
func (c *Client) ListModels(filter string) ([]string, error) {
	options := DefaultListModelsOptions()
	options.Filter = filter
	models, err := c.ListModelsWithOptions(options)
	if err != nil {
		return nil, err
	}
	return models, nil
}

// ListModelsWithOptions returns a list of models with advanced options
func (c *Client) ListModelsWithOptions(options ListModelsOptions) ([]string, error) {
	// First, initialize the client if not already initialized
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.InitClient(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize client: %w", err)
	}

	log.Println("Getting list of supported models from the API")

	// Create the options for the model client
	var clientOpts []option.ClientOption
	if c.APIKey != "" {
		clientOpts = append(clientOpts, option.WithAPIKey(c.APIKey))
	}
	if c.httpTransport != nil {
		clientOpts = append(clientOpts, option.WithHTTPClient(&http.Client{Transport: c.httpTransport}))
	}

	var models []string
	var modelInfos []ModelInfo
	var apiErrors []string

	// If no specific API versions are requested, use defaults
	if len(options.APIVersions) == 0 {
		options.APIVersions = []APIVersion{APIVersionBeta, APIVersionAlpha}
	}

	// Try each API version
	for _, version := range options.APIVersions {
		var versionModels []string
		var versionInfos []ModelInfo
		var err error

		switch version {
		case APIVersionBeta:
			versionModels, versionInfos, err = c.listModelsV1Beta(ctx, clientOpts)
		case APIVersionAlpha:
			versionModels, versionInfos, err = c.listModelsV1Alpha(ctx, clientOpts)
		case APIVersionV1:
			versionModels, versionInfos, err = c.listModelsV1(ctx, clientOpts)
		}

		if err != nil {
			apiErrors = append(apiErrors, fmt.Sprintf("%s: %v", version, err))
			continue
		}

		models = append(models, versionModels...)
		modelInfos = append(modelInfos, versionInfos...)
	}

	// If we didn't get any models from any API, fall back to our hardcoded list
	if len(models) == 0 {
		log.Println("No models returned from APIs, falling back to hardcoded list")
		if len(apiErrors) > 0 {
			log.Printf("API errors: %s", strings.Join(apiErrors, "; "))
		}
		return c.getStandardModels(options.Filter)
	}

	// Filter models if a filter is provided
	if options.Filter != "" {
		var filteredModels []string
		for _, model := range models {
			if strings.Contains(strings.ToLower(model), strings.ToLower(options.Filter)) {
				filteredModels = append(filteredModels, model)
			}
		}
		return filteredModels, nil
	}

	return models, nil
}

// listModelsV1Beta lists models using the v1beta API
func (c *Client) listModelsV1Beta(ctx context.Context, clientOpts []option.ClientOption) ([]string, []ModelInfo, error) {
	// Create the model client
	modelClient, err := langalphabeta.NewModelClient(ctx, clientOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create v1beta model client: %w", err)
	}
	defer modelClient.Close()

	// Create the request
	req := &langbetapb.ListModelsRequest{}

	// Call ListModels and get iterator
	it := modelClient.ListModels(ctx, req)

	var models []string
	var modelInfos []ModelInfo

	for {
		model, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("error iterating v1beta models: %w", err)
		}

		// Store both with and without 'models/' prefix for compatibility
		modelName := model.GetName()
		models = append(models, modelName)

		// Create ModelInfo
		modelInfo := ModelInfo{
			Name:        modelName,
			DisplayName: model.GetDisplayName(),
			Description: model.GetDescription(),
			Version:     APIVersionBeta,
			SupportsSSE: true, // Most v1beta models support SSE
		}
		modelInfos = append(modelInfos, modelInfo)

		// Add version without 'models/' prefix if it exists
		if strings.HasPrefix(modelName, "models/") {
			models = append(models, strings.TrimPrefix(modelName, "models/"))
		}
	}

	return models, modelInfos, nil
}

// listModelsV1Alpha lists models using the v1alpha API
func (c *Client) listModelsV1Alpha(ctx context.Context, clientOpts []option.ClientOption) ([]string, []ModelInfo, error) {
	// Create the model client
	modelClient, err := langalpha.NewModelClient(ctx, clientOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create v1alpha model client: %w", err)
	}
	defer modelClient.Close()

	// Create the request
	req := &langalphapb.ListModelsRequest{}

	// Call ListModels and get iterator
	it := modelClient.ListModels(ctx, req)

	var models []string
	var modelInfos []ModelInfo

	for {
		model, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("error iterating v1alpha models: %w", err)
		}

		// Store both with and without 'models/' prefix for compatibility
		modelName := model.GetName()
		models = append(models, modelName)

		// Create ModelInfo
		modelInfo := ModelInfo{
			Name:        modelName,
			DisplayName: model.GetDisplayName(),
			Description: model.GetDescription(),
			Version:     APIVersionAlpha,
			SupportsSSE: true, // Most v1alpha models support SSE
		}
		modelInfos = append(modelInfos, modelInfo)

		// Add version without 'models/' prefix if it exists
		if strings.HasPrefix(modelName, "models/") {
			models = append(models, strings.TrimPrefix(modelName, "models/"))
		}
	}

	return models, modelInfos, nil
}

// listModelsV1 lists models using the v1 API (for future compatibility)
func (c *Client) listModelsV1(ctx context.Context, clientOpts []option.ClientOption) ([]string, []ModelInfo, error) {
	// V1 API is not yet implemented, return empty list for now
	// This is a placeholder for future implementation when the v1 API is available
	return nil, nil, fmt.Errorf("v1 API not implemented yet")
}

// getStandardModels returns a list of standard models from our hardcoded list
func (c *Client) getStandardModels(filter string) ([]string, error) {
	log.Println("Using locally defined model list")
	standardModels := []string{
		"gemini-1.0-pro",
		"gemini-1.0-pro-latest",
		"gemini-1.0-pro-vision",
		"gemini-1.0-pro-vision-latest",
		"gemini-1.5-pro",
		"gemini-1.5-flash",
		"gemini-1.5-pro-latest",
		"gemini-1.5-flash-latest",
		"gemini-2.0-flash-live-001",
		"gemini-2.0-flash-live-preview-04-09",
		"gemini-2.0-pro-live-001",
		"gemini-2.0-pro-live-002",
		"gemini-2.5-flash-live",
		"gemini-2.5-pro-live",
		"models/gemini-1.0-pro",
		"models/gemini-1.0-pro-latest",
		"models/gemini-1.0-pro-vision",
		"models/gemini-1.0-pro-vision-latest",
		"models/gemini-1.5-pro",
		"models/gemini-1.5-flash",
		"models/gemini-1.5-pro-latest",
		"models/gemini-1.5-flash-latest",
		"models/gemini-2.0-flash-live-001",
		"models/gemini-2.0-flash-live-preview-04-09",
		"models/gemini-2.0-pro-live-001",
		"models/gemini-2.0-pro-live-002",
		"models/gemini-2.5-flash-live",
		"models/gemini-2.5-pro-live",
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

// ValidateModel checks if a model name is in the list of supported models
func (c *Client) ValidateModel(modelName string) (bool, error) {
	// Special case for Gemini 2.0 preview models - accept all of them
	if strings.Contains(modelName, "gemini-2.0") || strings.Contains(modelName, "models/gemini-2.0") {
		return true, nil
	}

	// Special case for Gemini 2.5 models - accept all of them
	if strings.Contains(modelName, "gemini-2.5") || strings.Contains(modelName, "models/gemini-2.5") {
		return true, nil
	}

	validModels, err := c.ListModels("")
	if err != nil {
		return false, err
	}

	for _, model := range validModels {
		if model == modelName {
			return true, nil
		}
	}

	return false, nil
}
