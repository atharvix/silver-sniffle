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
	hub   *Hub
	app   *App
}

// inbound is the only message shape the client sends us (type: pos, hide, list, ping).
type inbound struct {
	Type string  `json:"type"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	Acc  int     `json:"acc"`
	Age  int64   `json:"age"` // ms since the phone took this fix (0 from old builds)
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
	c := &Client{
		uid:   uid,
		conn:  conn,
		send:  make(chan []byte, sendBuffer),
		cells: map[string]bool{},
		hub:   a.hub,
		app:   a,
	}
	a.hub.attach(c)
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
			c.hub.clearGone(c.uid) // a socket is ordered: this fix was sent after any hide on it
			c.app.applyPos(c.uid, msg.Lat, msg.Lng, msg.Acc, msg.Age)
		case "hide":
			c.handleHide()
		case "list": // the app reopened its nearby screen
			c.hub.notify(c, nil)
		case "ping": // the app's liveness check: protocol pings are invisible to JavaScript
			c.sendJSON(map[string]string{"type": "pong"})
		}
	}
}

// applyPos records a user's position from the WebSocket or the native-HTTP
// fallback, re-points their live client, and pushes fresh nearby lists to
// everyone who can now see them. The latest fix always wins, even a vague one:
// it then makes the person ineligible (see eligible) instead of leaving them
// pinned at an older, better spot they may have left. T is when the phone took
// the fix, so a re-sent old fix can't pass as fresh.
func (a *App) applyPos(uid string, lat, lng float64, acc int, age int64) {
	if !validCoord(lat, lng) {
		return
	}
	acc = max(0, min(acc, 999))
	age = max(0, min(age, int64(24*time.Hour/time.Millisecond)))
	p := &Pres{UID: uid, Cell: cellOf(lat, lng), Lat: round6(lat), Lng: round6(lng), Acc: acc,
		T: time.Now().Add(-time.Duration(age) * time.Millisecond)}
	a.hub.countFix(p)
	touched := a.hub.setPresence(p)
	if touched == nil {
		return // older than what we already have (requests raced)
	}
	c := a.hub.client(uid) // saved to Postgres in the next batch (Hub.save), not here
	if c != nil {
		first := c.pos == nil || time.Since(c.pos.T) > freshFor // attach held the list back until now
		a.hub.subscribe(c, p)
		if first { // the app is on its scan screen waiting for this: answer now, not on the next flush
			a.hub.recompute(c)
		}
	}
	a.hub.notify(c, touched)
}

// handleHide removes the user from the map and clears their deck.
func (c *Client) handleHide() {
	touched := c.hub.dropPresence(c.uid) // memory first (and the hide guard), then the saved row
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = c.hub.forget(ctx, c.uid)
	cancel()
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
