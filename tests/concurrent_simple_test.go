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

type StreamResponse struct {
	StreamID string `json:"stream_id"`
	APIKey   string `json:"api_key"`
}

func TestSimpleConcurrentConnections(t *testing.T) {
	numClients := 5
	var wg sync.WaitGroup

	// Register client and get API key
	resp, err := http.Post("http://localhost:8080/register", "application/json", nil)
	require.NoError(t, err)
	var registration struct {
		ClientID string `json:"client_id"`
		APIKey   string `json:"api_key"`
	}
	err = json.NewDecoder(resp.Body).Decode(&registration)
	require.NoError(t, err)
	resp.Body.Close()

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			// Create stream with API key
			req, err := http.NewRequest("POST", "http://localhost:8080/stream/start", nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", registration.APIKey)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			var result struct {
				StreamID string `json:"stream_id"`
			}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)
			resp.Body.Close()

			// Connect WebSocket with API key
			headers := http.Header{}
			headers.Set("X-API-Key", registration.APIKey)
			url := fmt.Sprintf("ws://localhost:8080/stream/%s/ws", result.StreamID)
			c, _, err := websocket.DefaultDialer.Dial(url, headers)
			require.NoError(t, err)
			defer c.Close()

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
	numClients := 1000
	var wg sync.WaitGroup
	start := make(chan struct{})

	// Register client and get API key
	resp, err := http.Post("http://localhost:8080/register", "application/json", nil)
	require.NoError(t, err)
	var registration struct {
		ClientID string `json:"client_id"`
		APIKey   string `json:"api_key"`
	}
	err = json.NewDecoder(resp.Body).Decode(&registration)
	require.NoError(t, err)
	resp.Body.Close()

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			<-start

			// Create stream with API key
			req, err := http.NewRequest("POST", "http://localhost:8080/stream/start", nil)
			require.NoError(t, err)
			req.Header.Set("X-API-Key", registration.APIKey)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			var result struct {
				StreamID string `json:"stream_id"`
			}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)
			resp.Body.Close()

			// Connect WebSocket with API key
			headers := http.Header{}
			headers.Set("X-API-Key", registration.APIKey)
			url := fmt.Sprintf("ws://localhost:8080/stream/%s/ws", result.StreamID)
			c, _, err := websocket.DefaultDialer.Dial(url, headers)
			require.NoError(t, err)
			defer c.Close()

			message := fmt.Sprintf(`{"stream_id": "%s", "data": "test from client %d"}`, result.StreamID, clientID)
			err = c.WriteMessage(websocket.TextMessage, []byte(message))
			require.NoError(t, err)

			c.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, response, err := c.ReadMessage()
			require.NoError(t, err)
			t.Logf("Client %d received: %s", clientID, string(response))
		}(i)
	}

	close(start)
	wg.Wait()
}
