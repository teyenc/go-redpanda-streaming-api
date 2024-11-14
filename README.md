# Real-Time Data Streaming API

A high-performance API built with Go and Redpanda for real-time data streaming.

## Prerequisites

- Go 1.20+
- Docker
- Docker Compose

## Project Structure

```
streaming-api/
├── cmd/
│   └── api/
│       └── main.go           # Main server
├── internal/
│   └── models/
│       └── stream.go         # Data models
├── pkg/
│   ├── auth/
│   │   └── handler.go        # Authentication
│   ├── kafka/
│   │   └── handler.go        # Kafka operations
│   └── websocket/
│       └── handler.go        # WebSocket handling
├── tests/
│   ├── benchmark_test.go     # Performance tests
│   ├── integration_test.go   # Integration tests
│   └── kafka_test.go         # Unit tests
└── docker-compose.yml        # Redpanda setup
```

## Setup

1. Clone the repository:
```bash
git clone https://github.com/teyenc/go-redpanda-streaming-api
cd streaming-api
```

2. Install dependencies:
```bash
go mod init streaming-api
go get github.com/gorilla/mux
go get github.com/gorilla/websocket
go get github.com/twmb/franz-go/pkg/kgo
go get github.com/google/uuid
go get golang.org/x/time/rate
go get github.com/stretchr/testify
```

3. Start Redpanda:
```bash
# Stop any existing instances
docker-compose down

# Start fresh instance
docker-compose up -d

# Verify it's running
docker ps | grep redpanda
```

4. Start the server:
```bash
go run cmd/api/main.go
```

## API Usage

1. Register a client:
```bash
curl -X POST http://localhost:8080/register
```
Response:
```json
{
  "client_id": "...",
  "api_key": "..."
}
```

2. Create a stream:
```bash
curl -X POST -H "X-API-Key: YOUR_API_KEY" http://localhost:8080/stream/start
```
Response:
```json
{
  "stream_id": "..."
}
```

3. Connect via WebSocket:
```javascript
const ws = new WebSocket('ws://localhost:8080/stream/YOUR_STREAM_ID/ws');
ws.onmessage = (event) => console.log(event.data);
```

## Running Tests

1. Unit Tests:
```bash
go test -v ./tests -run TestKafkaHandler
```

2. Integration Tests:
```bash
go test -v ./tests -run TestStreamIntegration
```

3. Performance Benchmark:
```bash
# Reset Redpanda first
docker-compose down
docker-compose up -d
sleep 5  # Wait for Redpanda to start

# Run benchmark
go test -v -run=XXX -bench=BenchmarkStreamPerformance ./tests
```
At this point you should see a lot of logs in the terminal that start the server.

This should be done in about 90 seconds, depending on the machine you use.

## Performance Metrics

The benchmark test measures:
- Concurrent connections (1000 streams)
- Message throughput
- Latency (min/max/average)
- Data transfer rates
- Success rates

Expected output includes:
- Active streams count
- Messages sent/received
- Average latency
- Throughput (msgs/sec)
- Data transfer rates (MB)

## Troubleshooting

1. If Redpanda connection fails:
```bash
# Check Redpanda logs
docker logs streaming-api_redpanda_1

# Verify port availability
netstat -an | grep 9092
```

2. If tests fail:
- Ensure Redpanda is running
- Check API server is running
- Verify no port conflicts
- Reset Redpanda before benchmarks
- Restart docker-compose

## Security Notes

- API keys are required for all endpoints
- Each client can only access their own streams
- WebSocket connections require authentication
- Rate limiting is enabled per stream
