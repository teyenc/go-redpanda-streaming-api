package tests

import (
	"context"
	"testing"
	"time"

	"streaming-api/pkg/kafka"
)

func TestKafkaHandler(t *testing.T) {
	handler := kafka.NewHandler([]string{"localhost:9092"})
	streamID := "test-stream"

	// Test producer creation
	producer, err := handler.NewProducer(streamID)
	if err != nil {
		t.Fatalf("Failed to create producer: %v", err)
	}
	defer producer.Close()

	// Test consumer creation
	consumer, err := handler.NewConsumer(streamID)
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}
	defer consumer.Close()

	// Test message production and consumption
	msgChan := make(chan []byte, 1)
	handler.ConsumeMessages(consumer, msgChan)

	// Send test message
	testMsg := []byte("test message")
	err = handler.ProduceMessage(context.Background(), producer, "stream-"+streamID, testMsg)
	if err != nil {
		t.Fatalf("Failed to produce message: %v", err)
	}

	// Wait for message
	select {
	case received := <-msgChan:
		if string(received) != string(testMsg) {
			t.Errorf("Message mismatch: got %s, want %s", received, testMsg)
		}
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for message")
	}
}
