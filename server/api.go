package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	maxNameChars    = 50
	maxRoleWords    = 7
	maxLookingWords = 20
	maxPhotoBytes   = 3 << 20
	maxPhotoSide    = 4000
)

type Profile struct {
	Name    string `json:"name"`
	Role    string `json:"role"`
	Looking string `json:"looking"`
}

type App struct {
	store  *Store
	hub    *Hub
	photos *Photos
}

func (p Profile) validate() error {
	roleWords, lookingWords := len(strings.Fields(p.Role)), len(strings.Fields(p.Looking))
	switch {
	case p.Name == "" || utf8.RuneCountInString(p.Name) > maxNameChars:
		return errors.New("name must be 1-50 characters")
	case roleWords < 1 || roleWords > maxRoleWords || len(p.Role) > 120:
		return errors.New("what you do must be 1-7 words")
	case lookingWords < 1 || lookingWords > maxLookingWords || len(p.Looking) > 300:
		return errors.New("what you're looking for must be 1-20 words")
	}
	return nil
}

// cleanPhoto accepts only a real JPEG of sane size and re-encodes it, dropping any metadata (EXIF GPS).
func cleanPhoto(raw []byte) ([]byte, error) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width > maxPhotoSide || cfg.Height > maxPhotoSide {
		return nil, errors.New("photo must be a JPEG up to 4000 px")
	}
	img, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("photo can't be read")
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (a *App) card(uid string, p Profile, photoKey string) Card {
	return Card{UID: uid, Name: p.Name, Role: p.Role, Looking: p.Looking, Img: a.photos.url(photoKey)}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	metricRequests.WithLabelValues(strconv.Itoa(status)).Inc()
	writeJSON(w, status, map[string]string{"error": msg})
}

// unavailable logs a dependency failure and answers 503, never 401: the app deletes its token on 401.
func unavailable(w http.ResponseWriter, what string, err error) {
	slog.Error(what, "err", err)
	fail(w, http.StatusServiceUnavailable, "try again")
}

func (a *App) authed(w http.ResponseWriter, r *http.Request) (string, bool) {
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found || token == "" {
		fail(w, http.StatusUnauthorized, "sign in")
		return "", false
	}
	uid, err := a.store.userFor(r.Context(), token)
	switch {
	case errors.Is(err, errNoSession):
		fail(w, http.StatusUnauthorized, "sign in")
		return "", false
	case err != nil:
		unavailable(w, "session lookup", err)
		return "", false
	}
	return uid, true
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.Handle("GET /metrics", promhttp.Handler()) // nginx doesn't expose it; read on the server itself
	mux.HandleFunc("POST /api/session", a.newSession)
	mux.HandleFunc("PUT /api/profile", a.putProfile)
	mux.HandleFunc("PUT /api/visibility", a.putVisibility)
	mux.HandleFunc("POST /api/device", a.postDevice)
	mux.HandleFunc("POST /api/block", a.postBlock)
	mux.HandleFunc("POST /api/report", a.postReport)
	mux.HandleFunc("DELETE /api/account", a.deleteAccount)
	mux.HandleFunc("GET /ws", a.serveSocket)
	return mux
}

func (a *App) newSession(w http.ResponseWriter, r *http.Request) {
	uid, token, err := a.store.newAccount(r.Context())
	if err != nil {
		unavailable(w, "new account", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"uid": uid, "token": token})
}

func (a *App) putProfile(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.authed(w, r)
	if !ok {
		return
	}
	var body struct {
		Profile
		Photo  string `json:"photo"`  // base64 JPEG
		Agreed bool   `json:"agreed"` // "I'm 18 or older and accept the terms"
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes*4/3+4096)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "bad request")
		return
	}
	p := Profile{strings.TrimSpace(body.Name), strings.TrimSpace(body.Role), strings.TrimSpace(body.Looking)}
	if err := p.validate(); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if !body.Agreed {
		fail(w, http.StatusBadRequest, "confirm you're 18 or older and accept the terms")
		return
	}
	if err := a.store.agree(r.Context(), uid); err != nil {
		unavailable(w, "agree", err)
		return
	}
	raw, err := base64.StdEncoding.DecodeString(body.Photo)
	if err != nil {
		fail(w, http.StatusBadRequest, "photo is required")
		return
	}
	photo, err := cleanPhoto(raw)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	key, err := a.photos.put(r.Context(), uid, photo)
	if err != nil {
		unavailable(w, "store photo", err)
		return
	}
	_, oldKey, err := a.store.saveProfile(r.Context(), uid, p, key)
	if err != nil {
		_ = a.photos.delete(r.Context(), key) // nothing points at it
		unavailable(w, "save profile", err)
		return
	}
	if oldKey != "" && oldKey != key {
		if err := a.photos.delete(r.Context(), oldKey); err != nil {
			slog.Warn("delete old photo", "err", err) // harmless: an orphan, removed with the account
		}
	}
	if err := a.hub.setCard(r.Context(), a.card(uid, p, key)); err != nil {
		slog.Warn("publish card", "err", err) // saved; it reaches the hub on the next connect
	}
	w.WriteHeader(http.StatusNoContent)
}

// putVisibility is "Hide my profile": enforced here, so every phone and old build obeys it.
func (a *App) putVisibility(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.authed(w, r)
	if !ok {
		return
	}
	var body struct {
		Visible *bool `json:"visible"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body); err != nil || body.Visible == nil {
		fail(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := a.store.setHidden(r.Context(), uid, !*body.Visible); err != nil {
		unavailable(w, "set hidden", err)
		return
	}
	var card *Card
	if *body.Visible {
		p, key, has, err := a.store.profile(r.Context(), uid)
		if err != nil {
			unavailable(w, "load profile", err)
			return
		}
		if has {
			c := a.card(uid, p, key)
			card = &c
		}
	}
	if err := a.hub.setHidden(r.Context(), uid, !*body.Visible, card); err != nil {
		unavailable(w, "hub visibility", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// postDevice stores the phone's push token so the server can wake it when someone is near.
func (a *App) postDevice(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.authed(w, r)
	if !ok {
		return
	}
	var body struct{ Token, Platform string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil ||
		body.Token == "" || len(body.Token) > 4096 || (body.Platform != "android" && body.Platform != "ios") {
		fail(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := a.store.saveDevice(r.Context(), uid, body.Token, body.Platform); err != nil {
		unavailable(w, "save device", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var reportReasons = map[string]bool{"fake": true, "inappropriate": true, "harassment": true, "spam": true, "underage": true, "other": true}

// postBlock hides two people from each other for good.
func (a *App) postBlock(w http.ResponseWriter, r *http.Request) {
	uid, target, ok := a.targetOf(w, r, nil)
	if ok {
		a.blockPair(w, r, uid, target)
	}
}

// postReport records why someone was reported for moderation, and blocks them for the reporter.
func (a *App) postReport(w http.ResponseWriter, r *http.Request) {
	var reason string
	uid, target, ok := a.targetOf(w, r, &reason)
	if !ok {
		return
	}
	if !reportReasons[reason] {
		fail(w, http.StatusBadRequest, "pick a reason")
		return
	}
	if err := a.store.report(r.Context(), uid, target, reason); err != nil {
		unavailable(w, "report", err)
		return
	}
	a.blockPair(w, r, uid, target)
}

// targetOf reads {"uid": ..., "reason": ...}; the target must be another existing account.
func (a *App) targetOf(w http.ResponseWriter, r *http.Request, reason *string) (uid, target string, ok bool) {
	if uid, ok = a.authed(w, r); !ok {
		return "", "", false
	}
	var body struct{ UID, Reason string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body); err != nil || body.UID == "" || body.UID == uid {
		fail(w, http.StatusBadRequest, "bad request")
		return "", "", false
	}
	exists, err := a.store.exists(r.Context(), body.UID)
	if err != nil {
		unavailable(w, "find user", err)
		return "", "", false
	}
	if !exists {
		fail(w, http.StatusNotFound, "that person is gone")
		return "", "", false
	}
	if reason != nil {
		*reason = body.Reason
	}
	return uid, body.UID, true
}

func (a *App) blockPair(w http.ResponseWriter, r *http.Request, uid, target string) {
	if err := a.store.block(r.Context(), uid, target); err != nil {
		unavailable(w, "block", err)
		return
	}
	if err := a.hub.block(r.Context(), uid, target); err != nil {
		slog.Warn("hub block", "err", err) // saved; applied in full on the next connect
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) deleteAccount(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.authed(w, r)
	if !ok {
		return
	}
	if err := a.store.deleteAccount(r.Context(), uid); err != nil {
		unavailable(w, "delete account", err)
		return
	}
	if err := a.hub.remove(r.Context(), uid); err != nil {
		slog.Error("hub remove", "err", err) // the account is gone; live state expires within minutes anyway
	}
	if err := a.photos.deleteAll(r.Context(), uid); err != nil {
		slog.Error("delete photos", "err", err) // to clean up later: objects under p/<uid>/
	}
	w.WriteHeader(http.StatusNoContent)
}

// serveSocket: the phone sends {"type":"fix",lat,lng,acc,still} and {"type":"seen",seen:[{t,rssi}]};
// the server pushes {"type":"deck",cards}, {"type":"ble",token} and {"type":"vis",visible}.
func (a *App) serveSocket(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.authed(w, r)
	if !ok {
		return
	}
	p, key, hasCard, err := a.store.profile(r.Context(), uid)
	var hidden bool
	if err == nil {
		hidden, err = a.store.hidden(r.Context(), uid)
	}
	var blocked []string
	if err == nil {
		blocked, err = a.store.blockedWith(r.Context(), uid)
	}
	if err != nil {
		unavailable(w, "load profile", err)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4096)
	metricSockets.Inc()
	opened := time.Now()
	slog.Info("socket open", "uid", uid)
	defer func() {
		metricSockets.Dec()
		slog.Info("socket closed", "uid", uid, "secs", int(time.Since(opened).Seconds()))
	}()

	c := &client{uid: uid, send: make(chan []byte, 16)}
	var card *Card
	if hasCard {
		cd := a.card(uid, p, key)
		card = &cd
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		for {
			select {
			case msg := <-c.send:
				wctx, done := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Write(wctx, websocket.MessageText, msg)
				done()
				if err != nil {
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	if err := a.hub.attach(ctx, c, card, hidden, blocked); err != nil {
		slog.Error("attach", "err", err)
		return
	}
	defer func() {
		// The request context is already cancelled here: give the cleanup its own short one.
		dctx, done := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		defer done()
		a.hub.detach(dctx, c)
	}()

	for {
		// Moving phones report every few seconds and still ones every minute: 3 minutes of silence is a dead socket.
		rctx, done := context.WithTimeout(ctx, 3*time.Minute)
		_, data, err := conn.Read(rctx)
		done()
		if err != nil {
			return
		}
		var m struct {
			Type          string
			Lat, Lng, Acc float64
			Still         bool
			Seen          []Sighting
		}
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.Type {
		case "fix":
			accepted, err := a.hub.report(ctx, uid, fix{m.Lat, m.Lng, m.Acc, time.Now(), m.Still})
			if err != nil {
				slog.Warn("report", "err", err)
			} else if accepted {
				metricFixes.Inc()
			}
		case "seen":
			if len(m.Seen) <= 64 {
				metricSightings.Inc()
				if err := a.hub.sighted(ctx, uid, m.Seen, time.Now()); err != nil {
					slog.Warn("sighted", "err", err)
				}
			}
		}
	}
}
