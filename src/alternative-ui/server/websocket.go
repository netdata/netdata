// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/src/alternative-ui/metrics"
)

const (
	websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	conn       net.Conn
	hub        *WebSocketHub
	send       chan []byte
	subscribed map[string]bool // node IDs subscribed to
	mu         sync.RWMutex
}

// WebSocketHub manages WebSocket connections
type WebSocketHub struct {
	store      *metrics.Store
	clients    map[*WebSocketClient]bool
	broadcast  chan []byte
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	mu         sync.RWMutex
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub(store *metrics.Store) *WebSocketHub {
	return &WebSocketHub{
		store:      store,
		clients:    make(map[*WebSocketClient]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
	}
}

// Run starts the hub's main loop
func (h *WebSocketHub) Run() {
	// Subscribe to store updates
	updates := h.store.Subscribe()
	defer h.store.Unsubscribe(updates)

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("WebSocket client connected, total: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("WebSocket client disconnected, total: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()

		case msg := <-updates:
			// Broadcast store updates to all clients
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			frame := makeWebSocketFrame(data)
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- frame:
				default:
					// Skip slow clients
				}
			}
			h.mu.RUnlock()
		}
	}
}

// HandleConnection handles a new WebSocket connection
func (h *WebSocketHub) HandleConnection(w http.ResponseWriter, r *http.Request) {
	// Check for WebSocket upgrade
	if r.Header.Get("Upgrade") != "websocket" {
		http.Error(w, "Expected websocket upgrade", http.StatusBadRequest)
		return
	}

	// Perform WebSocket handshake
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "Missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	// Calculate accept key
	h1 := sha1.New()
	h1.Write([]byte(key + websocketGUID))
	acceptKey := base64.StdEncoding.EncodeToString(h1.Sum(nil))

	// Hijack the connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket upgrade failed", http.StatusInternalServerError)
		return
	}

	conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "WebSocket upgrade failed", http.StatusInternalServerError)
		return
	}

	// Send handshake response
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"
	conn.Write([]byte(response))

	// Create client
	client := &WebSocketClient{
		conn:       conn,
		hub:        h,
		send:       make(chan []byte, 256),
		subscribed: make(map[string]bool),
	}

	h.register <- client

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()

	// Send initial data
	nodes := h.store.GetNodes()
	initMsg := metrics.WebSocketMessage{
		Type:    "init",
		Payload: nodes,
	}
	data, _ := json.Marshal(initMsg)
	client.send <- makeWebSocketFrame(data)
}

// readPump reads messages from the WebSocket connection
func (c *WebSocketClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	for {
		// Read frame header
		header := make([]byte, 2)
		_, err := c.conn.Read(header)
		if err != nil {
			return
		}

		// Reset deadline
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// Parse frame
		fin := (header[0] & 0x80) != 0
		opcode := header[0] & 0x0f
		masked := (header[1] & 0x80) != 0
		payloadLen := int(header[1] & 0x7f)

		// Handle extended payload length
		if payloadLen == 126 {
			ext := make([]byte, 2)
			c.conn.Read(ext)
			payloadLen = int(ext[0])<<8 | int(ext[1])
		} else if payloadLen == 127 {
			ext := make([]byte, 8)
			c.conn.Read(ext)
			payloadLen = int(ext[4])<<24 | int(ext[5])<<16 | int(ext[6])<<8 | int(ext[7])
		}

		// Read mask if present
		var mask []byte
		if masked {
			mask = make([]byte, 4)
			c.conn.Read(mask)
		}

		// Read payload
		payload := make([]byte, payloadLen)
		if payloadLen > 0 {
			c.conn.Read(payload)
		}

		// Unmask payload
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}

		// Handle opcode
		switch opcode {
		case 0x1: // Text frame
			if fin {
				c.handleMessage(payload)
			}
		case 0x8: // Close
			return
		case 0x9: // Ping
			c.send <- makeWebSocketFrame(payload) // Send pong
		case 0xA: // Pong
			// Ignore
		}
	}
}

// writePump writes messages to the WebSocket connection
func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// Hub closed the channel
				c.conn.Write(makeCloseFrame())
				return
			}
			c.conn.Write(message)

		case <-ticker.C:
			// Send ping
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			c.conn.Write(makePingFrame())
		}
	}
}

// handleMessage processes incoming messages from client
func (c *WebSocketClient) handleMessage(data []byte) {
	var msg struct {
		Type    string   `json:"type"`
		NodeIDs []string `json:"node_ids,omitempty"`
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "subscribe":
		c.mu.Lock()
		for _, id := range msg.NodeIDs {
			c.subscribed[id] = true
		}
		c.mu.Unlock()

	case "unsubscribe":
		c.mu.Lock()
		for _, id := range msg.NodeIDs {
			delete(c.subscribed, id)
		}
		c.mu.Unlock()

	case "ping":
		response := metrics.WebSocketMessage{Type: "pong"}
		data, _ := json.Marshal(response)
		c.send <- makeWebSocketFrame(data)
	}
}

// makeWebSocketFrame creates a WebSocket text frame
func makeWebSocketFrame(payload []byte) []byte {
	length := len(payload)
	var frame []byte

	if length < 126 {
		frame = make([]byte, 2+length)
		frame[0] = 0x81 // FIN + text opcode
		frame[1] = byte(length)
		copy(frame[2:], payload)
	} else if length < 65536 {
		frame = make([]byte, 4+length)
		frame[0] = 0x81
		frame[1] = 126
		frame[2] = byte(length >> 8)
		frame[3] = byte(length)
		copy(frame[4:], payload)
	} else {
		frame = make([]byte, 10+length)
		frame[0] = 0x81
		frame[1] = 127
		frame[6] = byte(length >> 24)
		frame[7] = byte(length >> 16)
		frame[8] = byte(length >> 8)
		frame[9] = byte(length)
		copy(frame[10:], payload)
	}

	return frame
}

// makeCloseFrame creates a WebSocket close frame
func makeCloseFrame() []byte {
	return []byte{0x88, 0x00}
}

// makePingFrame creates a WebSocket ping frame
func makePingFrame() []byte {
	return []byte{0x89, 0x00}
}
