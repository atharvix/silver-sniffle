package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Waker sends silent high-priority FCM messages that restart a phone's presence service,
// so a phone whose app was swiped away reports a fresh position when someone is near it.
type Waker struct {
	http    *http.Client
	project string
	store   *Store
	queue   chan string
}

var errTokenGone = errors.New("push token no longer valid")

func newWaker(ctx context.Context, credentialsFile, project string, store *Store) (*Waker, error) {
	b, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, err
	}
	creds, err := google.CredentialsFromJSON(ctx, b, "https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		return nil, err
	}
	client := oauth2.NewClient(ctx, creds.TokenSource)
	client.Timeout = 10 * time.Second
	return &Waker{http: client, project: project, store: store, queue: make(chan string, 4096)}, nil
}

// enqueue never blocks the hub: when the queue is full the wake is dropped (the next fix nearby retries).
func (w *Waker) enqueue(uid string) {
	select {
	case w.queue <- uid:
	default:
		slog.Warn("wake queue full")
	}
}

func (w *Waker) run(ctx context.Context, workers int) {
	for range workers {
		go func() {
			for {
				select {
				case uid := <-w.queue:
					w.wake(ctx, uid)
				case <-ctx.Done():
					return
				}
			}
		}()
	}
}

func (w *Waker) wake(ctx context.Context, uid string) {
	tokens, err := w.store.deviceTokens(ctx, uid)
	if err != nil {
		slog.Error("wake: device tokens", "err", err)
		return
	}
	for _, token := range tokens {
		err := w.send(ctx, token)
		switch {
		case errors.Is(err, errTokenGone):
			_ = w.store.dropDevice(ctx, token)
		case err != nil:
			slog.Warn("wake failed", "err", err)
		}
	}
}

func (w *Waker) send(ctx context.Context, token string) error {
	body, _ := json.Marshal(map[string]any{"message": map[string]any{
		"token":   token,
		"data":    map[string]string{"type": "wake"},
		"android": map[string]any{"priority": "HIGH", "ttl": "30s"},
	}})
	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", w.project)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := w.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusOK {
		return nil
	}
	reply, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	// FCM answers 404 UNREGISTERED for uninstalled apps and 400 for malformed tokens: forget those.
	if res.StatusCode == http.StatusNotFound || (res.StatusCode == http.StatusBadRequest && bytes.Contains(reply, []byte("registration token"))) {
		return errTokenGone
	}
	return fmt.Errorf("fcm %d: %s", res.StatusCode, reply)
}
