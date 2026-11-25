// Command peer is a fake Kinjo user for device tests: it signs up, uploads a card and
// stands at a given spot, printing every deck it receives.
//
//	go run ./cmd/peer -lat 28.6139 -lng 77.2090 -name Asha
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

func main() {
	api := flag.String("api", "http://localhost:8001", "server base URL")
	lat := flag.Float64("lat", 0, "latitude to stand at")
	lng := flag.Float64("lng", 0, "longitude to stand at")
	name := flag.String("name", "Test Peer", "card name")
	flag.Parse()
	if *lat == 0 && *lng == 0 {
		log.Fatal("give -lat and -lng")
	}
	ctx := context.Background()

	var session struct{ UID, Token string }
	must(call(ctx, http.MethodPost, *api+"/api/session", "", nil, &session))
	log.Printf("signed up as %s", session.UID)

	must(call(ctx, http.MethodPut, *api+"/api/profile", session.Token, map[string]string{
		"name": *name, "role": "Testing Kinjo nearby", "looking": "Someone to check this works", "photo": testPhoto(),
	}, nil))

	ws := strings.Replace(*api, "http", "ws", 1) + "/ws"
	conn, _, err := websocket.Dial(ctx, ws, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + session.Token}}})
	must(err)
	defer conn.CloseNow()
	go func() {
		for {
			_, msg, err := conn.Read(ctx)
			if err != nil {
				log.Fatalf("socket closed: %v", err)
			}
			log.Printf("deck: %s", msg)
		}
	}()
	for {
		fix, _ := json.Marshal(map[string]any{"type": "fix", "lat": *lat, "lng": *lng, "acc": 5})
		must(conn.Write(ctx, websocket.MessageText, fix))
		time.Sleep(2 * time.Second)
	}
}

func call(ctx context.Context, method, url, token string, body, out any) error {
	var buf bytes.Buffer
	if body != nil {
		must(json.NewEncoder(&buf).Encode(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, &buf)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s", method, url, res.Status)
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

// testPhoto is a plain grey portrait, base64 JPEG.
func testPhoto() string {
	img := image.NewRGBA(image.Rect(0, 0, 500, 700))
	for y := range 700 {
		for x := range 500 {
			img.Set(x, y, color.RGBA{uint8(60 + y/12), 70, 90, 255})
		}
	}
	var buf bytes.Buffer
	must(jpeg.Encode(&buf, img, nil))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
