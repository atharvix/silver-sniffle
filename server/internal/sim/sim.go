// Package sim is a pretend phone for tests: it does what the Kinjo app does, over the real API and socket.
package sim

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

var client = &http.Client{Timeout: 30 * time.Second}

// Photo is a small grey portrait, base64 JPEG, made once.
var Photo = func() string {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 300, 420)), nil); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}()

// Phone is one simulated person.
type Phone struct {
	API, Token, UID string

	mu          sync.Mutex
	conn        *websocket.Conn // nil when not connected
	deck        []string        // card names in the latest deck
	msgs, bytes int64
}

// SignUp creates an anonymous account, like the app's first Continue.
func SignUp(ctx context.Context, api string) (*Phone, error) {
	p := &Phone{API: api}
	var s struct{ UID, Token string }
	var err error
	for attempt := range 3 { // the app tries 3 times, spread out, when it can't reach the server
		if err = p.call(ctx, http.MethodPost, "/api/session", nil, &s); err == nil {
			break
		}
		time.Sleep(time.Duration(300*(attempt+1)+rand.IntN(500)) * time.Millisecond)
	}
	if err != nil {
		return nil, err
	}
	p.UID, p.Token = s.UID, s.Token
	return p, nil
}

// SaveCard saves (or edits) the card, agreeing to the terms like the app does.
func (p *Phone) SaveCard(ctx context.Context, name, role, looking string) error {
	return p.call(ctx, http.MethodPut, "/api/profile", map[string]any{
		"name": name, "role": role, "looking": looking, "photo": Photo, "agreed": true,
	}, nil)
}

// Connect opens the live socket and keeps the latest deck.
func (p *Phone) Connect(ctx context.Context) error {
	ws := strings.Replace(p.API, "http", "ws", 1) + "/ws"
	conn, _, err := websocket.Dial(ctx, ws, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + p.Token}}})
	if err != nil {
		return err
	}
	conn.SetReadLimit(4 << 20)
	p.mu.Lock()
	p.conn = conn
	p.mu.Unlock()
	go func() {
		for {
			_, msg, err := conn.Read(context.Background())
			if err != nil {
				p.mu.Lock()
				if p.conn == conn {
					p.conn = nil // the server dropped us: Report now fails
				}
				p.mu.Unlock()
				return
			}
			var m struct {
				Type  string
				Cards []struct{ Name string }
			}
			if json.Unmarshal(msg, &m) != nil {
				continue
			}
			p.mu.Lock()
			p.msgs++
			p.bytes += int64(len(msg))
			if m.Type == "deck" {
				p.deck = p.deck[:0]
				for _, c := range m.Cards {
					p.deck = append(p.deck, c.Name)
				}
			}
			p.mu.Unlock()
		}
	}()
	return nil
}

// Report sends a position like the presence service does.
func (p *Phone) Report(ctx context.Context, lat, lng, acc float64, still bool) error {
	return p.send(ctx, map[string]any{"type": "fix", "lat": lat, "lng": lng, "acc": acc, "still": still})
}

// Send writes any message, including malformed ones for robustness tests.
func (p *Phone) Send(ctx context.Context, raw []byte) error {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	return conn.Write(ctx, websocket.MessageText, raw)
}

func (p *Phone) send(ctx context.Context, v any) error {
	b, _ := json.Marshal(v)
	return p.Send(ctx, b)
}

// Traffic is how many messages and bytes the server has sent this phone.
func (p *Phone) Traffic() (msgs, bytes int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.msgs, p.bytes
}

// Sees reports the names on the phone's screen right now.
func (p *Phone) Sees() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.deck)
}

// WaitUntil polls the screen until ok returns true or the time runs out; it returns how long it took.
func (p *Phone) WaitUntil(ok func(names []string) bool, within time.Duration) (time.Duration, bool) {
	start := time.Now()
	for time.Since(start) < within {
		if ok(p.Sees()) {
			return time.Since(start), true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return within, false
}

// SetVisible is the "Show my profile" switch.
func (p *Phone) SetVisible(ctx context.Context, visible bool) error {
	return p.call(ctx, http.MethodPut, "/api/visibility", map[string]any{"visible": visible}, nil)
}

func (p *Phone) Block(ctx context.Context, uid string) error {
	return p.call(ctx, http.MethodPost, "/api/block", map[string]any{"uid": uid}, nil)
}

func (p *Phone) Delete(ctx context.Context) error {
	return p.call(ctx, http.MethodDelete, "/api/account", nil, nil)
}

// Drop closes the socket abruptly, like a phone losing signal or being killed.
func (p *Phone) Drop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		p.conn.CloseNow()
		p.conn = nil
	}
}

func (p *Phone) call(ctx context.Context, method, path string, body, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, p.API+path, &buf)
	if err != nil {
		return err
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s", method, path, res.Status)
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}
