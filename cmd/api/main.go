package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"golang.org/x/time/rate"

	"streaming-api/internal/models"
	"streaming-api/pkg/auth"
	"streaming-api/pkg/kafka"
	"streaming-api/pkg/websocket"
)

type StreamManager struct {
	streams   map[string]*models.Stream
	mu        sync.RWMutex
	kafka     *kafka.Handler
	wsHandler *websocket.Handler
	config    models.StreamConfig
	authMgr   *auth.Manager
	// Map to track which streams belong to which clients
	clientStreams map[string]map[string]struct{} // clientID -> set of streamIDs
	clientMu      sync.RWMutex
}

func NewStreamManager(kafkaBrokers []string, config models.StreamConfig) *StreamManager {
	sm := &StreamManager{
		streams:       make(map[string]*models.Stream),
		kafka:         kafka.NewHandler(kafkaBrokers),
		config:        config,
		authMgr:       auth.NewManager(),
		clientStreams: make(map[string]map[string]struct{}),
	}

	sm.wsHandler = websocket.NewHandler(func(data []byte) error {
		var msg models.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("failed to unmarshal message: %w", err)
		}

		// Verify stream ownership
		sm.mu.RLock()
		stream, exists := sm.streams[msg.StreamID]
		sm.mu.RUnlock()

		if !exists {
			return fmt.Errorf("stream not found: %s", msg.StreamID)
		}

		// Process and produce message
		processedMsg := models.Message{
			StreamID:  msg.StreamID,
			Data:      msg.Data,
			Timestamp: time.Now().UnixNano(),
		}

		processedData, err := json.Marshal(processedMsg)
		if err != nil {
			return fmt.Errorf("failed to marshal processed message: %w", err)
		}

		err = sm.kafka.ProduceMessage(context.Background(), stream.Producer,
			"stream-"+msg.StreamID, processedData)
		if err != nil {
			return fmt.Errorf("failed to produce message: %w", err)
		}

		return nil
	})

	return sm
}

// RegisterClient creates a new API key for a client
func (sm *StreamManager) RegisterClient() (string, string, error) {
	clientID := uuid.New().String()
	apiKey, err := sm.authMgr.GenerateAPIKey(clientID)
	if err != nil {
		return "", "", err
	}

	sm.clientMu.Lock()
	sm.clientStreams[clientID] = make(map[string]struct{})
	sm.clientMu.Unlock()

	return clientID, apiKey.Key, nil
}

// CreateStream creates a new stream for a client
func (sm *StreamManager) CreateStream(clientID string) (*models.Stream, error) {
	streamID := uuid.New().String()

	producer, err := sm.kafka.NewProducer(streamID)
	if err != nil {
		return nil, err
	}

	consumer, err := sm.kafka.NewConsumer(streamID)
	if err != nil {
		producer.Close()
		return nil, err
	}

	stream := &models.Stream{
		ID:            streamID,
		Producer:      producer,
		Consumer:      consumer,
		Done:          make(chan struct{}),
		RateLimiter:   rate.NewLimiter(rate.Limit(sm.config.RateLimit), sm.config.BurstLimit),
		ProcessedData: make(chan []byte, sm.config.BufferSize),
	}

	// Register stream to client
	sm.clientMu.Lock()
	sm.clientStreams[clientID][streamID] = struct{}{}
	sm.clientMu.Unlock()

	sm.mu.Lock()
	sm.streams[streamID] = stream
	sm.mu.Unlock()

	sm.kafka.ConsumeMessages(consumer, stream.ProcessedData)

	return stream, nil
}

// verifyStreamAccess checks if a client has access to a stream
func (sm *StreamManager) verifyStreamAccess(clientID, streamID string) bool {
	sm.clientMu.RLock()
	defer sm.clientMu.RUnlock()

	streams, exists := sm.clientStreams[clientID]
	if !exists {
		return false
	}

	_, hasAccess := streams[streamID]
	return hasAccess
}

func main() {
	config := models.StreamConfig{
		RateLimit:  100.0,
		BurstLimit: 200,
		BufferSize: 1000,
	}

	sm := NewStreamManager([]string{"localhost:9092"}, config)

	r := mux.NewRouter()
	api := r.PathPrefix("/stream").Subrouter()

	// Apply authentication middleware to all routes
	api.Use(sm.authMgr.Middleware)

	// Register new client and get API key
	r.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		clientID, apiKey, err := sm.RegisterClient()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"client_id": clientID,
			"api_key":   apiKey,
		})
	}).Methods("POST")

	// Create new stream
	api.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		clientID, err := sm.authMgr.GetClientID(apiKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		stream, err := sm.CreateStream(clientID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"stream_id": stream.ID,
		})
	}).Methods("POST")

	// Handle WebSocket connections
	api.HandleFunc("/{stream_id}/ws", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		streamID := vars["stream_id"]

		// Verify client has access to this stream
		apiKey := r.Header.Get("X-API-Key")
		clientID, err := sm.authMgr.GetClientID(apiKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		if !sm.verifyStreamAccess(clientID, streamID) {
			http.Error(w, "Unauthorized access to stream", http.StatusForbidden)
			return
		}

		sm.mu.RLock()
		stream, exists := sm.streams[streamID]
		sm.mu.RUnlock()

		if !exists {
			http.Error(w, "Stream not found", http.StatusNotFound)
			return
		}

		if err := sm.wsHandler.HandleConnection(stream, w, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	log.Printf("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
