package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
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
	UID     string   `json:"uid"`
	Email   string   `json:"email"`
	Profile *Profile `json:"profile"` // null until the user completes their card
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
	resp := meResponse{UID: u.ID, Email: u.Email}
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

func validateProfile(p Profile) string {
	switch {
	case p.Name == "" || len(p.Name) > maxName:
		return "name is required (max 40 chars)"
	case p.Role == "" || len(p.Role) > maxRole:
		return "role is required (max 60 chars)"
	case p.Look == "" || len(p.Look) > maxLook:
		return "look is required (max 450 chars)"
	case p.Photo == "" || len(p.Photo) >= maxPhoto:
		return "photo is required (under 750 KB)"
	default:
		return ""
	}
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
				a.fcm.send(ctx, []string{in.PushToken}, n.Title, n.Body,
					map[string]string{"notifId": strconv.FormatInt(n.ID, 10)})
				_ = a.store.markNotificationSent(ctx, n.ID)
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
	_ = a.store.deleteDevice(r.Context(), in.PushToken)
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
	if err := a.store.createEmailVerification(r.Context(), token, uid, u.Email, 24*time.Hour); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("server error"))
		return
	}
	link := publicBase(r) + "/auth/verify?token=" + url.QueryEscape(token)
	if err := a.sendVerificationLink(u.Email, link); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("couldn't send email"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleVerifyEmail is the link target; it consumes the token and bounces back
// to the app (success or error) via the configured FRONTEND_URL.
func (a *App) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	uid, err := a.store.consumeEmailVerification(r.Context(), token)
	if err != nil || uid == "" {
		http.Redirect(w, r, a.cfg.FrontendURL+"#verify_error=1", http.StatusFound)
		return
	}
	http.Redirect(w, r, a.cfg.FrontendURL+"#verified=1", http.StatusFound)
}

// publicBase reconstructs this server's externally-visible base URL (behind a
// TLS-terminating proxy it reads X-Forwarded-Proto).
func publicBase(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	return proto + "://" + r.Host
}

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
