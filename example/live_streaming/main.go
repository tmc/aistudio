// Copyright 2025 Google LLC
// Licensed under the Apache License, Version 2.0

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/tmc/aistudio/api"
)

var addr = flag.String("addr", "localhost:8080", "http service address")
var useVertex = flag.Bool("vertex", false, "Use Vertex AI instead of Gemini API")
var modelName = flag.String("model", "gemini-1.5-pro", "Model name to use")
var apiKey = flag.String("api-key", "", "API key for Gemini API (not needed for Vertex AI with ADC)")
var projectID = flag.String("project-id", "", "Project ID for Vertex AI (only needed when using Vertex AI)")
var location = flag.String("location", "us-central1", "Location for Vertex AI (only used with Vertex)")
var geminiVersion = flag.String("gemini-version", "v1beta", "Gemini API version (v1alpha or v1beta)")

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Define message structure for WebSocket communication
type message struct {
	Content string `json:"content"`
}

// Define response structure for WebSocket communication
type response struct {
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

func main() {
	flag.Parse()
	log.SetFlags(0)

	// Log which API we're using
	if *useVertex {
		log.Printf("Using Vertex AI with project %s in %s", *projectID, *location)
	} else {
		log.Printf("Using Gemini API with version %s and model %s", *geminiVersion, *modelName)
	}

	// Set up HTTP handlers
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/live", liveHandler)

	// Start HTTP server
	log.Printf("Server listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

// Handle WebSocket connections for live API interaction
func liveHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade error:", err)
		return
	}
	defer c.Close()

	// Create a background context
	ctx := context.Background()

	log.Printf("WebSocket connection established")

	// Message handling loop
	for {
		// Read message from WebSocket
		messageType, messageData, err := c.ReadMessage()
		if err != nil {
			log.Println("read error:", err)
			break
		}

		// Only process text messages
		if messageType != websocket.TextMessage {
			continue
		}

		// Parse message JSON
		var msg message
		if err := json.Unmarshal(messageData, &msg); err != nil {
			log.Println("unmarshal error:", err)
			continue
		}

		// Process the request
		respContent, respErr := processLiveRequest(ctx, msg)

		// Create response
		resp := response{
			Content: respContent,
		}
		if respErr != nil {
			resp.Error = respErr.Error()
		}

		// Send response back to WebSocket client
		respBytes, err := json.Marshal(resp)
		if err != nil {
			log.Println("marshal error:", err)
			continue
		}

		if err := c.WriteMessage(websocket.TextMessage, respBytes); err != nil {
			log.Println("write error:", err)
			break
		}
	}
}

func processLiveRequest(ctx context.Context, msg message) (string, error) {
	// Use our aistudio API client
	client, cleanup, err := initAIStudioClient(ctx)
	if err != nil {
		return "", err
	}
	defer cleanup()

	return processAIStudioRequest(ctx, client, msg.Content)
}

// initAIStudioClient initializes our API client with the correct settings
func initAIStudioClient(ctx context.Context) (*api.Client, func(), error) {
	// Determine API key - first from command line, then environment variables
	apiKeyValue := *apiKey
	if apiKeyValue == "" {
		apiKeyValue = os.Getenv("API_KEY")
		if apiKeyValue == "" {
			apiKeyValue = os.Getenv("GEMINI_API_KEY")
		}
	}

	config := &api.APIClientConfig{
		ModelName: *modelName,
	}

	// Configure client based on whether we're using Vertex AI
	if *useVertex {
		if *projectID == "" {
			return nil, nil, fmt.Errorf("project ID is required for Vertex AI")
		}
		config.Backend = api.BackendVertexAI
		config.ProjectID = *projectID
		config.Location = *location
		config.APIKey = apiKeyValue // Optional for Vertex AI
	} else {
		if apiKeyValue == "" {
			return nil, nil, fmt.Errorf("API key is required for Gemini API")
		}
		config.Backend = api.BackendGeminiAPI
		config.APIKey = apiKeyValue
	}

	// Create and initialize the client
	client, err := api.NewClient(ctx, config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create API client: %v", err)
	}

	// Set the Gemini API version if specified
	if !*useVertex && *geminiVersion != "" {
		client.GeminiVersion = *geminiVersion
	}

	// Create a cleanup function
	cleanup := func() {
		client.Close()
	}

	return client, cleanup, nil
}

// processAIStudioRequest sends a request to our API client
func processAIStudioRequest(ctx context.Context, client *api.Client, content string) (string, error) {
	// Create a stream config
	config := &api.StreamClientConfig{
		ModelName: *modelName,
	}

	// Initialize a bidirectional stream
	stream, err := client.InitBidiStream(ctx, config)
	if err != nil {
		return "", fmt.Errorf("failed to initialize stream: %v", err)
	}
	defer stream.CloseSend()

	// Send message to the stream
	if err := client.SendMessageToBidiStream(stream, content); err != nil {
		return "", fmt.Errorf("failed to send message: %v", err)
	}

	// Receive response
	var fullResponse string
	for {
		resp, err := stream.Recv()
		if err != nil {
			break // End of stream or error
		}

		// Extract text from response
		output := api.ExtractBidiOutput(resp)
		fullResponse += output.Text

		if output.TurnComplete {
			break
		}
	}

	return fullResponse, nil
}

// HTML template for the web UI
var homeTemplate = template.Must(template.New("").Parse(`
<!DOCTYPE html>
<html>
<head>
    <title>AIStudio Live API Demo</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            max-width: 800px;
            margin: 0 auto;
            padding: 20px;
            line-height: 1.6;
        }
        #messages {
            margin-top: 20px;
            border: 1px solid #ddd;
            padding: 10px;
            border-radius: 5px;
            min-height: 300px;
            max-height: 500px;
            overflow-y: auto;
        }
        #input {
            margin-top: 10px;
            width: 100%;
            padding: 10px;
            box-sizing: border-box;
            border: 1px solid #ddd;
            border-radius: 5px;
        }
        .message {
            margin-bottom: 10px;
            padding: 10px;
            border-radius: 5px;
        }
        .user {
            background-color: #e6f7ff;
            align-self: flex-end;
        }
        .model {
            background-color: #f0f0f0;
            align-self: flex-start;
        }
        .error {
            color: red;
            font-style: italic;
        }
        h1 {
            color: #333;
        }
        button {
            margin-top: 10px;
            padding: 10px 15px;
            background-color: #4CAF50;
            color: white;
            border: none;
            border-radius: 5px;
            cursor: pointer;
        }
        button:hover {
            background-color: #45a049;
        }
    </style>
</head>
<body>
    <h1>AIStudio Live API Demo</h1>
    <div id="messages"></div>
    <textarea id="input" placeholder="Type your message here..." rows="4"></textarea>
    <button id="send">Send</button>

    <script>
        const wsURL = "{{.}}";
        const messagesContainer = document.getElementById('messages');
        const inputBox = document.getElementById('input');
        const sendButton = document.getElementById('send');
        let ws;

        function connect() {
            ws = new WebSocket(wsURL);

            ws.onopen = function() {
                console.log('Connected to server');
                addSystemMessage('Connected to server. Start chatting!');
                sendButton.disabled = false;
            };

            ws.onmessage = function(event) {
                const response = JSON.parse(event.data);
                if (response.error) {
                    addErrorMessage(response.error);
                } else {
                    addModelMessage(response.content);
                }
            };

            ws.onclose = function() {
                console.log('Disconnected from server');
                addSystemMessage('Disconnected from server. Reconnecting...');
                sendButton.disabled = true;
                // Try to reconnect after 2 seconds
                setTimeout(connect, 2000);
            };

            ws.onerror = function(error) {
                console.error('WebSocket error:', error);
                addErrorMessage('Connection error. See console for details.');
            };
        }

        function sendMessage() {
            const message = inputBox.value.trim();
            if (message && ws && ws.readyState === WebSocket.OPEN) {
                addUserMessage(message);
                ws.send(JSON.stringify({ content: message }));
                inputBox.value = '';
            }
        }

        function addMessage(content, className) {
            const messageDiv = document.createElement('div');
            messageDiv.className = 'message ' + className;
            messageDiv.textContent = content;
            messagesContainer.appendChild(messageDiv);
            messagesContainer.scrollTop = messagesContainer.scrollHeight;
        }

        function addUserMessage(content) {
            addMessage('You: ' + content, 'user');
        }

        function addModelMessage(content) {
            addMessage('AI: ' + content, 'model');
        }

        function addSystemMessage(content) {
            addMessage('System: ' + content, 'system');
        }

        function addErrorMessage(content) {
            addMessage('Error: ' + content, 'error');
        }

        // Event listeners
        sendButton.addEventListener('click', sendMessage);
        inputBox.addEventListener('keypress', function(event) {
            if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault();
                sendMessage();
            }
        });

        // Initialize connection
        connect();
    </script>
</body>
</html>
`))

// Render the home page
func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Determine WebSocket protocol based on HTTP request
	wsProtocol := "ws"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		wsProtocol = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/live", wsProtocol, r.Host)
	homeTemplate.Execute(w, wsURL)
}
