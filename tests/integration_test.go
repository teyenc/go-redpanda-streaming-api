package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestStreamIntegration(t *testing.T) {
	// Register client
	resp, err := http.Post("http://localhost:8080/register", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	var registration struct {
		ClientID string `json:"client_id"`
		APIKey   string `json:"api_key"`
	}
	err = json.NewDecoder(resp.Body).Decode(&registration)
	if err != nil {
		t.Fatalf("Failed to decode registration: %v", err)
	}
	resp.Body.Close()

	// Create stream
	req, err := http.NewRequest("POST", "http://localhost:8080/stream/start", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("X-API-Key", registration.APIKey)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}

	var result struct {
		StreamID string `json:"stream_id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	resp.Body.Close()

	// Connect WebSocket
	headers := http.Header{}
	headers.Set("X-API-Key", registration.APIKey)
	url := fmt.Sprintf("ws://localhost:8080/stream/%s/ws", result.StreamID)
	c, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		t.Fatalf("WebSocket connection failed: %v", err)
	}
	defer c.Close()

	// Test message exchange
	message := fmt.Sprintf(`{"stream_id": "%s", "data": "integration test"}`, result.StreamID)
	err = c.WriteMessage(websocket.TextMessage, []byte(message))
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Wait for response
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, response, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to receive message: %v", err)
	}

	// Verify response
	var msgResponse struct {
		StreamID  string `json:"stream_id"`
		Data      string `json:"data"`
		Timestamp int64  `json:"timestamp"`
	}
	err = json.Unmarshal(response, &msgResponse)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if msgResponse.StreamID != result.StreamID {
		t.Errorf("StreamID mismatch: got %s, want %s", msgResponse.StreamID, result.StreamID)
	}
	if msgResponse.Data != "integration test" {
		t.Errorf("Data mismatch: got %s, want integration test", msgResponse.Data)
	}
	if msgResponse.Timestamp == 0 {
		t.Error("Timestamp not set")
	}
}
