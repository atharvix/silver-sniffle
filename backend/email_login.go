package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"
)

// "Continue with email": we email a 6-digit code and the person types it into the
// app. That proves they read the inbox, so the account is matched on a verified
// address, exactly like a provider that vouches for its email.
const (
	codeTTL      = 10 * time.Minute
	codeTries    = 5           // wrong guesses before a code is burned
	codeGap      = time.Minute // between sends to one address (repeat taps)
	codesPerHour = 5
)

type emailCode struct {
	hash  [32]byte
	exp   time.Time
	tries int
	sent  []time.Time // sends in the last hour
}

// ponytail: in memory, fine for one backend instance; move to Postgres to run several.
var emailCodes = struct {
	sync.Mutex
	m map[string]*emailCode
}{m: map[string]*emailCode{}}

func codeHash(email, code string) [32]byte { return sha256.Sum256([]byte(email + ":" + code)) }

// normEmail accepts a bare address only ("a@b.c", not "Name <a@b.c>"), lowercased.
func normEmail(s string) (string, bool) {
	s = strings.TrimSpace(s)
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || len(s) > 254 {
		return "", false
	}
	return strings.ToLower(s), true
}

// reserveCode makes a fresh code for email, or returns false when one was sent
// less than codeGap ago or codesPerHour have gone out this hour. It returns
// (code, isResend, ok).
func reserveCode(email string, now time.Time) (string, bool, bool) {
	emailCodes.Lock()
	defer emailCodes.Unlock()
	for k, c := range emailCodes.m { // drop finished entries so the map stays small
		if now.After(c.exp) && (len(c.sent) == 0 || now.Sub(c.sent[len(c.sent)-1]) > time.Hour) {
			delete(emailCodes.m, k)
		}
	}
	c := emailCodes.m[email]
	if c == nil {
		c = &emailCode{}
		emailCodes.m[email] = c
	}
	recent := c.sent[:0]
	for _, t := range c.sent {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	c.sent = recent
	isResend := len(recent) > 0
	if len(recent) >= codesPerHour || (len(recent) > 0 && now.Sub(recent[len(recent)-1]) < codeGap) {
		return "", false, false
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", false, false
	}
	code := fmt.Sprintf("%06d", n.Int64())
	c.hash, c.exp, c.tries, c.sent = codeHash(email, code), now.Add(codeTTL), 0, append(recent, now)
	return code, isResend, true
}

// checkCode consumes the code: "" once for the right code in time, otherwise
// "expired" (or never sent), "wrong", or "too_many" (code burned).
func checkCode(email, code string, now time.Time) string {
	emailCodes.Lock()
	defer emailCodes.Unlock()
	c := emailCodes.m[email]
	if c == nil || c.exp.IsZero() || now.After(c.exp) {
		return "expired"
	}
	want := c.hash
	if got := codeHash(email, code); subtle.ConstantTimeCompare(got[:], want[:]) == 1 {
		c.exp = time.Time{} // single use; keep sent for the rate limit
		return ""
	}
	if c.tries++; c.tries >= codeTries {
		c.exp = time.Time{}
		return "too_many"
	}
	return "wrong"
}

// emailStart sends a sign-in code. It answers the same whether or not an account
// exists, so it can't be used to find out who uses Kinjo.
func (a *App) emailStart(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email string }
	if decodeJSON(r, &in) != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	email, ok := normEmail(in.Email)
	if !ok {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_email"))
		return
	}
	code, isResend, ok := reserveCode(email, time.Now())
	if !ok {
		writeJSON(w, http.StatusTooManyRequests, errBody("too_soon"))
		return
	}

	// Check if this email already belongs to an existing user
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	userExists, err := a.store.hasUserWithEmail(ctx, email)
	cancel()
	if err != nil {
		log.Printf("hasUserWithEmail %s: %v", email, err)
	}

	go func() { // the mail server can take seconds; answer now
		var sendErr error
		if !isResend && !userExists {
			// First-time email signup: welcome + OTP in same email
			sendErr = a.sendWelcomeWithOTPEmail(email, code)
		} else {
			// Resend or returning user: clean OTP only, no welcome
			sendErr = a.sendOTPEmail(email, code)
		}
		if sendErr != nil {
			log.Printf("sign-in code email (%s, resend=%v, exists=%v): %v", email, isResend, userExists, sendErr)
		}
	}()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// emailVerify trades a correct code for a session, creating the account on first use.
func (a *App) emailVerify(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Code string }
	if decodeJSON(r, &in) != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	email, ok := normEmail(in.Email)
	if !ok {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_email"))
		return
	}
	if why := checkCode(email, strings.TrimSpace(in.Code), time.Now()); why != "" {
		writeJSON(w, http.StatusBadRequest, errBody(why))
		return
	}
	uid, created, err := a.store.emailUser(r.Context(), email)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	session, err := randToken(32)
	if err != nil || a.store.createSession(r.Context(), session, uid) != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
		return
	}
	if created {
		// In-app welcome notification (user already received the welcome email with their OTP)
		a.notifyUser(uid, "Welcome to Kinjo 👋",
			"You're all set. Finish your card so people within 30 m can find you.", nil)
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": session})
}
