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

type wsClient struct {
	subs map[string]bool
	wmu  sync.Mutex
}

type WSHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]*wsClient
}

func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[*websocket.Conn]*wsClient),
	}
}

func (h *WSHub) Register(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[conn] = &wsClient{subs: make(map[string]bool)}
	h.mu.Unlock()
}

func (h *WSHub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
}

func (h *WSHub) Subscribe(conn *websocket.Conn, botID string) {
	h.mu.Lock()
	if cl, ok := h.clients[conn]; ok {
		cl.subs[botID] = true
	}
	h.mu.Unlock()
}

func (h *WSHub) Unsubscribe(conn *websocket.Conn, botID string) {
	h.mu.Lock()
	if cl, ok := h.clients[conn]; ok {
		delete(cl.subs, botID)
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

	for conn, cl := range h.clients {
		if cl.subs[botID] || cl.subs["*"] {
			cl.wmu.Lock()
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("[-] WS write error: %v", err)
			}
			cl.wmu.Unlock()
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

		writeJSON := func(v interface{}) {
			hub.mu.RLock()
			cl, ok := hub.clients[conn]
			hub.mu.RUnlock()
			if !ok {
				return
			}
			cl.wmu.Lock()
			conn.WriteJSON(v)
			cl.wmu.Unlock()
		}

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
					writeJSON(WSMessage{Type: "subscribed", BotID: req.BotID})
				}
			case "unsubscribe":
				if req.BotID != "" {
					hub.Unsubscribe(conn, req.BotID)
					writeJSON(WSMessage{Type: "unsubscribed", BotID: req.BotID})
				}
			}
		}
	}
}
