package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
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
	store     *Store
	hub       *Hub
	publicURL string
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

func (a *App) card(uid string, p Profile, updated time.Time) Card {
	return Card{UID: uid, Name: p.Name, Role: p.Role, Looking: p.Looking,
		Img: fmt.Sprintf("%s/api/photo/%s?v=%d", a.publicURL, uid, updated.Unix())}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// authed resolves the bearer token. A database failure is 503, never 401: the app deletes its token on 401.
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
		slog.Error("session lookup", "err", err)
		fail(w, http.StatusServiceUnavailable, "try again")
		return "", false
	}
	return uid, true
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /api/session", a.newSession)
	mux.HandleFunc("PUT /api/profile", a.putProfile)
	mux.HandleFunc("PUT /api/visibility", a.putVisibility)
	mux.HandleFunc("POST /api/device", a.postDevice)
	mux.HandleFunc("DELETE /api/account", a.deleteAccount)
	mux.HandleFunc("GET /api/photo/{uid}", a.getPhoto)
	mux.HandleFunc("GET /ws", a.serveSocket)
	return mux
}

func (a *App) newSession(w http.ResponseWriter, r *http.Request) {
	uid, token, err := a.store.newAccount(r.Context())
	if err != nil {
		slog.Error("new account", "err", err)
		fail(w, http.StatusServiceUnavailable, "try again")
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
		Photo string `json:"photo"` // base64 JPEG
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
	updated, err := a.store.saveProfile(r.Context(), uid, p, photo)
	if err != nil {
		slog.Error("save profile", "err", err)
		fail(w, http.StatusServiceUnavailable, "try again")
		return
	}
	a.hub.setCard(a.card(uid, p, updated))
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
		slog.Error("set hidden", "err", err)
		fail(w, http.StatusServiceUnavailable, "try again")
		return
	}
	var card *Card
	if *body.Visible {
		p, updated, has, err := a.store.profile(r.Context(), uid)
		if err != nil {
			slog.Error("load profile", "err", err)
			fail(w, http.StatusServiceUnavailable, "try again")
			return
		}
		if has {
			c := a.card(uid, p, updated)
			card = &c
		}
	}
	a.hub.setHidden(uid, !*body.Visible, card)
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
		slog.Error("save device", "err", err)
		fail(w, http.StatusServiceUnavailable, "try again")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) deleteAccount(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.authed(w, r)
	if !ok {
		return
	}
	if err := a.store.deleteAccount(r.Context(), uid); err != nil {
		slog.Error("delete account", "err", err)
		fail(w, http.StatusServiceUnavailable, "try again")
		return
	}
	a.hub.remove(uid)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) getPhoto(w http.ResponseWriter, r *http.Request) {
	photo, err := a.store.photo(r.Context(), r.PathValue("uid"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // the URL changes with every new photo
	_, _ = w.Write(photo)
}

// serveSocket: the phone sends {"type":"fix",lat,lng,acc}; the server pushes {"type":"deck",cards}.
func (a *App) serveSocket(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.authed(w, r)
	if !ok {
		return
	}
	p, updated, hasCard, err := a.store.profile(r.Context(), uid)
	var hidden bool
	if err == nil {
		hidden, err = a.store.hidden(r.Context(), uid)
	}
	if err != nil {
		slog.Error("load profile", "err", err)
		fail(w, http.StatusServiceUnavailable, "try again")
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(4096)
	c := &client{uid: uid, send: make(chan []byte, 16)}
	var card *Card
	if hasCard {
		cd := a.card(uid, p, updated)
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
	a.hub.attach(c, card, hidden)
	defer a.hub.detach(c)
	defer conn.CloseNow()

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
			a.hub.report(uid, fix{m.Lat, m.Lng, m.Acc, time.Now(), m.Still})
		case "seen":
			if len(m.Seen) <= 64 {
				a.hub.sighted(uid, m.Seen, time.Now())
			}
		}
	}
}
