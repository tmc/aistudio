package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tmc/aistudio"
	"github.com/tmc/aistudio/api"
)

func main() {
	// Parse command-line flags
	modelFlag := flag.String("model", "gemini-2.0-flash-live-001", "Model to test")
	promptFlag := flag.String("prompt", "Hello! Please reply with a very brief message.", "Prompt to send")
	wsFlag := flag.Bool("ws", false, "Use WebSocket protocol")
	vertexFlag := flag.Bool("vertex", false, "Use Vertex AI")
	projectIDFlag := flag.String("project-id", "", "Google Cloud project ID (for Vertex AI)")
	locationFlag := flag.String("location", "us-central1", "Google Cloud location (for Vertex AI)")
	verboseFlag := flag.Bool("v", false, "Verbose output")
	timeoutFlag := flag.Duration("timeout", 30*time.Second, "Timeout for requests")

	flag.Parse()

	// Enable detailed logging if verbose mode is enabled
	if *verboseFlag {
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
		os.Setenv("DEBUG_AISTUDIO", "1")
	} else {
		// Reduce log noise
		log.SetOutput(os.Stderr)
	}

	// Get API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY environment variable not set")
	}

	// Project ID for Vertex AI
	projectID := *projectIDFlag
	if *vertexFlag && projectID == "" {
		projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
		if projectID == "" {
			log.Fatal("No project ID specified. Use --project-id flag or set GOOGLE_CLOUD_PROJECT")
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()

	// Determine if we're testing a live model
	isLiveModel := api.IsLiveModel(*modelFlag)
	log.Printf("Testing model: %s (Live model: %v)", *modelFlag, isLiveModel)
	log.Printf("WebSocket mode: %v", *wsFlag)

	// Set up options
	var opts []aistudio.Option
	opts = append(opts, aistudio.WithModel(*modelFlag))
	opts = append(opts, aistudio.WithAPIKey(apiKey))

	// Enable WebSocket if requested
	if *wsFlag {
		opts = append(opts, aistudio.WithWebSocket(true))
		log.Println("WebSocket protocol enabled")
	}

	// Configure Vertex AI if requested
	if *vertexFlag {
		opts = append(opts, aistudio.WithVertexAI(true, projectID, *locationFlag))
		log.Printf("Using Vertex AI with project: %s, location: %s", projectID, *locationFlag)
	}

	// Create a new aistudio model with our options
	model := aistudio.New(opts...)

	// Initialize the model
	if _, err := model.InitModel(); err != nil {
		log.Fatalf("Failed to initialize model: %v", err)
	}

	// Create API client directly
	client, err := api.NewClient(context.Background(), &api.APIClientConfig{
		APIKey:    apiKey,
		ModelName: *modelFlag,
		Backend:   api.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatalf("Failed to create API client: %v", err)
	}

	// Initialize the client
	if err := client.InitClient(context.Background()); err != nil {
		log.Fatalf("Failed to initialize API client: %v", err)
	}

	// Create stream configuration
	streamConfig := &api.StreamClientConfig{
		ModelName:       *modelFlag,
		Temperature:     0.7,
		TopP:            0.95,
		TopK:            40,
		MaxOutputTokens: 256,
		EnableWebSocket: *wsFlag,
	}

	// Initialize bidirectional stream
	log.Println("Initializing stream...")
	stream, err := client.InitBidiStream(ctx, streamConfig)
	if err != nil {
		log.Fatalf("Failed to initialize stream: %v", err)
	}
	defer stream.CloseSend()

	// Send the message
	log.Printf("Sending message: %s", *promptFlag)
	if err := client.SendMessageToBidiStream(stream, *promptFlag); err != nil {
		log.Fatalf("Failed to send message: %v", err)
	}

	// Receive and display response
	var response string
	var receivedChunks int
	var turnComplete bool

	log.Println("Waiting for response...")
	for !turnComplete {
		resp, err := stream.Recv()
		if err != nil {
			log.Fatalf("Error receiving response: %v", err)
		}

		receivedChunks++

		// Extract text from response
		output := api.ExtractOutput(resp)
		response += output.Text
		turnComplete = output.TurnComplete

		log.Printf("Received chunk #%d: %d bytes", receivedChunks, len(output.Text))

		if turnComplete {
			log.Println("Turn complete")
			break
		}

		// Simple timeout check to prevent infinite loops
		select {
		case <-ctx.Done():
			log.Fatalf("Timeout waiting for complete response")
			return
		default:
			// Continue receiving
		}
	}

	// Print the response
	fmt.Printf("\nResponse from %s (WebSocket: %v):\n%s\n", *modelFlag, *wsFlag, response)
	log.Printf("Received %d chunks, total length: %d characters", receivedChunks, len(response))
}
