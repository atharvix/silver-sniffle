package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMessage = 4 * 1024 // app messages are tiny
	sendBuffer = 16
)

// Client is one open WebSocket connection for a signed-in user. It carries the
// nearby list, live notifications and visibility changes.
type Client struct {
	uid  string
	conn *websocket.Conn
	send chan []byte
	hub  *Hub
	app  *App
}

// inbound is the only message shape the app sends (type: pos, hide, list, ping).
type inbound struct {
	Type string  `json:"type"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	Acc  int     `json:"acc"`
	Age  int64   `json:"age"` // ms since the fix was taken
}

// serveWS authenticates and upgrades. Browsers can't set custom WebSocket headers,
// so the app sends its token as a subprotocol (["kinjo", token]): it travels in a
// header, not the URL, so it never lands in access logs. Older app builds still
// send ?token=.
func (a *App) serveWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	for _, p := range websocket.Subprotocols(r) {
		if p != "kinjo" {
			token = p
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	uid, err := a.store.uidForToken(ctx, token)
	cancel()
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if uid == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the error response
	}
	c := &Client{uid: uid, conn: conn, send: make(chan []byte, sendBuffer), hub: a.hub, app: a}
	if a.hub.isHidden(uid) { // hidden on another device: this app stops sharing too
		c.sendJSON(map[string]any{"type": "vis", "visible": false})
	}
	a.hub.attach(c)
	go c.writePump()
	c.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.detach(c)
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessage)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg inbound
		if json.Unmarshal(data, &msg) != nil {
			continue // ignore malformed frames rather than dropping the socket
		}
		switch msg.Type {
		case "pos": // browser fix; a socket is ordered, so it came after any hide on it
			c.hub.addFix(c.uid, msg.Lat, msg.Lng, msg.Acc, msg.Age, true, time.Now())
		case "hide":
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = c.hub.leave(ctx, c.uid)
			cancel()
		case "list": // the app reopened its nearby screen
			c.hub.markClient(c)
		case "ping": // the app's liveness check: protocol pings are invisible to JavaScript
			c.sendJSON(map[string]string{"type": "pong"})
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if c.conn.WriteMessage(websocket.TextMessage, msg) != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if c.conn.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		}
	}
}

// sendJSON queues a message; false when the buffer is full (a slow client). Never
// close(send): other goroutines may still send. Close the conn instead.
func (c *Client) sendJSON(v any) bool {
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	select {
	case c.send <- b:
		return true
	default:
		return false
	}
}
