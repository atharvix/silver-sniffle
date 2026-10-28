package main

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"strings"
	"sync"
	"time"
)

// App Review's account (DEMO_EMAIL): it signs in with the fixed DEMO_CODE instead of an
// emailed code, and its deck always carries the sample cards below, wherever the
// reviewer is. The sample people (uid demo_*) never have a position or a Bluetooth
// token, so nobody else ever sees them. Hiding the demo account hides them too.

//go:embed demo/*.jpg
var demoPhotos embed.FS

const demoPrefix = "demo_"

var demoCards = []struct{ uid, name, role, look, photo string }{
	{"demo_tara", "Tara Kapoor", "Associate @ Northfield Ventures",
		"Meeting pre-seed founders in consumer and fintech this week.", "tara.jpg"},
	{"demo_ashna", "Ashna", "Founder, healthcare AI startup",
		"Founder building a healthcare AI startup, pre-seed raised, looking to connect with other founders + investors to share ideas and opportunities.", "ashna.jpg"},
	{"demo_nhan", "Nhan S", "Software Engineer",
		"Software engineer interested in projects or joining an early-stage startup, open to any industry.", "nhan.jpg"},
}

// seedDemo writes the sample people and returns the demo account's uid (created
// if it doesn't exist yet, already verified like any email sign-in).
func (s *Store) seedDemo(ctx context.Context, email string) (string, error) {
	for _, d := range demoCards {
		b, err := demoPhotos.ReadFile("demo/" + d.photo)
		if err != nil {
			return "", err
		}
		if _, err := s.pool.Exec(ctx, `INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`, d.uid); err != nil {
			return "", err
		}
		p := Profile{Name: d.name, Role: d.role, Look: d.look, Photo: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b)}
		if err := s.saveProfile(ctx, d.uid, p); err != nil {
			return "", err
		}
	}
	uid, _, err := s.emailUser(ctx, email)
	return uid, err
}

func (a *App) isDemo(email string) bool { return a.cfg.DemoEmail != "" && email == a.cfg.DemoEmail }

// The demo code is fixed, so wrong guesses are capped for everyone together:
// demoTries per hour, then the demo sign-in is closed until the hour is up.
const demoTries = 20

var demoFails = struct {
	sync.Mutex
	t []time.Time
}{}

func (a *App) checkDemoCode(code string, now time.Time) string {
	demoFails.Lock()
	defer demoFails.Unlock()
	recent := demoFails.t[:0]
	for _, t := range demoFails.t {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	demoFails.t = recent
	if len(recent) >= demoTries {
		return "too_many"
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(code)), []byte(a.cfg.DemoCode)) == 1 {
		return ""
	}
	demoFails.t = append(demoFails.t, now)
	return "wrong"
}

// setDemo names the account whose deck gets the sample cards.
func (h *Hub) setDemo(uid string) {
	h.mu.Lock()
	h.demo = uid
	h.mu.Unlock()
}
