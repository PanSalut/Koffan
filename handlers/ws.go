package handlers

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gofiber/websocket/v2"
)

// WebSocket client connections
var (
	clients   = make(map[*websocket.Conn]*webSocketClient)
	clientsMu sync.RWMutex
)

// webSocketClient serializes every write to a connection. The websocket
// implementation supports one concurrent writer only; broadcasts and pong
// responses can otherwise overlap and panic.
type webSocketClient struct {
	conn    webSocketWriter
	writeMu sync.Mutex
}

type webSocketWriter interface {
	WriteJSON(interface{}) error
	WriteMessage(int, []byte) error
	Close() error
}

func (client *webSocketClient) writeJSON(value interface{}) error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	return client.conn.WriteJSON(value)
}

func (client *webSocketClient) writeMessage(messageType int, data []byte) error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	return client.conn.WriteMessage(messageType, data)
}

func (client *webSocketClient) close() error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	return client.conn.Close()
}

// WebSocketMessage represents a message sent to clients
type WebSocketMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// WebSocketHandler handles WebSocket connections
func WebSocketHandler(c *websocket.Conn) {
	client := &webSocketClient{conn: c}

	// Register client
	clientsMu.Lock()
	clients[c] = client
	clientCount := len(clients)
	clientsMu.Unlock()

	log.Printf("WebSocket client connected. Total clients: %d", clientCount)

	defer func() {
		// Unregister client
		clientsMu.Lock()
		delete(clients, c)
		clientCount := len(clients)
		clientsMu.Unlock()
		_ = client.close()
		log.Printf("WebSocket client disconnected. Total clients: %d", clientCount)
	}()

	// Keep connection alive and handle incoming messages
	for {
		messageType, msg, err := c.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle ping/pong
		if messageType == websocket.TextMessage {
			var message map[string]string
			if err := json.Unmarshal(msg, &message); err == nil {
				if message["type"] == "ping" {
					if err := client.writeJSON(map[string]string{"type": "pong"}); err != nil {
						log.Printf("Failed to send WebSocket pong: %v", err)
						break
					}
				}
			}
		}
	}
}

// BroadcastUpdate sends an update to all connected WebSocket clients
func BroadcastUpdate(eventType string, data interface{}) {
	message := WebSocketMessage{
		Type: eventType,
		Data: data,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal WebSocket message: %v", err)
		return
	}

	// Copy the clients while holding the map lock, then release it before any
	// network I/O. Each client has its own write lock, so concurrent broadcasts
	// remain safe without blocking registrations or disconnects globally.
	clientsMu.RLock()
	clientSnapshot := make([]*webSocketClient, 0, len(clients))
	for _, client := range clients {
		clientSnapshot = append(clientSnapshot, client)
	}
	clientsMu.RUnlock()

	clientCount := len(clientSnapshot)
	log.Printf("Broadcasting %s to %d clients", eventType, clientCount)

	successCount := 0
	for _, client := range clientSnapshot {
		err := client.writeMessage(websocket.TextMessage, messageBytes)
		if err != nil {
			log.Printf("Failed to send WebSocket message to client: %v", err)
			// Don't remove client here, let the read loop handle it
		} else {
			successCount++
		}
	}

	log.Printf("Broadcast %s completed: %d/%d clients received", eventType, successCount, clientCount)
}

// WebSocketUpgrade middleware to upgrade HTTP to WebSocket
func WebSocketUpgrade(c *websocket.Conn) error {
	return nil
}
