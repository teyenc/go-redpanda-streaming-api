// pkg/websocket/handler.go

package websocket

import (
	"context"
	"log"
	"net/http"
	"streaming-api/internal/models"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096, // Increased buffer size for high volume
	WriteBufferSize: 4096, // Increased buffer size for high volume
	CheckOrigin: func(r *http.Request) bool {
		return true // Implement origin check for production
	},
}

type Handler struct {
	MessageHandler func([]byte) error
}

func NewHandler(messageHandler func([]byte) error) *Handler {
	return &Handler{
		MessageHandler: messageHandler,
	}
}

func (h *Handler) HandleConnection(stream *models.Stream, w http.ResponseWriter, r *http.Request) error {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	stream.WebSocket = conn

	// Start read/write pumps
	go h.readPump(stream)
	go h.writePump(stream)

	return nil
}

func (h *Handler) readPump(stream *models.Stream) {
	defer func() {
		stream.WebSocket.Close()
		close(stream.Done)
	}()

	stream.WebSocket.SetReadLimit(512 * 1024) // 512KB max message size
	stream.WebSocket.SetReadDeadline(time.Now().Add(60 * time.Second))
	// stream.WebSocket.SetReadDeadline(time.Now().Add(60 * time.Second))
	stream.WebSocket.SetPongHandler(func(string) error {
		stream.WebSocket.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := stream.WebSocket.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			return
		}

		if err := stream.RateLimiter.Wait(context.Background()); err != nil {
			log.Printf("Rate limit exceeded for stream %s", stream.ID)
			continue
		}

		if err := h.MessageHandler(message); err != nil {
			log.Printf("Failed to handle message: %v", err)
			continue
		}
	}
}

func (h *Handler) writePump(stream *models.Stream) {
	ticker := time.NewTicker(time.Second)
	defer func() {
		ticker.Stop()
		stream.WebSocket.Close()
	}()

	for {
		select {
		case <-stream.Done:
			return
		case message := <-stream.ProcessedData:
			stream.WebSocket.SetWriteDeadline(time.Now().Add(60 * time.Second))
			if err := stream.WebSocket.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}
		case <-ticker.C:
			stream.WebSocket.SetWriteDeadline(time.Now().Add(60 * time.Second))
			if err := stream.WebSocket.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
