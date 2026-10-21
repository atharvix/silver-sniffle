package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// provider is one OpenID Connect sign-in (LinkedIn, Google): same flow, different endpoints.
type provider struct {
	name, col                        string // col: the users column holding this provider's subject
	authURL, tokenURL, userinfoURL   string
	clientID, clientSecret, redirect string
	extra                            url.Values // provider-specific authorize params
}

func linkedInProvider(c Config) *provider {
	return &provider{
		name: "linkedin", col: "linkedin_sub",
		authURL:     "https://www.linkedin.com/oauth/v2/authorization",
		tokenURL:    "https://www.linkedin.com/oauth/v2/accessToken",
		userinfoURL: "https://api.linkedin.com/v2/userinfo",
		clientID:    c.LinkedInClientID, clientSecret: c.LinkedInClientSecret, redirect: c.LinkedInRedirectURL,
		// Passkey + Sign in with Google/Apple on LinkedIn's own login page in
		// apps, so people who aren't signed in don't have to type a password.
		extra: url.Values{"enable_extended_login": {"true"}},
	}
}

// googleProvider is nil (sign-in off) until GOOGLE_CLIENT_ID/SECRET are set. Its
// redirect is fixed to this server: register <origin>/auth/google/callback.
func googleProvider(c Config, publicURL string) *provider {
	if c.GoogleClientID == "" || c.GoogleClientSecret == "" {
		return nil
	}
	return &provider{
		name: "google", col: "google_sub",
		authURL:     "https://accounts.google.com/o/oauth2/v2/auth",
		tokenURL:    "https://oauth2.googleapis.com/token",
		userinfoURL: "https://openidconnect.googleapis.com/v1/userinfo",
		clientID:    c.GoogleClientID, clientSecret: c.GoogleClientSecret, redirect: publicURL + "/auth/google/callback",
		extra: url.Values{"prompt": {"select_account"}}, // let people pick which Google account
	}
}

type ctxKey string

const ctxUID ctxKey = "uid"

// randToken returns a URL-safe random string with ~n bytes of entropy.
func randToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// startLogin redirects the browser to the provider's consent screen. A random
// state is round-tripped through a short-lived cookie to defend against CSRF.
func (a *App) startLogin(p *provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p == nil {
			a.failLogin(w, r, "provider_off")
			return
		}
		a.beginLogin(w, r, p)
	}
}

func (a *App) beginLogin(w http.ResponseWriter, r *http.Request, p *provider) {
	state, err := randToken(24)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	a.authCookie(w, "li_state", state, 600)
	// The app sends a PKCE challenge so only it can redeem the login (RFC 8252).
	// No challenge (older app builds): clear any stale one and use the legacy hand-off.
	if ch := r.URL.Query().Get("challenge"); len(ch) == 43 {
		a.authCookie(w, "li_pkce", ch, 600)
	} else {
		a.authCookie(w, "li_pkce", "", -1)
	}
	q := url.Values{
		"response_type": {"code"},
		"client_id":     {p.clientID},
		"redirect_uri":  {p.redirect},
		"state":         {state},
		"scope":         {"openid profile email"},
	}
	for k, v := range p.extra {
		q[k] = v
	}
	http.Redirect(w, r, p.authURL+"?"+q.Encode(), http.StatusFound)
}

// userinfo is the subset of the OIDC userinfo claims we use (same on both providers).
type userinfo struct {
	Sub           string `json:"sub"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

// callback finishes the OAuth dance: verify state, exchange the code, read the
// profile, create/refresh the account, and hand the SPA a session token.
func (a *App) callback(p *provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p == nil {
			a.failLogin(w, r, "provider_off")
			return
		}
		a.finishLogin(w, r, p)
	}
}

func (a *App) finishLogin(w http.ResponseWriter, r *http.Request, p *provider) {
	// Providers report user-declined consent as an error param, not a code.
	if e := r.URL.Query().Get("error"); e != "" {
		a.failLogin(w, r, e)
		return
	}
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie("li_state")
	if err != nil || state == "" || cookie.Value != state {
		a.failLogin(w, r, "bad_state")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		a.failLogin(w, r, "no_code")
		return
	}

	tok, err := a.exchangeCode(r.Context(), p, code)
	if err != nil {
		a.failLogin(w, r, "token_exchange")
		return
	}
	info, err := a.fetchUserinfo(r.Context(), p, tok)
	if err != nil || info.Sub == "" {
		a.failLogin(w, r, "userinfo")
		return
	}

	uid, created, err := a.store.upsertUser(r.Context(), p, info)
	if err != nil {
		a.failLogin(w, r, "db_user")
		return
	}
	// Seed a new profile with the provider's name. Photos are never taken from a
	// provider: everyone uploads their own.
	_ = a.store.seedProfile(r.Context(), uid, info.Name)
	a.hub.prof.invalidate(uid)
	// First-ever login: welcome the new user (email + in-app notification).
	if created {
		go a.welcomeNewUser(uid, info.Email, info.Name, info.EmailVerified)
	}

	// PKCE app: hand back a one-time code, redeemed with the verifier at /auth/exchange,
	// so a deep link intercepted by another app is useless.
	if ch, err := r.Cookie("li_pkce"); err == nil && ch.Value != "" {
		code, err := randToken(32)
		if err != nil {
			a.failLogin(w, r, "session")
			return
		}
		putHandoff(code, handoff{uid: uid, challenge: ch.Value, exp: time.Now().Add(2 * time.Minute)})
		http.Redirect(w, r, a.cfg.FrontendURL+"#code="+code, http.StatusFound)
		return
	}

	session, err := randToken(32)
	if err != nil || a.store.createSession(r.Context(), session, uid) != nil {
		a.failLogin(w, r, "session")
		return
	}

	// Hand the token back in the URL fragment. FRONTEND_URL is either a web
	// origin (https://host) or the app deep link (kinjo://auth); appending
	// "#token=..." works for both, and the token never reaches a server log.
	http.Redirect(w, r, a.cfg.FrontendURL+"#token="+url.QueryEscape(session), http.StatusFound)
}

// authCookie sets a short-lived cookie scoped to the OAuth round trip.
func (a *App) authCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   strings.HasPrefix(a.publicURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

type handoff struct {
	uid, challenge string
	exp            time.Time
}

// handoffs maps one-time codes from the kinjo://auth deep link to the login they
// finish. ponytail: in-memory, fine for one backend instance; move to Postgres to run several.
var handoffs = struct {
	sync.Mutex
	m map[string]handoff
}{m: map[string]handoff{}}

func putHandoff(code string, h handoff) {
	handoffs.Lock()
	defer handoffs.Unlock()
	for k, v := range handoffs.m {
		if time.Now().After(v.exp) {
			delete(handoffs.m, k)
		}
	}
	handoffs.m[code] = h
}

// redeem consumes a one-time code (single use, even on failure) and returns its
// uid if the verifier hashes to the challenge the app sent at the start.
func redeem(code, verifier string) (string, bool) {
	handoffs.Lock()
	h, ok := handoffs.m[code]
	delete(handoffs.m, code)
	handoffs.Unlock()
	sum := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(sum[:])
	if !ok || time.Now().After(h.exp) || subtle.ConstantTimeCompare([]byte(got), []byte(h.challenge)) != 1 {
		return "", false
	}
	return h.uid, true
}

// exchange turns a redeemed one-time code into a session token.
func (a *App) exchange(w http.ResponseWriter, r *http.Request) {
	var in struct{ Code, Verifier string }
	if decodeJSON(r, &in) != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	uid, ok := redeem(in.Code, in.Verifier)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errBody("invalid code"))
		return
	}
	session, err := randToken(32)
	if err != nil || a.store.createSession(r.Context(), session, uid) != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("session"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": session})
}

func (a *App) failLogin(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, a.cfg.FrontendURL+"#auth_error="+url.QueryEscape(reason), http.StatusFound)
}

func (a *App) exchangeCode(ctx context.Context, p *provider, code string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.redirect},
		"client_id":     {p.clientID},
		"client_secret": {p.clientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", &httpError{resp.StatusCode, string(body)}
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	return out.AccessToken, nil
}

func (a *App) fetchUserinfo(ctx context.Context, p *provider, accessToken string) (userinfo, error) {
	var info userinfo
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userinfoURL, nil)
	if err != nil {
		return info, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := a.http.Do(req)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return info, &httpError{resp.StatusCode, string(body)}
	}
	err = json.Unmarshal(body, &info)
	return info, err
}

// requireAuth wraps a handler, rejecting requests without a valid session token.
func (a *App) requireAuth(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, err := a.uidFromRequest(r)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errBody("unavailable"))
			return
		}
		if uid == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r, uid)
	}
}

// uidFromRequest resolves the caller's uid from the Bearer token, or "" if none.
// A database failure is an error, not "no session": a 401 makes the app delete
// its token, so treating a DB blip as 401 used to sign every active user out.
func (a *App) uidFromRequest(r *http.Request) (string, error) {
	token := bearer(r)
	if token == "" {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	return a.store.uidForToken(ctx, token)
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return ""
}

// cors applies the configured allow-list and answers preflight requests.
func (a *App) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && a.cfg.originAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, ngrok-skip-browser-warning, X-Admin-Token")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type httpError struct {
	code int
	body string
}

func (e *httpError) Error() string { return "http " + itoa(e.code) + ": " + e.body }
