package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMessage = 4 * 1024 // position updates are tiny
	sendBuffer = 16
)

// Client is one open WebSocket connection for a signed-in user.
type Client struct {
	uid   string
	conn  *websocket.Conn
	send  chan []byte
	pos   *Pres           // last reported position (nil = not currently sharing)
	cells map[string]bool // cells this client is subscribed to
	shown map[string]bool // uids currently on this client's deck (hysteresis)
	hub   *Hub
	app   *App
}

// inbound is the only message shape the client sends us.
type inbound struct {
	Type string  `json:"type"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	Acc  int     `json:"acc"`
}

// serveWS authenticates via the ?token= query param (browsers can't set custom
// WebSocket headers), upgrades the connection, and starts the pumps.
func (a *App) serveWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	uid, err := a.store.uidForToken(ctx, token)
	cancel()
	if err != nil || uid == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the error response
	}
	c := &Client{
		uid:   uid,
		conn:  conn,
		send:  make(chan []byte, sendBuffer),
		cells: map[string]bool{},
		shown: map[string]bool{},
		hub:   a.hub,
		app:   a,
	}
	// One live connection per user: replace any older one.
	a.hub.mu.Lock()
	if old := a.hub.clients[uid]; old != nil {
		close(old.send)
	}
	a.hub.clients[uid] = c
	a.hub.mu.Unlock()

	go c.writePump()
	c.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unsubscribe(c)
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
		case "pos":
			c.app.applyPos(c.uid, msg.Lat, msg.Lng, msg.Acc)
		case "hide":
			c.handleHide()
		}
	}
}

// freshFix is how long a fix stays trusted over a much less accurate newcomer.
const freshFix = time.Minute

// applyPos records a user's position from the WebSocket or the native-HTTP
// fallback, re-points their live client, and pushes fresh nearby lists to
// everyone who can now see them. A coarse fix (screen off: Wi-Fi/cell
// location) never displaces a fresh, much better one; it only refreshes the
// timestamp, so a phone that's still there stays visible.
func (a *App) applyPos(uid string, lat, lng float64, acc int) {
	if !validCoord(lat, lng) {
		return
	}
	now := time.Now()
	acc = max(0, min(acc, 999))
	p := &Pres{UID: uid, Cell: cellOf(lat, lng), Lat: round6(lat), Lng: round6(lng), Acc: acc, T: now}
	if prev := a.hub.position(uid); prev != nil && worse(acc, prev, now) {
		kept := *prev
		kept.T = now
		p = &kept
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := a.store.upsertPresence(ctx, uid, *p); err != nil {
		log.Printf("presence write failed for %s: %v", uid, err) // memory still updates
	}
	cancel()
	touched := a.hub.setPresence(p)
	c := a.hub.client(uid)
	if c != nil {
		a.hub.subscribe(c, p)
	}
	a.hub.notify(c, touched)
}

// worse reports whether a new fix should lose to the stored one: it's unusable,
// or the stored fix is fresh and the new one is far less accurate.
func worse(acc int, prev *Pres, now time.Time) bool {
	return acc > badAccM || (now.Sub(prev.T) < freshFix && acc > 2*max(prev.Acc, 10))
}

// handleHide removes the user from the map and clears their deck.
func (c *Client) handleHide() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = c.app.store.deletePresence(ctx, c.uid)
	cancel()
	touched := c.hub.dropPresence(c.uid)
	c.hub.subscribe(c, nil) // stop listening
	c.sendJSON(map[string]any{"type": "nearby", "people": []nearbyPerson{}})
	c.hub.notify(nil, touched) // let others drop this person
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok { // hub closed the channel (replaced or shutting down)
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
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

// sendJSON queues a message. If the buffer is full the client is too slow; we
// drop this update rather than block the hub — the next recompute re-syncs it.
func (c *Client) sendJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case c.send <- b:
	default:
	}
}

func validCoord(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 && !(lat == 0 && lng == 0)
}

func round6(v float64) float64 {
	return float64(int64(v*1e6+sign(v)*0.5)) / 1e6
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}
