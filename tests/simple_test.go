package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type StreamResponse struct {
	StreamID string `json:"stream_id"`
}

func TestBasicStreamWorkflow(t *testing.T) {
	t.Log("=== Starting Basic Stream Workflow Test ===")

	// 1. Create a new stream
	t.Log("1. Creating new stream...")
	resp, err := http.Post("http://localhost:8080/stream/start", "application/json", nil)
	require.NoError(t, err, "Failed to create stream")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read response")

	var streamResp StreamResponse
	err = json.Unmarshal(body, &streamResp)
	require.NoError(t, err, "Failed to parse response")
	require.NotEmpty(t, streamResp.StreamID, "Stream ID should not be empty")
	t.Logf("→ Stream created successfully with ID: %s", streamResp.StreamID)

	// 2. Connect to WebSocket
	t.Log("2. Connecting to WebSocket...")
	url := fmt.Sprintf("ws://localhost:8080/stream/%s/ws", streamResp.StreamID)
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err, "Failed to connect to WebSocket")
	defer c.Close()
	t.Log("→ WebSocket connection established")

	// 3. Test message exchange
	t.Log("3. Setting up message handlers...")
	done := make(chan struct{})
	received := make(chan []byte)

	// Start reading messages
	go func() {
		defer close(done)
		t.Log("→ Starting message listener")
		_, message, err := c.ReadMessage()
		if err != nil {
			t.Errorf("Failed to read message: %v", err)
			return
		}
		t.Log("→ Message received from server")
		received <- message
	}()

	// Prepare and send test message
	t.Log("4. Sending test message...")
	testMsg := struct {
		StreamID string    `json:"stream_id"`
		Data     string    `json:"data"`
		Time     time.Time `json:"time"`
	}{
		StreamID: streamResp.StreamID,
		Data:     "Test message from simple workflow test",
		Time:     time.Now(),
	}

	msgBytes, err := json.Marshal(testMsg)
	require.NoError(t, err, "Failed to marshal message")

	err = c.WriteMessage(websocket.TextMessage, msgBytes)
	require.NoError(t, err, "Failed to send message")
	t.Logf("→ Test message sent: %s", string(msgBytes))

	// Wait for response with timeout
	t.Log("5. Waiting for response...")
	select {
	case msg := <-received:
		t.Log("→ Response received, validating...")
		var response map[string]interface{}
		err := json.Unmarshal(msg, &response)
		require.NoError(t, err, "Failed to parse response message")

		assert.Equal(t, streamResp.StreamID, response["stream_id"], "Stream ID should match")
		assert.Equal(t, "Test message from simple workflow test", response["data"], "Message data should match")
		assert.NotNil(t, response["timestamp"], "Response should include timestamp")
		t.Logf("→ Response validated successfully: %s", string(msg))

	case <-time.After(5 * time.Second):
		t.Fatal("❌ Timeout waiting for response")
	}

	t.Log("=== Basic Stream Workflow Test Completed Successfully ===")
}
