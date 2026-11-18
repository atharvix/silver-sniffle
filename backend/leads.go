package main

import (
	"html"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Leads: the website's partnership form (and any later "contact us" form). Stored, emailed to
// ADMIN_EMAILS, listed and exported in Kinjo Control. The sender's IP is used only for the
// rate limit, never stored.
const leadsSchema = `
CREATE TABLE IF NOT EXISTS leads (
  id         bigserial PRIMARY KEY,
  name       text NOT NULL,
  email      text NOT NULL,
  phone      text NOT NULL DEFAULT '',
  message    text NOT NULL DEFAULT '',
  source     text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
)`

var leadLimit = struct {
	sync.Mutex
	m map[string][]time.Time
}{m: map[string][]time.Time{}}

// 5 a client per hour, 100 for everyone together.
func leadAllowed(ip string, now time.Time) bool {
	leadLimit.Lock()
	defer leadLimit.Unlock()
	keep := func(ts []time.Time) []time.Time {
		out := ts[:0]
		for _, t := range ts {
			if now.Sub(t) < time.Hour {
				out = append(out, t)
			}
		}
		return out
	}
	all := keep(leadLimit.m["*"])
	mine := keep(leadLimit.m[ip])
	if len(mine) >= 5 || len(all) >= 100 {
		leadLimit.m["*"], leadLimit.m[ip] = all, mine
		return false
	}
	leadLimit.m["*"], leadLimit.m[ip] = append(all, now), append(mine, now)
	return true
}

func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	h, _, _ := net.SplitHostPort(r.RemoteAddr)
	return h
}

// POST /api/leads {name, email, phone, message, source}: public.
func (a *App) handleLead(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, Email, Phone, Message, Source, Website string }
	if decodeJSON(r, &in) != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	if in.Website != "" { // honeypot: people never see this field
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	email, ok := normEmail(in.Email)
	name := clip(strings.TrimSpace(in.Name), 100)
	if !ok || name == "" {
		writeJSON(w, http.StatusBadRequest, errBody("name and a valid email are required"))
		return
	}
	if !leadAllowed(clientIP(r), time.Now()) {
		writeJSON(w, http.StatusTooManyRequests, errBody("too many requests, try again later"))
		return
	}
	phone, msg, src := clip(strings.TrimSpace(in.Phone), 40), clip(strings.TrimSpace(in.Message), 4000), clip(strings.TrimSpace(in.Source), 40)
	if _, err := a.store.pool.Exec(r.Context(), `INSERT INTO leads (name, email, phone, message, source) VALUES ($1, $2, $3, $4, $5)`,
		name, email, phone, msg, src); err != nil {
		log.Printf("lead: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	log.Printf("new lead (%s)", src)
	go func() {
		subject := "New Kinjo lead: " + name
		text := "Name: " + name + "\r\nEmail: " + email + "\r\nPhone: " + phone + "\r\nFrom: " + src + "\r\n\r\n" + msg + "\r\n"
		body := emailShell("New lead.", name+" sent the "+src+" form.",
			mailP("<b>"+html.EscapeString(name)+"</b> · "+html.EscapeString(email)+" · "+html.EscapeString(phone))+
				mailP(strings.ReplaceAll(html.EscapeString(msg), "\n", "<br>")))
		for _, to := range a.cfg.AdminEmails {
			if err := a.sendMailMIME(to, subject, text, body); err != nil {
				log.Printf("lead email: %v", err)
			}
		}
	}()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type Lead struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Message   string    `json:"message"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"createdAt"`
}

func (a *App) leads(r *http.Request) ([]Lead, error) {
	rows, err := a.store.pool.Query(r.Context(), `SELECT id, name, email, phone, message, source, created_at FROM leads ORDER BY created_at DESC LIMIT 2000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lead{}
	for rows.Next() {
		var l Lead
		if err := rows.Scan(&l.ID, &l.Name, &l.Email, &l.Phone, &l.Message, &l.Source, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GET /admin/leads (DELETE ?id= removes one).
func (a *App) handleAdminLeads(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		_, _ = a.store.pool.Exec(r.Context(), `DELETE FROM leads WHERE id = $1`, r.URL.Query().Get("id"))
	}
	ls, err := a.leads(r)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"leads": ls})
}
