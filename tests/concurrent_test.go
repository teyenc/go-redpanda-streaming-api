package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestClient struct {
	StreamID string
	Conn     *websocket.Conn
	Messages []string
	mu       sync.Mutex
	t        testing.TB
}

func NewTestClient(t testing.TB) (*TestClient, error) {
	t.Helper()

	// Create stream
	t.Log("Creating new test client...")
	resp, err := http.Post("http://localhost:8080/stream/start", "application/json", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var streamResp struct {
		StreamID string `json:"stream_id"`
	}
	if err := json.Unmarshal(body, &streamResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	t.Logf("→ Stream created with ID: %s", streamResp.StreamID)

	// Connect WebSocket
	url := fmt.Sprintf("ws://localhost:8080/stream/%s/ws", streamResp.StreamID)
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect websocket: %w", err)
	}
	t.Log("→ WebSocket connection established")

	client := &TestClient{
		StreamID: streamResp.StreamID,
		Conn:     c,
		Messages: make([]string, 0),
		t:        t,
	}

	// Start message reader
	go client.readMessages()

	return client, nil
}

func (c *TestClient) readMessages() {
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			// Don't report error on normal closure
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
				c.t.Logf("WebSocket read error: %v", err)
			}
			return
		}
		c.mu.Lock()
		c.Messages = append(c.Messages, string(message))
		c.t.Logf("→ Client %s received: %s", c.StreamID[0:8], string(message))
		c.mu.Unlock()
	}
}

func (c *TestClient) SendMessage(data string) error {
	msg := struct {
		StreamID string `json:"stream_id"`
		Data     string `json:"data"`
	}{
		StreamID: c.StreamID,
		Data:     data,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	c.t.Logf("→ Client %s sending: %s", c.StreamID[0:8], string(msgBytes))
	return c.Conn.WriteMessage(websocket.TextMessage, msgBytes)
}

func (c *TestClient) Close() {
	c.t.Logf("→ Closing client %s", c.StreamID[0:8])
	c.Conn.Close()
}

func (c *TestClient) GetMessages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.Messages...)
}

func TestConcurrentStreams(t *testing.T) {
	t.Log("=== Starting Concurrent Streams Test ===")

	numClients := 10 // Reduced for clarity in logs
	messagesPerClient := 5
	t.Logf("Setting up test with %d clients, %d messages per client", numClients, messagesPerClient)

	clients := make([]*TestClient, numClients)
	var wg sync.WaitGroup

	// Create clients
	t.Log("1. Creating test clients...")
	for i := 0; i < numClients; i++ {
		client, err := NewTestClient(t)
		require.NoError(t, err, "Failed to create client %d", i)
		clients[i] = client
		defer client.Close()
		t.Logf("→ Client %d created with ID: %s", i, client.StreamID[0:8])
	}

	// Send messages concurrently
	t.Log("2. Starting concurrent message sending...")
	wg.Add(numClients)
	startTime := time.Now()

	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer wg.Done()
			client := clients[clientID]

			for j := 0; j < messagesPerClient; j++ {
				msg := fmt.Sprintf("Message %d from client %d", j, clientID)
				err := client.SendMessage(msg)
				if err != nil {
					t.Errorf("Client %d failed to send message %d: %v", clientID, j, err)
				}
				time.Sleep(100 * time.Millisecond) // Rate limiting
			}
			t.Logf("→ Client %d finished sending %d messages", clientID, messagesPerClient)
		}(i)
	}

	// Wait for all messages to be sent
	wg.Wait()
	duration := time.Since(startTime)
	t.Logf("3. All messages sent in %v", duration)

	// Wait for message processing
	t.Log("4. Waiting for message processing...")
	time.Sleep(2 * time.Second)

	// Verify messages
	t.Log("5. Verifying messages...")
	totalMessages := 0
	for i, client := range clients {
		messages := client.GetMessages()
		messageCount := len(messages)
		totalMessages += messageCount
		assert.GreaterOrEqualf(t, messageCount, messagesPerClient,
			"Client %d should receive at least %d messages", i, messagesPerClient)
		t.Logf("→ Client %d received %d messages", i, messageCount)
	}

	t.Logf("=== Concurrent Test Completed ===")
	t.Logf("Total messages processed: %d", totalMessages)
	t.Logf("Average messages per second: %.2f", float64(totalMessages)/duration.Seconds())
}

func TestStreamReliability(t *testing.T) {
	t.Log("=== Starting Stream Reliability Test ===")

	t.Log("1. Creating initial connection...")
	client, err := NewTestClient(t)
	require.NoError(t, err)
	defer client.Close()

	// Test reconnection
	t.Log("2. Testing reconnection...")
	t.Log("→ Closing initial connection")
	client.Conn.Close()
	time.Sleep(time.Second)

	// Reconnect
	t.Log("3. Attempting to reconnect...")
	url := fmt.Sprintf("ws://localhost:8080/stream/%s/ws", client.StreamID)
	newConn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	client.Conn = newConn
	t.Log("→ Reconnection successful")

	// Verify can still send/receive
	t.Log("4. Verifying message sending after reconnect...")
	err = client.SendMessage("Test message after reconnection")
	assert.NoError(t, err)
	t.Log("→ Message sent successfully after reconnect")

	t.Log("=== Stream Reliability Test Completed ===")
}

func BenchmarkStreamThroughput(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"Small-1KB", 1024},
		{"Medium-10KB", 10 * 1024},
		{"Large-100KB", 100 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			client, err := NewTestClient(b)
			require.NoError(b, err)
			defer client.Close()

			message := make([]byte, size.size)
			for i := range message {
				message[i] = 'a'
			}
			testMessage := string(message)

			b.ResetTimer()
			b.SetBytes(int64(size.size))

			for i := 0; i < b.N; i++ {
				err := client.SendMessage(testMessage)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
