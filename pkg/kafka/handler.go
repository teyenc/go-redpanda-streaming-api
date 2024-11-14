// pkg/kafka/handler.go

package kafka

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

type Handler struct {
	brokers []string
}

func NewHandler(brokers []string) *Handler {
	return &Handler{
		brokers: brokers,
	}
}

func (h *Handler) createTopicIfNotExists(client *kgo.Client, topic string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // Increased timeout
	defer cancel()

	createReq := &kmsg.CreateTopicsRequest{
		Topics: []kmsg.CreateTopicsRequestTopic{
			{
				Topic:             topic,
				NumPartitions:     1,
				ReplicationFactor: 1,
			},
		},
		ValidateOnly: false,
	}

	resp, err := createReq.RequestWith(ctx, client)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	for _, topic := range resp.Topics {
		if topic.ErrorCode != 0 && topic.ErrorCode != 36 { // 36 is topic already exists
			return fmt.Errorf("failed to create topic %s: %v", topic.Topic, topic.ErrorCode)
		}
	}

	return nil
}

func (h *Handler) NewProducer(streamID string) (*kgo.Client, error) {

	producer, err := kgo.NewClient(
		kgo.SeedBrokers(h.brokers...),
		kgo.ProducerBatchMaxBytes(2*1024*1024), // Double the batch size
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
		// kgo.ProducerBatchCompression(kgo.SnappyCompression), // Use efficient compression
		kgo.RetryTimeout(20*time.Second), // Increase timeout for retries
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	// Create topic for this stream
	topicName := "stream-" + streamID
	if err := h.createTopicIfNotExists(producer, topicName); err != nil {
		producer.Close()
		return nil, fmt.Errorf("failed to create topic: %w", err)
	}

	return producer, nil
}

func (h *Handler) NewConsumer(streamID string) (*kgo.Client, error) {
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(h.brokers...),
		kgo.ConsumerGroup("stream-"+streamID),
		kgo.ConsumeTopics("stream-"+streamID),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.AllowAutoTopicCreation(),
		kgo.RetryTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	// Create topic for this stream
	topicName := "stream-" + streamID
	if err := h.createTopicIfNotExists(consumer, topicName); err != nil {
		consumer.Close()
		return nil, fmt.Errorf("failed to create topic: %w", err)
	}

	return consumer, nil
}

func (h *Handler) ProduceMessage(ctx context.Context, producer *kgo.Client, topic string, data []byte) error {
	record := &kgo.Record{
		Topic: topic,
		Value: data,
	}

	result := producer.ProduceSync(ctx, record)
	if err := result.FirstErr(); err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}

	log.Printf("Message produced to topic %s", topic)
	return nil
}

func (h *Handler) ConsumeMessages(consumer *kgo.Client, msgChan chan<- []byte) {
	go func() {
		for {
			fetches := consumer.PollFetches(context.Background())
			if fetches.IsClientClosed() {
				return
			}

			fetches.EachRecord(func(record *kgo.Record) {
				select {
				case msgChan <- record.Value:
					log.Printf("Message consumed from topic %s", record.Topic)
				default:
					log.Printf("Warning: message buffer full, dropping message")
				}
			})
		}
	}()
}
