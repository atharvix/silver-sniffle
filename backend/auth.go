package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// LinkedIn "Sign In with LinkedIn using OpenID Connect" endpoints.
const (
	liAuthorizeURL = "https://www.linkedin.com/oauth/v2/authorization"
	liTokenURL     = "https://www.linkedin.com/oauth/v2/accessToken"
	liUserinfoURL  = "https://api.linkedin.com/v2/userinfo"
	liScopes       = "openid profile email"
)

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

// startLogin redirects the browser to LinkedIn's consent screen. A random state
// is round-tripped through a short-lived cookie to defend against CSRF.
func (a *App) startLogin(w http.ResponseWriter, r *http.Request) {
	state, err := randToken(24)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "li_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   strings.HasPrefix(a.cfg.LinkedInRedirectURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
	q := url.Values{
		"response_type": {"code"},
		"client_id":     {a.cfg.LinkedInClientID},
		"redirect_uri":  {a.cfg.LinkedInRedirectURL},
		"state":         {state},
		"scope":         {liScopes},
	}
	http.Redirect(w, r, liAuthorizeURL+"?"+q.Encode(), http.StatusFound)
}

// userinfo is the subset of LinkedIn's OIDC userinfo claims we use.
type userinfo struct {
	Sub     string `json:"sub"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Picture string `json:"picture"`
}

// callback finishes the OAuth dance: verify state, exchange the code, read the
// profile, create/refresh the account, and hand the SPA a session token.
func (a *App) callback(w http.ResponseWriter, r *http.Request) {
	// LinkedIn reports user-declined consent as an error param, not a code.
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

	tok, err := a.exchangeCode(r.Context(), code)
	if err != nil {
		a.failLogin(w, r, "token_exchange")
		return
	}
	info, err := a.fetchUserinfo(r.Context(), tok)
	if err != nil || info.Sub == "" {
		a.failLogin(w, r, "userinfo")
		return
	}

	uid, created, err := a.store.upsertUserFromLinkedIn(r.Context(), info.Sub, info.Email, info.Name)
	if err != nil {
		a.failLogin(w, r, "db_user")
		return
	}
	// Seed name + photo on first login only; never clobber an edited profile.
	_ = a.store.seedProfile(r.Context(), uid, info.Name, info.Picture)
	// First-ever login: welcome the new user (email + in-app notification).
	if created {
		go a.welcomeNewUser(uid, info.Email, info.Name)
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

func (a *App) failLogin(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, a.cfg.FrontendURL+"#auth_error="+url.QueryEscape(reason), http.StatusFound)
}

func (a *App) exchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {a.cfg.LinkedInRedirectURL},
		"client_id":     {a.cfg.LinkedInClientID},
		"client_secret": {a.cfg.LinkedInClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, liTokenURL,
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

func (a *App) fetchUserinfo(ctx context.Context, accessToken string) (userinfo, error) {
	var info userinfo
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, liUserinfoURL, nil)
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
		uid := a.uidFromRequest(r)
		if uid == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r, uid)
	}
}

// uidFromRequest resolves the caller's uid from the Bearer token, or "" if none.
func (a *App) uidFromRequest(r *http.Request) string {
	token := bearer(r)
	if token == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	uid, err := a.store.uidForToken(ctx, token)
	if err != nil {
		return ""
	}
	return uid
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
