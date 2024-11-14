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
	"streaming-api/pkg/kafka"
	"streaming-api/pkg/websocket"
)

type StreamManager struct {
	streams   map[string]*models.Stream
	mu        sync.RWMutex
	kafka     *kafka.Handler
	wsHandler *websocket.Handler
	config    models.StreamConfig
}

func NewStreamManager(kafkaBrokers []string, config models.StreamConfig) *StreamManager {
	sm := &StreamManager{
		streams: make(map[string]*models.Stream),
		kafka:   kafka.NewHandler(kafkaBrokers),
		config:  config,
	}

	sm.wsHandler = websocket.NewHandler(func(data []byte) error {
		var msg models.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("failed to unmarshal message: %w", err)
		}

		sm.mu.RLock()
		stream, exists := sm.streams[msg.StreamID]
		sm.mu.RUnlock()

		if !exists {
			return fmt.Errorf("stream not found: %s", msg.StreamID)
		}

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

func (sm *StreamManager) CreateStream() (*models.Stream, error) {
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

	sm.mu.Lock()
	sm.streams[streamID] = stream
	sm.mu.Unlock()

	sm.kafka.ConsumeMessages(consumer, stream.ProcessedData)

	return stream, nil
}

func main() {
	config := models.StreamConfig{
		RateLimit:  100.0, // messages per second
		BurstLimit: 200,   // burst capacity
		BufferSize: 1000,  // channel buffer size
	}

	sm := NewStreamManager([]string{"localhost:9092"}, config)

	r := mux.NewRouter()
	api := r.PathPrefix("/stream").Subrouter()

	api.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		stream, err := sm.CreateStream()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"stream_id": stream.ID,
		})
	}).Methods("POST")

	api.HandleFunc("/{stream_id}/ws", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		streamID := vars["stream_id"]

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
