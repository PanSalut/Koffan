package handlers

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gofiber/websocket/v2"
)

// WebSocket client connections
var (
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.RWMutex
)

// WebSocketMessage represents a message sent to clients
type WebSocketMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// WebSocketHandler handles WebSocket connections
func WebSocketHandler(c *websocket.Conn) {
	// Register client
	clientsMu.Lock()
	clients[c] = true
	clientsMu.Unlock()

	log.Printf("WebSocket client connected. Total clients: %d", len(clients))

	defer func() {
		// Unregister client
		clientsMu.Lock()
		delete(clients, c)
		clientsMu.Unlock()
		c.Close()
		log.Printf("WebSocket client disconnected. Total clients: %d", len(clients))
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
					c.WriteJSON(map[string]string{"type": "pong"})
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

	clientsMu.RLock()
	clientCount := len(clients)
	log.Printf("Broadcasting %s to %d clients", eventType, clientCount)

	successCount := 0
	for client := range clients {
		err := client.WriteMessage(websocket.TextMessage, messageBytes)
		if err != nil {
			log.Printf("Failed to send WebSocket message to client: %v", err)
			// Don't remove client here, let the read loop handle it
		} else {
			successCount++
		}
	}
	clientsMu.RUnlock()

	log.Printf("Broadcast %s completed: %d/%d clients received", eventType, successCount, clientCount)
}

// WebSocketUpgrade middleware to upgrade HTTP to WebSocket
func WebSocketUpgrade(c *websocket.Conn) error {
	return nil
}
