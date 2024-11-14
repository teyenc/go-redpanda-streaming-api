package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type BenchmarkMetrics struct {
	TotalMessages    int64
	TotalLatency     int64 // nanoseconds
	MaxLatency       int64 // nanoseconds
	MinLatency       int64 // nanoseconds
	FailedMessages   int64
	ActiveStreams    int64
	BytesSent        int64
	BytesReceived    int64
	MessagesSent     int64
	MessagesReceived int64
}

func BenchmarkStreamPerformance(b *testing.B) {
	numClients := 1000
	duration := 1 * time.Minute
	metrics := &BenchmarkMetrics{MinLatency: int64(^uint64(0) >> 1)} // Set min latency to max int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	startTime := time.Now() // Add this to track actual start time

	// Register client and get API key first
	resp, err := http.Post("http://localhost:8080/register", "application/json", nil)
	if err != nil {
		b.Fatalf("Failed to register client: %v", err)
	}
	var registration struct {
		ClientID string `json:"client_id"`
		APIKey   string `json:"api_key"`
	}
	err = json.NewDecoder(resp.Body).Decode(&registration)
	if err != nil {
		b.Fatalf("Failed to decode registration: %v", err)
	}
	resp.Body.Close()

	// Print system info
	b.Logf("Go Version: %s", runtime.Version())
	b.Logf("CPU Cores: %d", runtime.NumCPU())
	b.Logf("GOMAXPROCS: %d", runtime.GOMAXPROCS(0))

	// Start clients
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			// Create stream with API key
			req, err := http.NewRequest("POST", "http://localhost:8080/stream/start", nil)
			if err != nil {
				b.Errorf("Failed to create request: %v", err)
				return
			}
			req.Header.Set("X-API-Key", registration.APIKey)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				b.Errorf("Failed to create stream: %v", err)
				return
			}

			var result struct {
				StreamID string `json:"stream_id"`
			}
			err = json.NewDecoder(resp.Body).Decode(&result)
			if err != nil {
				b.Errorf("Failed to decode response: %v", err)
				return
			}
			resp.Body.Close()

			// Connect WebSocket with API key
			headers := http.Header{}
			headers.Set("X-API-Key", registration.APIKey)
			url := fmt.Sprintf("ws://localhost:8080/stream/%s/ws", result.StreamID)
			c, _, err := websocket.DefaultDialer.Dial(url, headers)
			if err != nil {
				b.Errorf("WebSocket connection failed: %v", err)
				return
			}
			defer c.Close()

			atomic.AddInt64(&metrics.ActiveStreams, 1)
			defer atomic.AddInt64(&metrics.ActiveStreams, -1)

			// Wait for start signal
			<-start

			// Start message reader
			go func() {
				for {
					_, message, err := c.ReadMessage()
					if err != nil {
						return
					}
					atomic.AddInt64(&metrics.MessagesReceived, 1)
					atomic.AddInt64(&metrics.BytesReceived, int64(len(message)))
				}
			}()

			// Send messages until duration expires
			ticker := time.NewTicker(100 * time.Millisecond) // 10 messages per second per client
			defer ticker.Stop()
			timeout := time.After(duration)

			for {
				select {
				case <-timeout:
					return
				case <-ticker.C:
					// Record start time
					sendTime := time.Now()

					// Send message
					message := fmt.Sprintf(`{"stream_id":"%s","data":"benchmark test %d","timestamp":%d}`,
						result.StreamID, clientID, sendTime.UnixNano())

					err := c.WriteMessage(websocket.TextMessage, []byte(message))
					if err != nil {
						atomic.AddInt64(&metrics.FailedMessages, 1)
						continue
					}

					// Record metrics
					atomic.AddInt64(&metrics.MessagesSent, 1)
					atomic.AddInt64(&metrics.BytesSent, int64(len(message)))

					// Calculate latency
					latency := time.Since(sendTime).Nanoseconds()
					atomic.AddInt64(&metrics.TotalLatency, latency)
					atomic.AddInt64(&metrics.TotalMessages, 1)

					// Update min/max latency
					for {
						currentMax := atomic.LoadInt64(&metrics.MaxLatency)
						if latency <= currentMax || atomic.CompareAndSwapInt64(&metrics.MaxLatency, currentMax, latency) {
							break
						}
					}

					for {
						currentMin := atomic.LoadInt64(&metrics.MinLatency)
						if latency >= currentMin || atomic.CompareAndSwapInt64(&metrics.MinLatency, currentMin, latency) {
							break
						}
					}
				}
			}
		}(i)
	}

	// Start benchmark
	b.ResetTimer()
	close(start)

	// Print real-time metrics every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				total := atomic.LoadInt64(&metrics.TotalMessages)
				if total > 0 {
					avgLatency := time.Duration(atomic.LoadInt64(&metrics.TotalLatency) / total)
					maxLatency := time.Duration(atomic.LoadInt64(&metrics.MaxLatency))
					minLatency := time.Duration(atomic.LoadInt64(&metrics.MinLatency))
					elapsed := time.Since(startTime)

					b.Logf("\nCurrent Metrics (after %v):", elapsed)
					b.Logf("Active Streams: %d", atomic.LoadInt64(&metrics.ActiveStreams))
					b.Logf("Messages Sent: %d", atomic.LoadInt64(&metrics.MessagesSent))
					b.Logf("Messages Received: %d", atomic.LoadInt64(&metrics.MessagesReceived))
					b.Logf("Failed Messages: %d", atomic.LoadInt64(&metrics.FailedMessages))
					b.Logf("Average Latency: %v", avgLatency)
					b.Logf("Min Latency: %v", minLatency)
					b.Logf("Max Latency: %v", maxLatency)
					b.Logf("Throughput: %d msgs/sec", atomic.LoadInt64(&metrics.MessagesSent)/int64(elapsed.Seconds()))
					b.Logf("Data Sent: %.2f MB", float64(atomic.LoadInt64(&metrics.BytesSent))/(1024*1024))
					b.Logf("Data Received: %.2f MB", float64(atomic.LoadInt64(&metrics.BytesReceived))/(1024*1024))
				}
			}
		}
	}()

	// Wait for all clients to finish
	wg.Wait()
	ticker.Stop()

	// Calculate final elapsed time
	elapsed := time.Since(startTime)

	// Print final results
	total := atomic.LoadInt64(&metrics.TotalMessages)
	if total > 0 {
		avgLatency := time.Duration(atomic.LoadInt64(&metrics.TotalLatency) / total)
		maxLatency := time.Duration(atomic.LoadInt64(&metrics.MaxLatency))
		minLatency := time.Duration(atomic.LoadInt64(&metrics.MinLatency))

		b.Logf("\nFinal Results:")
		b.Logf("Total Duration: %v", elapsed)
		b.Logf("Total Messages Sent: %d", atomic.LoadInt64(&metrics.MessagesSent))
		b.Logf("Total Messages Received: %d", atomic.LoadInt64(&metrics.MessagesReceived))
		b.Logf("Average Latency: %v", avgLatency)
		b.Logf("Min Latency: %v", minLatency)
		b.Logf("Max Latency: %v", maxLatency)
		b.Logf("Message Success Rate: %.2f%%", 100*(1-float64(atomic.LoadInt64(&metrics.FailedMessages))/float64(atomic.LoadInt64(&metrics.MessagesSent))))
		b.Logf("Average Throughput: %d msgs/sec", atomic.LoadInt64(&metrics.MessagesSent)/int64(elapsed.Seconds()))
		b.Logf("Total Data Transferred: %.2f MB", float64(atomic.LoadInt64(&metrics.BytesSent)+atomic.LoadInt64(&metrics.BytesReceived))/(1024*1024))
	}
}
