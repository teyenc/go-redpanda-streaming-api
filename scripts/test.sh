#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

echo "Starting API test..."

# Check if server is running
echo -e "${GREEN}Checking if server is running...${NC}"
if ! nc -z localhost 8080; then
    echo -e "${RED}Server is not running on port 8080${NC}"
    echo "Please start the server first with: go run cmd/api/main.go"
    exit 1
fi

# Create a new stream
echo -e "${GREEN}Creating new stream...${NC}"
CURL_RESPONSE=$(curl -v -X POST http://localhost:8080/stream/start)
echo "Raw response: $CURL_RESPONSE"

STREAM_ID=$(echo $CURL_RESPONSE | jq -r '.stream_id')
echo "Parsed Stream ID: $STREAM_ID"

if [ -z "$STREAM_ID" ] || [ "$STREAM_ID" = "null" ]; then
    echo -e "${RED}Failed to get valid stream ID${NC}"
    exit 1
fi

# Wait a moment for stream setup
sleep 1

# Run the WebSocket client
echo -e "${GREEN}Starting WebSocket client...${NC}"
go run tests/ws_client.go -stream=$STREAM_ID