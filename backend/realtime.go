package main

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMessage = 4 * 1024 // app messages are tiny
	sendBuffer = 16
	// a reporter may say nothing for long (a still phone asleep in Doze): nginx's TCP keepalive
	// closes a dead one's socket; this only catches a stuck one, just past nginx's 1 h
	reporterWait = 65 * time.Minute
)

// Client is one open WebSocket connection for a signed-in user. It carries the
// nearby list, live notifications and visibility changes.
type Client struct {
	uid      string
	conn     *websocket.Conn
	send     chan []byte
	hub      *Hub
	app      *App
	reporter bool // the phone service's socket: fixes in, mode out, no decks and no server pings

	mu        sync.Mutex
	delta     bool                    // the app said hello {delta:true}: changes only, nothing when nothing changed
	last      map[string]nearbyPerson // the deck it has (nil: send the whole list next)
	lastOrder []string
}

// sendDeck sends a deck: the whole list, or for a delta client only what changed
// ({type:"delta", add, del, order}) and nothing at all when nothing did. False when the buffer
// is full; the next send is then a whole list.
func (c *Client) sendDeck(people []nearbyPerson, whole bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	order := make([]string, len(people))
	now := make(map[string]nearbyPerson, len(people))
	for i, p := range people {
		order[i], now[p.UID] = p.UID, p
	}
	if whole || !c.delta || c.last == nil {
		ok := c.sendJSON(map[string]any{"type": "nearby", "people": people})
		c.last, c.lastOrder = now, order
		if !ok {
			c.last = nil
		}
		return ok
	}
	var add []nearbyPerson
	for _, p := range people {
		if old, ok := c.last[p.UID]; !ok || old != p {
			add = append(add, p)
		}
	}
	var del []string
	for uid := range c.last {
		if _, ok := now[uid]; !ok {
			del = append(del, uid)
		}
	}
	if len(add) == 0 && len(del) == 0 && slices.Equal(order, c.lastOrder) {
		return true
	}
	ok := c.sendJSON(map[string]any{"type": "delta", "add": add, "del": del, "order": order})
	c.last, c.lastOrder = now, order
	if !ok {
		c.last = nil
	}
	return ok
}

// new sockets per second (bucket): after a restart every phone redials; past this they get
// 503 and their jittered backoff spreads them out. Each costs one indexed session lookup:
// 20,000 phones redialing at once were back in ~2 min at 300/s (loadv3), seconds at this.
const wsPerSecond, wsBurst = 1500, 3000

type bucket struct {
	mu                  sync.Mutex
	rate, burst, tokens float64
	at                  time.Time
}

func newBucket(rate, burst float64) *bucket {
	return &bucket{rate: rate, burst: burst, tokens: burst, at: time.Now()}
}

func (b *bucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.tokens = min(b.burst, b.tokens+now.Sub(b.at).Seconds()*b.rate)
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// inbound is the only message shape the app sends (type: pos, hide, list, ping).
type inbound struct {
	Type  string   `json:"type"`
	Lat   float64  `json:"lat"`
	Lng   float64  `json:"lng"`
	Acc   int      `json:"acc"`
	Age   int64    `json:"age"`   // ms since the fix was taken
	Still bool     `json:"still"` // the phone isn't moving: keep it placed while it reports
	Wifi  []string `json:"wifi"`  // hashed Wi-Fi network ids seen with the fix (8 hex chars each)
	Delta bool     `json:"delta"` // hello: the app takes delta decks
	Spd   *float64 `json:"spd"`   // m/s, when the phone knows (for prediction)
	Hdg   *float64 `json:"hdg"`   // degrees from north
}

// wifiIDs parses the phone's hashed network ids: at most wifiMax, sorted, no repeats.
func wifiIDs(in []string) []uint32 {
	if len(in) == 0 {
		return nil
	}
	out := make([]uint32, 0, min(len(in), wifiMax))
	for _, s := range in {
		if len(out) == wifiMax {
			break
		}
		if v, err := strconv.ParseUint(s, 16, 32); err == nil && len(s) == 8 {
			out = append(out, uint32(v))
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (m inbound) report() report {
	r := report{lat: m.Lat, lng: m.Lng, acc: m.Acc, age: m.Age, still: m.Still, wifi: wifiIDs(m.Wifi), spd: -1, hdg: -1}
	if m.Spd != nil && m.Hdg != nil {
		r.spd, r.hdg = *m.Spd, *m.Hdg
	}
	return r
}

// serveWS authenticates and upgrades. Browsers can't set custom WebSocket headers,
// so the app sends its token as a subprotocol (["kinjo", token]): it travels in a
// header, not the URL, so it never lands in access logs. Older app builds still
// send ?token=.
func (a *App) serveWS(w http.ResponseWriter, r *http.Request) {
	if !a.wsLimit.allow() {
		w.Header().Set("Retry-After", "2")
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	token := r.URL.Query().Get("token")
	reporter := false
	for _, p := range websocket.Subprotocols(r) {
		switch p {
		case "kinjo":
		case "reporter": // the phone's own service (["kinjo", token, "reporter"])
			reporter = true
		default:
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
	c := &Client{uid: uid, conn: conn, send: make(chan []byte, sendBuffer), hub: a.hub, app: a, reporter: reporter}
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
	wait := pongWait
	if c.reporter {
		wait = reporterWait // no server pings (each would wake the phone's radio): its own reports keep it open
	}
	_ = c.conn.SetReadDeadline(time.Now().Add(wait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if c.reporter {
			_ = c.conn.SetReadDeadline(time.Now().Add(wait))
		}
		var msg inbound
		if json.Unmarshal(data, &msg) != nil {
			continue // ignore malformed frames rather than dropping the socket
		}
		switch msg.Type {
		case "pos": // a fix (browser or the phone's service); a socket is ordered, so it came after any hide on it
			r := msg.report()
			r.held = c.reporter
			c.hub.enqueue(c, r, time.Now()) // applyLoop applies it (latest wins) and answers a reporter's mode
		case "hello": // the app's features: {delta:true} = changes only
			c.mu.Lock()
			c.delta = msg.Delta
			c.mu.Unlock()
		case "hide":
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = c.hub.leave(ctx, c.uid)
			cancel()
		case "list": // the app reopened its nearby screen and waits for a list: the whole one, even if unchanged
			c.mu.Lock()
			c.last = nil
			c.mu.Unlock()
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
	tick := ticker.C
	if c.reporter {
		tick = nil
	}
	for {
		select {
		case msg := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if c.conn.WriteMessage(websocket.TextMessage, msg) != nil {
				return
			}
		case <-tick:
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
