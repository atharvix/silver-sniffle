package main

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"strings"
	"time"
)

// Kinjo Control sign-in: an emailed 6-digit code, only for the addresses in ADMIN_EMAILS.
// The code is its own (keyed "admin:<email>"), so an app sign-in code never opens the
// panel. A good code gives an admin session: a random token, stored as sha256 only,
// valid adminSessionTTL. ADMIN_TOKEN still works as the X-Admin-Token header when set
// (scripts, break-glass); leave it unset to allow email sign-in only.
const adminSessionTTL = 12 * time.Hour

const adminSchema = `
CREATE TABLE IF NOT EXISTS admin_sessions (
  token_hash text PRIMARY KEY,
  email      text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL
)`

func (a *App) adminOn() bool { return a.cfg.AdminToken != "" || len(a.cfg.AdminEmails) > 0 }

func (a *App) isAdminEmail(email string) bool {
	ok := 0
	for _, e := range a.cfg.AdminEmails {
		ok |= subtle.ConstantTimeCompare([]byte(e), []byte(email))
	}
	return ok == 1
}

// adminFromRequest: "" unless X-Admin-Token is ADMIN_TOKEN or a live admin session.
func (a *App) adminFromRequest(r *http.Request) string {
	got := r.Header.Get("X-Admin-Token")
	if got == "" {
		return ""
	}
	if a.cfg.AdminToken != "" && subtle.ConstantTimeCompare([]byte(got), []byte(a.cfg.AdminToken)) == 1 {
		return "token"
	}
	var email string
	err := a.store.pool.QueryRow(r.Context(), `SELECT email FROM admin_sessions WHERE token_hash = $1 AND expires_at > now()`, hashToken(got)).Scan(&email)
	if err != nil || !a.isAdminEmail(email) { // removed from ADMIN_EMAILS = signed out
		return ""
	}
	return email
}

// POST /admin/auth/start {email}: always 200 for a well-formed address, so it doesn't say
// who is an admin. Only listed addresses get a code.
func (a *App) handleAdminAuthStart(w http.ResponseWriter, r *http.Request) {
	if len(a.cfg.AdminEmails) == 0 {
		writeJSON(w, http.StatusForbidden, errBody("email sign-in is off: set ADMIN_EMAILS"))
		return
	}
	var in struct{ Email string }
	email, ok := "", false
	if decodeJSON(r, &in) == nil {
		email, ok = normEmail(in.Email)
	}
	if !ok {
		writeJSON(w, http.StatusBadRequest, errBody("Enter a valid email."))
		return
	}
	if !a.isAdminEmail(email) {
		log.Printf("admin sign-in asked for an unlisted address")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	code, _, ok := reserveCode("admin:"+email, time.Now())
	if !ok {
		writeJSON(w, http.StatusTooManyRequests, errBody("A code was sent a moment ago. Check your inbox."))
		return
	}
	go func() {
		subject := "Kinjo Control sign-in code: " + code
		text := "Your Kinjo Control sign-in code is: " + code + "\r\n\r\nIt expires in 10 minutes. If you didn't ask for it, someone is trying to open the admin panel with your address: nobody can without this code.\r\n"
		html := emailShell("Kinjo Control.", "Your admin sign-in code is "+code+".",
			mailP(`Enter this code to open Kinjo Control:`)+mailCode(code)+
				mailFine(`Didn’t ask for it? Someone tried to open the admin panel with your address. Nobody can without this code.`))
		if err := a.sendMailMIME(email, subject, text, html); err != nil {
			log.Printf("admin code email: %v", err)
		}
	}()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /admin/auth/verify {email, code} → {token, email, expiresAt}
func (a *App) handleAdminAuthVerify(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Code string }
	if decodeJSON(r, &in) != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	email, ok := normEmail(in.Email)
	code := strings.TrimSpace(in.Code)
	if !ok || len(code) != 6 || !a.isAdminEmail(email) {
		writeJSON(w, http.StatusUnauthorized, errBody("That code isn't right."))
		return
	}
	switch checkCode("admin:"+email, code, time.Now()) {
	case "":
	case "too_many", "expired":
		writeJSON(w, http.StatusUnauthorized, errBody("That code has expired. Ask for a new one."))
		return
	default:
		writeJSON(w, http.StatusUnauthorized, errBody("That code isn't right."))
		return
	}
	tok, err := randToken(32)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("try again"))
		return
	}
	exp := time.Now().Add(adminSessionTTL)
	if _, err := a.store.pool.Exec(r.Context(), `INSERT INTO admin_sessions (token_hash, email, expires_at) VALUES ($1, $2, $3)`, hashToken(tok), email, exp); err != nil {
		log.Printf("admin session: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, errBody("try again"))
		return
	}
	_, _ = a.store.pool.Exec(r.Context(), `DELETE FROM admin_sessions WHERE expires_at < now()`)
	log.Printf("admin signed in")
	writeJSON(w, http.StatusOK, map[string]any{"token": tok, "email": email, "expiresAt": exp})
}

// POST /admin/auth/logout: ends this admin session.
func (a *App) handleAdminAuthLogout(w http.ResponseWriter, r *http.Request) {
	if t := r.Header.Get("X-Admin-Token"); t != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		_, _ = a.store.pool.Exec(ctx, `DELETE FROM admin_sessions WHERE token_hash = $1`, hashToken(t))
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /admin/auth/me: who is signed in (the panel's check on load).
func (a *App) handleAdminAuthMe(w http.ResponseWriter, r *http.Request) {
	who := a.adminFromRequest(r)
	if who == "token" {
		who = ""
	}
	writeJSON(w, http.StatusOK, map[string]string{"email": who})
}
