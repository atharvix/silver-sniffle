// smoke checks the live proximity path end to end with two accounts whose session tokens
// are given in SMOKE_A / SMOKE_B (env, never the command line): both fetch Bluetooth
// tokens, open sockets, A reports hearing B; each socket must get the other's card.
// Then B hides and must leave A's deck. Exit 1 on any failure.
//
//	SMOKE_A=… SMOKE_B=… smoke -api http://127.0.0.1:8080 -a uidA -b uidB
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var api = flag.String("api", "http://127.0.0.1:8080", "backend")

func call(method, path, tok string, body any) (int, []byte) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, *api+path, r)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func fail(f string, a ...any) {
	fmt.Printf("FAIL: "+f+"\n", a...)
	os.Exit(1)
}

// deck streams the socket's nearby lists.
func deck(tok string) chan []string {
	d := websocket.Dialer{Subprotocols: []string{"kinjo", tok}, HandshakeTimeout: 10 * time.Second}
	c, _, err := d.Dial("ws"+strings.TrimPrefix(*api, "http")+"/ws", nil)
	if err != nil {
		fail("socket: %v", err)
	}
	out := make(chan []string, 16)
	go func() {
		for {
			_, b, err := c.ReadMessage()
			if err != nil {
				return
			}
			var m struct {
				Type   string
				People []struct{ UID string }
			}
			if json.Unmarshal(b, &m) == nil && m.Type == "nearby" {
				var ids []string
				for _, p := range m.People {
					ids = append(ids, p.UID)
				}
				out <- ids
			}
		}
	}()
	return out
}

func wait(ch chan []string, uid string, want bool, limit time.Duration) time.Duration {
	start := time.Now()
	for {
		select {
		case ids := <-ch:
			has := false
			for _, id := range ids {
				has = has || id == uid
			}
			if has == want {
				return time.Since(start)
			}
		case <-time.After(limit - time.Since(start)):
			fail("%s %s within %v", uid, map[bool]string{true: "never showed", false: "never left"}[want], limit)
		}
	}
}

func main() {
	a, b := flag.String("a", "", "uid A"), flag.String("b", "", "uid B")
	flag.Parse()
	ta, tb := os.Getenv("SMOKE_A"), os.Getenv("SMOKE_B")
	if ta == "" || tb == "" || *a == "" || *b == "" {
		fail("set SMOKE_A, SMOKE_B, -a, -b")
	}
	token := func(s string) string {
		code, body := call("GET", "/api/ble-token", s, nil)
		var o struct{ Token string }
		if code != 200 || json.Unmarshal(body, &o) != nil || o.Token == "" {
			fail("ble-token: %d %s", code, body)
		}
		return o.Token
	}
	call("PUT", "/api/visibility", ta, map[string]bool{"visible": true})
	call("PUT", "/api/visibility", tb, map[string]bool{"visible": true})
	token(ta)
	bt := token(tb)
	da, db := deck(ta), deck(tb)
	time.Sleep(300 * time.Millisecond)

	t0 := time.Now()
	if code, body := call("POST", "/api/sightings", ta, map[string]any{"seen": []map[string]any{{"tok": bt, "rssi": -70}}}); code != 204 {
		fail("sightings: %d %s", code, body)
	}
	fmt.Printf("ok  A heard B -> B's card on A's socket in %v\n", wait(da, *b, true, 5*time.Second).Round(time.Millisecond))
	wait(db, *a, true, 5*time.Second)
	fmt.Printf("ok  A heard B -> A's card on B's socket by %v after the report (one phone hearing counts for both)\n", time.Since(t0).Round(time.Millisecond))

	if code, _ := call("PUT", "/api/visibility", tb, map[string]bool{"visible": false}); code != 200 {
		fail("hide: %d", code)
	}
	fmt.Printf("ok  B hid -> gone from A's deck in %v\n", wait(da, *b, false, 5*time.Second).Round(time.Millisecond))
	call("PUT", "/api/visibility", tb, map[string]bool{"visible": true})
	fmt.Println("PASS")
}
