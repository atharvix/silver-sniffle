package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Field limits mirror the old Firestore security rules exactly.
const (
	maxName  = 40
	maxRole  = 60
	maxLook  = 450
	maxPhoto = 750000
)

// meResponse is what the SPA reads on load to decide where to route.
type meResponse struct {
	UID           string   `json:"uid"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"emailVerified"`
	Profile       *Profile `json:"profile"` // null until the user completes their card
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request, uid string) {
	u, err := a.store.getUser(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("lookup failed"))
		return
	}
	p, ok, err := a.store.getProfile(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("lookup failed"))
		return
	}
	resp := meResponse{UID: u.ID, Email: u.Email, EmailVerified: u.Verified}
	if ok {
		resp.Profile = &p
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleSaveProfile validates and upserts the user's card.
func (a *App) handleSaveProfile(w http.ResponseWriter, r *http.Request, uid string) {
	var in Profile
	if err := decodeJSON(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Role = strings.TrimSpace(in.Role)
	in.Look = strings.TrimSpace(in.Look)
	if msg := validateProfile(in); msg != "" {
		writeJSON(w, http.StatusBadRequest, errBody(msg))
		return
	}
	if err := a.store.saveProfile(r.Context(), uid, in); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("save failed"))
		return
	}
	a.hub.prof.invalidate(uid)
	// A changed card should reach people already looking at it.
	a.hub.notify(nil, currentCells(a.hub, uid))
	p, _, _ := a.store.getProfile(r.Context(), uid)
	writeJSON(w, http.StatusOK, p)
}

// photoTypes are the only image formats stored or served: no SVG, which can
// carry script and would run on our origin if someone opened the photo URL.
var photoTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

// Lengths count characters, not bytes ("40" was ~13 Devanagari characters).
func validateProfile(p Profile) string {
	n := utf8.RuneCountInString
	switch {
	case p.Name == "" || n(p.Name) > maxName:
		return "name is required (max 40 chars)"
	case p.Role == "" || n(p.Role) > maxRole:
		return "role is required (max 60 chars)"
	case p.Look == "" || n(p.Look) > maxLook:
		return "look is required (max 450 chars)"
	case !validPhoto(p.Photo) || len(p.Photo) >= maxPhoto:
		return "photo must be an uploaded JPEG, PNG or WebP (under 750 KB)"
	default:
		return ""
	}
}

// validPhoto accepts inline images, plus legacy LinkedIn CDN links (refreshed to
// inline on next sign-in). Any other URL would make every viewer's phone fetch
// it, leaking their IP and when they were near that person.
func validPhoto(s string) bool {
	if strings.HasPrefix(s, "https://media.licdn.com/") {
		return true
	}
	meta, _, ok := strings.Cut(strings.TrimPrefix(s, "data:"), ";base64,")
	return ok && strings.HasPrefix(s, "data:") && photoTypes[meta]
}

// handlePhoto serves a profile photo by reference so realtime pushes stay tiny.
// The ?v= the hub adds (profile updated time) makes each version immutable, so
// phones download it once. Public on purpose: <img> can't send a Bearer token,
// and the unguessable uid is only ever shown to people already near that user.
func (a *App) handlePhoto(w http.ResponseWriter, r *http.Request) {
	p, ok := a.hub.prof.get(r.PathValue("uid"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(p.Photo, "https://media.licdn.com/") {
		http.Redirect(w, r, p.Photo, http.StatusFound)
		return
	}
	meta, data, found := strings.Cut(strings.TrimPrefix(p.Photo, "data:"), ";base64,")
	b, err := base64.StdEncoding.DecodeString(data)
	if !found || err != nil || !photoTypes[meta] {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", meta)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b)
}

// handleLogout ends the session and takes the user off the live map.
func (a *App) handleLogout(w http.ResponseWriter, r *http.Request, uid string) {
	_ = a.store.deleteSession(r.Context(), bearer(r))
	_ = a.store.deletePresence(r.Context(), uid)
	touched := a.hub.dropPresence(uid)
	a.hub.notify(nil, touched)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleDeleteAccount removes the user and all their data.
func (a *App) handleDeleteAccount(w http.ResponseWriter, r *http.Request, uid string) {
	touched := a.hub.dropPresence(uid)
	a.hub.prof.invalidate(uid)
	if err := a.store.deleteAccount(r.Context(), uid); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("delete failed"))
		return
	}
	a.hub.notify(nil, touched)
	if c := a.hub.client(uid); c != nil {
		c.conn.Close() // the account is gone; don't leave its live socket running
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handlePresence is the native-HTTP twin of the WebSocket "pos" message: Android
// throttles WebView sockets after ~5 min in the background, native HTTP isn't.
func (a *App) handlePresence(w http.ResponseWriter, r *http.Request, uid string) {
	var in inbound
	if decodeJSON(r, &in) != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	a.applyPos(uid, in.Lat, in.Lng, in.Acc)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

/* ---------------- push devices ---------------- */

// handleRegisterDevice stores the app's push token (e.g. FCM) for this user.
func (a *App) handleRegisterDevice(w http.ResponseWriter, r *http.Request, uid string) {
	var in struct {
		PushToken string `json:"pushToken"`
		Platform  string `json:"platform"`
	}
	if err := decodeJSON(r, &in); err != nil || strings.TrimSpace(in.PushToken) == "" {
		writeJSON(w, http.StatusBadRequest, errBody("pushToken required"))
		return
	}
	if err := a.store.upsertDevice(r.Context(), uid, in.PushToken, in.Platform); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("save failed"))
		return
	}
	// Deliver any notifications that were created before this device existed
	// (e.g. the welcome notification) — exactly once each, then mark them sent
	// so they never re-push on subsequent app opens.
	if a.fcm != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			list, err := a.store.undeliveredNotifications(ctx, uid, 10)
			if err != nil {
				return
			}
			for _, n := range list {
				sent, _ := a.fcm.send(ctx, []string{in.PushToken}, n.Title, n.Body,
					map[string]string{"notifId": strconv.FormatInt(n.ID, 10)})
				if sent > 0 {
					_ = a.store.markNotificationSent(ctx, n.ID)
				}
			}
		}()
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleDeleteDevice unregisters a push token (e.g. on logout).
func (a *App) handleDeleteDevice(w http.ResponseWriter, r *http.Request, uid string) {
	var in struct {
		PushToken string `json:"pushToken"`
	}
	if err := decodeJSON(r, &in); err != nil || in.PushToken == "" {
		writeJSON(w, http.StatusBadRequest, errBody("pushToken required"))
		return
	}
	_ = a.store.deleteDevice(r.Context(), uid, in.PushToken)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

/* ---------------- notifications feed ---------------- */

// handleNotifications returns the user's notification feed, newest first.
func (a *App) handleNotifications(w http.ResponseWriter, r *http.Request, uid string) {
	list, err := a.store.listNotifications(r.Context(), uid, 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("lookup failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": list})
}

// handleReadNotifications marks notifications read (all, or one via ?id=).
func (a *App) handleReadNotifications(w http.ResponseWriter, r *http.Request, uid string) {
	var id int64
	if v := r.URL.Query().Get("id"); v != "" {
		id, _ = strconv.ParseInt(v, 10, 64)
	}
	if err := a.store.markNotificationsRead(r.Context(), uid, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("update failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

/* ---------------- email verification ---------------- */

// handleSendVerification issues a token and emails (or logs) a verify link.
func (a *App) handleSendVerification(w http.ResponseWriter, r *http.Request, uid string) {
	u, err := a.store.getUser(r.Context(), uid)
	if err != nil || u.Email == "" {
		writeJSON(w, http.StatusBadRequest, errBody("no email on file"))
		return
	}
	token, err := randToken(24)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("server error"))
		return
	}
	status, err := a.store.reserveVerification(r.Context(), token, uid, u.Email, 24*time.Hour)
	switch {
	case err != nil:
		writeJSON(w, http.StatusServiceUnavailable, errBody("try again shortly"))
		return
	case status == "limited":
		writeJSON(w, http.StatusTooManyRequests, errBody("too many verification emails; try again later"))
		return
	case status == "recent": // a repeat tap: the email from a moment ago is on its way
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	// Answer now; SMTP can take seconds. ponytail: a failed send is only logged
	// (the user can resend after a minute); a queue with retries if this matters.
	go func() {
		if err := a.sendVerificationLink(u.Email, a.verifyURL(token)); err != nil {
			log.Printf("verification email for %s: %v", uid, err)
		}
	}()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// verificationLink issues a one-time, 24 h token and returns the link to email.
func (a *App) verificationLink(ctx context.Context, uid, email string) (string, error) {
	token, err := randToken(24)
	if err != nil {
		return "", err
	}
	if err := a.store.createEmailVerification(ctx, token, uid, email, 24*time.Hour); err != nil {
		return "", err
	}
	return a.verifyURL(token), nil
}

func (a *App) verifyURL(token string) string {
	return a.publicURL + "/auth/verify?token=" + url.QueryEscape(token)
}

// handleVerifyEmail is the link target. It answers with a small page (works on a
// desktop too, where kinjo:// can't open) that offers to jump back into the app.
func (a *App) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	uid, err := a.store.consumeEmailVerification(r.Context(), r.URL.Query().Get("token"))
	title, msg, frag, code := "Email verified", "Your email is confirmed. You can head back to Kinjo.", "verified=1", http.StatusOK
	if err != nil || uid == "" {
		title, msg, frag, code = "Link expired", "This link has expired. Open Kinjo, go to Settings and send a new one.", "verify_error=1", http.StatusGone
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")         // the URL carries a token
	h.Set("Referrer-Policy", "no-referrer")    // ...so never leak it onward
	h.Set("X-Content-Type-Options", "nosniff") //
	w.WriteHeader(code)
	fmt.Fprintf(w, verifyPage, title, title, msg, html.EscapeString(a.cfg.FrontendURL+"#"+frag))
}

const verifyPage = `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s · Kinjo</title>
<body style="margin:0;min-height:100vh;display:grid;place-items:center;background:#0a0a0a;color:#f1f1f1;font:16px/1.5 system-ui,sans-serif;text-align:center;padding:24px;box-sizing:border-box">
<main><p style="letter-spacing:.2em;font-weight:700;font-size:13px;margin:0 0 28px">KINJO</p>
<h1 style="font-weight:500;font-size:28px;margin:0 0 10px">%s</h1>
<p style="color:#a8a8a8;margin:0 auto 28px;max-width:32ch">%s</p>
<a href="%s" style="display:inline-block;background:#f1f1f1;color:#0a0a0a;padding:14px 26px;border-radius:999px;text-decoration:none;font-weight:600">Open Kinjo</a></main></body></html>`

// currentCells returns the cells a user currently occupies, for notify().
func currentCells(h *Hub, uid string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if p := h.presence[uid]; p != nil {
		return []string{p.Cell}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPhoto+8192))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

func errBody(msg string) map[string]string { return map[string]string{"error": msg} }
