package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	ActiveStreams     prometheus.Gauge
	TotalMessages     prometheus.Counter
	MessageLatency    prometheus.Histogram
	MessageSize       prometheus.Histogram
	RateLimitExceeded prometheus.Counter
	KafkaErrors       prometheus.Counter
	WebSocketErrors   prometheus.Counter
}

func NewMetrics(namespace string) *Metrics {
	return &Metrics{
		ActiveStreams: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "active_streams",
			Help:      "The current number of active streams",
		}),
		TotalMessages: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "total_messages",
			Help:      "The total number of processed messages",
		}),
		MessageLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "message_latency_seconds",
			Help:      "Message processing latency in seconds",
			Buckets:   prometheus.DefBuckets,
		}),
		MessageSize: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "message_size_bytes",
			Help:      "Size of messages in bytes",
			Buckets:   []float64{64, 128, 256, 512, 1024, 2048, 4096, 8192},
		}),
		RateLimitExceeded: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rate_limit_exceeded_total",
			Help:      "Number of times rate limit was exceeded",
		}),
		KafkaErrors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "kafka_errors_total",
			Help:      "Number of Kafka-related errors",
		}),
		WebSocketErrors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "websocket_errors_total",
			Help:      "Number of WebSocket-related errors",
		}),
	}
}
