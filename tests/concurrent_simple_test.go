package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestSimpleConcurrentConnections(t *testing.T) {
	numClients := 5
	var wg sync.WaitGroup

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			// Create stream
			resp, err := http.Post("http://localhost:8080/stream/start", "application/json", nil)
			require.NoError(t, err)

			var result struct {
				StreamID string `json:"stream_id"`
			}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			// Connect WebSocket
			url := fmt.Sprintf("ws://localhost:8080/stream/%s/ws", result.StreamID)
			c, _, err := websocket.DefaultDialer.Dial(url, nil)
			require.NoError(t, err)
			defer c.Close()

			// Send and receive one message
			message := fmt.Sprintf(`{"stream_id": "%s", "data": "test from client %d"}`, result.StreamID, clientID)
			err = c.WriteMessage(websocket.TextMessage, []byte(message))
			require.NoError(t, err)

			c.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, response, err := c.ReadMessage()
			require.NoError(t, err)
			t.Logf("Client %d received: %s", clientID, string(response))
		}(i)
	}

	wg.Wait()
}

func TestConcurrentConnections(t *testing.T) {
	numClients := 1000           // Number of concurrent clients
	var wg sync.WaitGroup        // WaitGroup to manage goroutines
	start := make(chan struct{}) // Channel to synchronize start

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			// Wait for the start signal
			<-start

			// Step 1: Create a stream via HTTP POST
			resp, err := http.Post("http://localhost:8080/stream/start", "application/json", nil)
			require.NoError(t, err)
			defer resp.Body.Close()

			var result struct {
				StreamID string `json:"stream_id"`
			}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			// Step 2: Connect WebSocket for bi-directional communication
			url := fmt.Sprintf("ws://localhost:8080/stream/%s/ws", result.StreamID)
			c, _, err := websocket.DefaultDialer.Dial(url, nil)
			require.NoError(t, err)
			defer c.Close()

			// Step 3: Send and receive a message over WebSocket
			message := fmt.Sprintf(`{"stream_id": "%s", "data": "test from client %d"}`, result.StreamID, clientID)
			err = c.WriteMessage(websocket.TextMessage, []byte(message))
			require.NoError(t, err)

			// Set read deadline and wait for a response message
			c.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, response, err := c.ReadMessage()
			require.NoError(t, err)
			t.Logf("Client %d received: %s", clientID, string(response))
		}(i)
	}

	// Close the start channel to release all waiting goroutines simultaneously
	close(start)

	// Wait for all goroutines to finish
	wg.Wait()
}
