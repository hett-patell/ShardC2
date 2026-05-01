package handlers

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

type WSMessage struct {
	Type      string      `json:"type"`
	BotID     string      `json:"bot_id,omitempty"`
	CommandID string      `json:"command_id,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

type WSHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]map[string]bool // conn -> set of subscribed bot IDs
}

func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[*websocket.Conn]map[string]bool),
	}
}

func (h *WSHub) Register(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[conn] = make(map[string]bool)
	h.mu.Unlock()
}

func (h *WSHub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
}

func (h *WSHub) Subscribe(conn *websocket.Conn, botID string) {
	h.mu.Lock()
	if subs, ok := h.clients[conn]; ok {
		subs[botID] = true
	}
	h.mu.Unlock()
}

func (h *WSHub) Unsubscribe(conn *websocket.Conn, botID string) {
	h.mu.Lock()
	if subs, ok := h.clients[conn]; ok {
		delete(subs, botID)
	}
	h.mu.Unlock()
}

func (h *WSHub) Broadcast(botID string, msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn, subs := range h.clients {
		if subs[botID] || subs["*"] {
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("[-] WS write error: %v", err)
			}
		}
	}
}

func WSUpgradeCheck() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	}
}

func WSHandler(hub *WSHub) func(*websocket.Conn) {
	return func(conn *websocket.Conn) {
		hub.Register(conn)
		defer func() {
			hub.Unregister(conn)
			conn.Close()
		}()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}

			var req struct {
				Action string `json:"action"`
				BotID  string `json:"bot_id"`
			}
			if json.Unmarshal(msg, &req) != nil {
				continue
			}

			switch req.Action {
			case "subscribe":
				if req.BotID != "" {
					hub.Subscribe(conn, req.BotID)
					conn.WriteJSON(WSMessage{Type: "subscribed", BotID: req.BotID})
				}
			case "unsubscribe":
				if req.BotID != "" {
					hub.Unsubscribe(conn, req.BotID)
					conn.WriteJSON(WSMessage{Type: "unsubscribed", BotID: req.BotID})
				}
			}
		}
	}
}
