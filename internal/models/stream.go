package models

import (
	"github.com/gorilla/websocket"
	"github.com/twmb/franz-go/pkg/kgo"
	"golang.org/x/time/rate"
)

// Stream represents an individual data stream
type Stream struct {
	ID            string
	Producer      *kgo.Client
	Consumer      *kgo.Client
	WebSocket     *websocket.Conn
	Done          chan struct{}
	RateLimiter   *rate.Limiter
	ProcessedData chan []byte
}

// Message represents the structure of streaming data
type Message struct {
	StreamID  string      `json:"stream_id"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

// StreamConfig holds configuration for a stream
type StreamConfig struct {
	RateLimit  float64
	BurstLimit int
	BufferSize int
}
