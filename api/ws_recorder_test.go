package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestWSRecorder tests the WebSocket recorder functionality
func TestWSRecorder(t *testing.T) {
	// Create temporary directory for test recordings
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("ws_recorder_test_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	recordFile := filepath.Join(tempDir, "test_recording.wsrec")

	// Test recording
	testRecord(t, recordFile)

	// Test replay
	testReplay(t, recordFile)
}

// testRecord records a WebSocket session
func testRecord(t *testing.T, recordFile string) {
	// Create recorder in record mode
	recorder, err := NewWSRecorder(recordFile, true)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}

	// Record some messages
	sendData := []string{
		`{"message":"Hello world"}`,
		`{"message":"Testing recorder"}`,
	}

	receiveData := []string{
		`{"response":"Hi there"}`,
		`{"response":"Recording works"}`,
		`{"response":"Final message"}`,
	}

	// Record send messages
	for _, msg := range sendData {
		err := recorder.RecordSend([]byte(msg), websocket.TextMessage)
		if err != nil {
			t.Fatalf("Failed to record send message: %v", err)
		}
	}

	// Record receive messages
	for _, msg := range receiveData {
		err := recorder.RecordReceive([]byte(msg), websocket.TextMessage)
		if err != nil {
			t.Fatalf("Failed to record receive message: %v", err)
		}
	}

	// Close the recorder
	if err := recorder.Close(); err != nil {
		t.Fatalf("Failed to close recorder: %v", err)
	}

	// Verify recording file exists
	if _, err := os.Stat(recordFile); os.IsNotExist(err) {
		t.Fatalf("Recording file not created: %s", recordFile)
	}

	t.Logf("Successfully created test recording at %s", recordFile)
}

// testReplay tests replaying a WebSocket session
func testReplay(t *testing.T, recordFile string) {
	// Create recorder in replay mode
	recorder, err := NewWSRecorder(recordFile, false)
	if err != nil {
		t.Fatalf("Failed to create replayer: %v", err)
	}

	// Get all messages
	var messages []struct {
		direction string
		data      []byte
		msgType   int
	}

	for {
		direction, data, msgType, err := recorder.GetNextMessage()
		if err != nil {
			if strings.Contains(err.Error(), "end of recording") {
				break
			}
			t.Fatalf("Error getting message: %v", err)
		}

		messages = append(messages, struct {
			direction string
			data      []byte
			msgType   int
		}{
			direction: direction,
			data:      data,
			msgType:   msgType,
		})
	}

	// Verify we have the correct number of messages
	if len(messages) != 5 { // 2 send + 3 receive
		t.Errorf("Expected 5 messages, got %d", len(messages))
	}

	// Count messages by direction
	var sendCount, receiveCount int
	for _, msg := range messages {
		if msg.direction == "send" {
			sendCount++
		} else if msg.direction == "receive" {
			receiveCount++
		}
	}

	// Verify counts
	if sendCount != 2 {
		t.Errorf("Expected 2 send messages, got %d", sendCount)
	}
	if receiveCount != 3 {
		t.Errorf("Expected 3 receive messages, got %d", receiveCount)
	}

	t.Logf("Successfully replayed test recording with %d total messages", len(messages))
}

// TestLiveClientWithRecorder tests the LiveClient with a recorder
func TestLiveClientWithRecorder(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	// Check for API key
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY environment variable not set")
	}

	// Create temporary directory for test recordings
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("live_client_test_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	recordFile := filepath.Join(tempDir, "live_client_test.wsrec")

	// Test with recording mode
	testLiveClientRecord(t, apiKey, recordFile)

	// Test with replay mode
	testLiveClientReplay(t, apiKey, recordFile)
}

// testLiveClientRecord tests LiveClient with a recorder in record mode
func testLiveClientRecord(t *testing.T, apiKey string, recordFile string) {
	// Create recorder in record mode
	recorder, err := NewWSRecorder(recordFile, true)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create LiveClient with recorder
	config := &StreamClientConfig{
		ModelName:       "gemini-2.0-flash-live-001",
		EnableWebSocket: true,
		Temperature:     0.7,
		TopP:            0.95,
		TopK:            40,
		MaxOutputTokens: 128,
	}

	client, err := NewLiveClient(ctx, apiKey, config, recorder)
	if err != nil {
		t.Fatalf("Failed to create LiveClient: %v", err)
	}

	// Initialize client
	if err := client.Initialize(); err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}

	// Send a message
	testMessage := "Hello, please respond with a brief greeting for a test."
	if err := client.SendMessage(testMessage); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Receive responses until turn complete
	var responseComplete bool
	var responseCount int
	var fullResponse string

	for !responseComplete && responseCount < 10 {
		output, err := client.ReceiveMessage()
		if err != nil {
			t.Fatalf("Failed to receive message: %v", err)
		}

		responseCount++
		fullResponse += output.Text
		responseComplete = output.TurnComplete

		t.Logf("Received chunk #%d: %q", responseCount, output.Text)

		if responseComplete {
			t.Logf("Response complete")
			break
		}
	}

	// Close client and recorder
	if err := client.Close(); err != nil {
		t.Fatalf("Failed to close client: %v", err)
	}

	// Verify recording file exists
	if _, err := os.Stat(recordFile); os.IsNotExist(err) {
		t.Fatalf("Recording file not created: %s", recordFile)
	}

	t.Logf("Successfully recorded live client session with %d messages", responseCount)
	t.Logf("Response: %q", fullResponse)
}

// testLiveClientReplay tests LiveClient with a recorder in replay mode
func testLiveClientReplay(t *testing.T, apiKey string, recordFile string) {
	// Create recorder in replay mode
	recorder, err := NewWSRecorder(recordFile, false)
	if err != nil {
		t.Fatalf("Failed to create replayer: %v", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create LiveClient with recorder in replay mode
	config := &StreamClientConfig{
		ModelName:       "gemini-2.0-flash-live-001",
		EnableWebSocket: true,
		Temperature:     0.7,
		TopP:            0.95,
		TopK:            40,
		MaxOutputTokens: 128,
	}

	client, err := NewLiveClient(ctx, apiKey, config, recorder)
	if err != nil {
		t.Fatalf("Failed to create LiveClient: %v", err)
	}

	// Initialize client
	if err := client.Initialize(); err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}

	// Send a message (should use message from recording)
	testMessage := "Hello, please respond with a brief greeting for a test."
	if err := client.SendMessage(testMessage); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Receive responses until turn complete
	var responseComplete bool
	var responseCount int
	var fullResponse string

	for !responseComplete && responseCount < 10 {
		output, err := client.ReceiveMessage()
		if err != nil {
			// End of recording is expected
			if strings.Contains(err.Error(), "end of recording") {
				t.Logf("End of recording reached")
				break
			}
			t.Fatalf("Failed to receive message: %v", err)
		}

		responseCount++
		fullResponse += output.Text
		responseComplete = output.TurnComplete

		t.Logf("Received chunk #%d from replay: %q", responseCount, output.Text)

		if responseComplete {
			t.Logf("Response complete")
			break
		}
	}

	// Close client
	if err := client.Close(); err != nil {
		t.Fatalf("Failed to close client: %v", err)
	}

	t.Logf("Successfully replayed live client session with %d messages", responseCount)
	t.Logf("Replayed response: %q", fullResponse)
}
