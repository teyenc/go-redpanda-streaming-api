package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestWebSocketConnection(t *testing.T) {
	// 1. Create stream
	resp, err := http.Post("http://localhost:8080/stream/start", "application/json", nil)
	require.NoError(t, err)

	var result struct {
		StreamID string `json:"stream_id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	// 2. Connect WebSocket
	url := fmt.Sprintf("ws://localhost:8080/stream/%s/ws", result.StreamID)
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer c.Close()

	// 3. Send a message
	message := fmt.Sprintf(`{"stream_id": "%s", "data": "test"}`, result.StreamID)
	err = c.WriteMessage(websocket.TextMessage, []byte(message))
	require.NoError(t, err)

	// 4. Wait for response
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, response, err := c.ReadMessage()
	require.NoError(t, err)
	t.Logf("Received: %s", string(response))
}
