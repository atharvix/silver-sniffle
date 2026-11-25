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

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	wakeSubject = "kinjo.wake"
	wakeStream  = "KINJO_WAKE"
)

// openWakeStream creates the JetStream work queue for wakes. A wake older than two minutes is useless, so it expires.
func openWakeStream(ctx context.Context, js jetstream.JetStream) error {
	_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      wakeStream,
		Subjects:  []string{wakeSubject},
		Retention: jetstream.WorkQueuePolicy,
		Storage:   jetstream.FileStorage,
		MaxAge:    2 * time.Minute,
	})
	return err
}

// queueWake is the hub's wake function: a core publish never blocks the hub, and JetStream keeps it until a pusher takes it.
func queueWake(nc *nats.Conn) func(string) {
	return func(uid string) {
		if err := nc.Publish(wakeSubject, []byte(uid)); err != nil {
			slog.Warn("queue wake", "err", err)
			return
		}
		metricWakes.WithLabelValues("queued").Inc()
	}
}

// Pusher takes wakes off the queue and sends silent high-priority FCM messages that restart a phone's
// presence service, so a phone whose app was swiped away reports a fresh position. Any number of servers
// can run one: the durable consumer hands each wake to exactly one of them.
type Pusher struct {
	http    *http.Client
	project string
	store   *Store
}

var errTokenGone = errors.New("push token no longer valid")

func newPusher(ctx context.Context, credentialsFile, project string, store *Store) (*Pusher, error) {
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
	return &Pusher{http: client, project: project, store: store}, nil
}

// run sends wakes with up to `workers` in flight until ctx ends.
func (p *Pusher) run(ctx context.Context, js jetstream.JetStream, workers int) error {
	consumer, err := js.CreateOrUpdateConsumer(ctx, wakeStream, jetstream.ConsumerConfig{
		Durable:    "pusher",
		AckPolicy:  jetstream.AckExplicitPolicy,
		AckWait:    30 * time.Second,
		MaxDeliver: 3,
	})
	if err != nil {
		return err
	}
	slots := make(chan struct{}, workers)
	consuming, err := consumer.Consume(func(msg jetstream.Msg) {
		slots <- struct{}{}
		go func() {
			defer func() { <-slots }()
			if err := p.wake(ctx, string(msg.Data())); err != nil {
				slog.Warn("wake failed, will retry", "err", err)
				_ = msg.NakWithDelay(5 * time.Second)
				return
			}
			_ = msg.Ack()
		}()
	})
	if err != nil {
		return err
	}
	<-ctx.Done()
	consuming.Stop()
	return nil
}

// wake pushes to every phone of the account. It returns an error only for failures worth retrying.
func (p *Pusher) wake(ctx context.Context, uid string) error {
	tokens, err := p.store.deviceTokens(ctx, uid)
	if err != nil {
		return err
	}
	var retry error
	for _, token := range tokens {
		err := p.send(ctx, token)
		switch {
		case err == nil:
			metricWakes.WithLabelValues("sent").Inc()
		case errors.Is(err, errTokenGone):
			metricWakes.WithLabelValues("gone").Inc()
			_ = p.store.dropDevice(ctx, token)
		default:
			metricWakes.WithLabelValues("failed").Inc()
			retry = err
		}
	}
	return retry
}

func (p *Pusher) send(ctx context.Context, token string) error {
	body, _ := json.Marshal(map[string]any{"message": map[string]any{
		"token":   token,
		"data":    map[string]string{"type": "wake"},
		"android": map[string]any{"priority": "HIGH", "ttl": "30s"},
	}})
	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", p.project)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := p.http.Do(req)
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
