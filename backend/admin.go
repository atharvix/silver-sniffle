package main

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// requireAdmin guards the admin API with the shared ADMIN_TOKEN. The whole API
// is off when the token is unset, so a misconfigured server can't be probed.
func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.AdminToken == "" {
			writeJSON(w, http.StatusForbidden, errBody("admin disabled"))
			return
		}
		got := r.Header.Get("X-Admin-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(a.cfg.AdminToken)) != 1 {
			writeJSON(w, http.StatusUnauthorized, errBody("unauthorized"))
			return
		}
		next(w, r)
	}
}

// handleAdminMetrics returns DB roll-ups plus live server telemetry.
func (a *App) handleAdminMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := a.store.metrics(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("metrics failed"))
		return
	}
	conn, live := a.hub.stats()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	writeJSON(w, http.StatusOK, map[string]any{
		"db": m,
		"telemetry": map[string]any{
			"connectedClients": conn,
			"livePresence":     live,
			"uptimeSeconds":    int(time.Since(a.startedAt).Seconds()),
			"startedAt":        a.startedAt,
			"goroutines":       runtime.NumGoroutine(),
			"memAllocMB":       ms.Alloc / (1024 * 1024),
			"fcmEnabled":       a.fcm != nil,
			"smtpEnabled":      a.cfg.smtpConfigured(),
		},
	})
}

func (a *App) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	users, err := a.store.listUsers(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("list failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

type adminEmailReq struct {
	UID       string `json:"uid"`
	Email     string `json:"email"`
	Broadcast bool   `json:"broadcast"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// handleAdminEmail sends a custom email to one user or broadcasts to everyone.
func (a *App) handleAdminEmail(w http.ResponseWriter, r *http.Request) {
	var in adminEmailReq
	if err := decodeJSON(r, &in); err != nil || in.Subject == "" || in.Body == "" {
		writeJSON(w, http.StatusBadRequest, errBody("subject and body required"))
		return
	}

	var recipients []string
	switch {
	case in.Broadcast:
		rs, err := a.store.allEmails(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody("lookup failed"))
			return
		}
		recipients = rs
	case in.Email != "":
		recipients = []string{in.Email}
	case in.UID != "":
		e, err := a.store.emailForUID(r.Context(), in.UID)
		if err != nil || e == "" {
			writeJSON(w, http.StatusBadRequest, errBody("user has no email"))
			return
		}
		recipients = []string{e}
	default:
		writeJSON(w, http.StatusBadRequest, errBody("set broadcast, email, or uid"))
		return
	}

	// Broadcasts can be large and slow (SMTP); send them in the background.
	if in.Broadcast {
		go a.blastEmail(recipients, in.Subject, in.Body)
		writeJSON(w, http.StatusOK, map[string]any{"queued": len(recipients)})
		return
	}
	sent, failed := a.blastEmail(recipients, in.Subject, in.Body)
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent, "failed": failed})
}

func (a *App) blastEmail(recipients []string, subject, body string) (sent, failed int) {
	for _, to := range recipients {
		if err := a.sendMail(to, subject, body); err != nil {
			failed++
		} else {
			sent++
		}
	}
	return sent, failed
}

type adminNotifyReq struct {
	UID       string          `json:"uid"`
	Broadcast bool            `json:"broadcast"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Data      json.RawMessage `json:"data"`
}

// handleAdminNotify sends a custom push/in-app notification to one user or all.
func (a *App) handleAdminNotify(w http.ResponseWriter, r *http.Request) {
	var in adminNotifyReq
	if err := decodeJSON(r, &in); err != nil || in.Title == "" {
		writeJSON(w, http.StatusBadRequest, errBody("title required"))
		return
	}
	var data any
	if len(in.Data) > 0 {
		data = in.Data
	}

	if in.Broadcast {
		// Durable first (one INSERT for everyone), then deliver in parallel.
		raw := in.Data
		if len(raw) == 0 {
			raw = nil
		}
		targets, err := a.store.broadcastNotification(r.Context(), in.Title, in.Body, raw)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody("broadcast failed"))
			return
		}
		go func() {
			// ponytail: 8 concurrent FCM calls (~10 min for 50k); raise if broadcasts drag.
			sem, wg := make(chan struct{}, 8), sync.WaitGroup{}
			for _, t := range targets {
				sem <- struct{}{}
				wg.Add(1)
				go func() {
					defer func() { <-sem; wg.Done() }()
					a.deliver(t, in.Title, in.Body, raw)
				}()
			}
			wg.Wait()
			log.Printf("broadcast %q delivered to %d users", in.Title, len(targets))
		}()
		writeJSON(w, http.StatusOK, map[string]any{"queued": len(targets)})
		return
	}
	if in.UID == "" {
		writeJSON(w, http.StatusBadRequest, errBody("set broadcast or uid"))
		return
	}
	a.notifyUser(in.UID, in.Title, in.Body, data)
	writeJSON(w, http.StatusOK, map[string]any{"sent": 1})
}
