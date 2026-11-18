package main

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Sign in with Apple, iPhone app only: the app shows Apple's own sheet and posts the
// signed identity token here. We check Apple's signature (RS256, keys from Apple),
// issuer, audience (our bundle id), expiry and the nonce the app made for this sign-in,
// so a token can't be replayed from elsewhere. Apple's name isn't used: people type their own.

const appleIssuer = "https://appleid.apple.com"

var appleKeysURL = "https://appleid.apple.com/auth/keys"

func appleProvider() *provider { return &provider{name: "apple", col: "apple_sub"} }

func appleAudience() string {
	if b := os.Getenv("APPLE_BUNDLE_ID"); b != "" {
		return b
	}
	return "com.kinjo.app"
}

// appleKeys caches Apple's signing keys by kid; an unknown kid refetches, at most once a minute.
var appleKeys struct {
	sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

func appleKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	appleKeys.Lock()
	defer appleKeys.Unlock()
	if k := appleKeys.keys[kid]; k != nil && time.Since(appleKeys.fetched) < 24*time.Hour {
		return k, nil
	}
	if time.Since(appleKeys.fetched) < time.Minute && appleKeys.keys != nil {
		if k := appleKeys.keys[kid]; k != nil {
			return k, nil
		}
		return nil, errors.New("apple: unknown key")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, appleKeysURL, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var set struct {
		Keys []struct{ Kid, Kty, N, E string } `json:"keys"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&set) != nil {
		return nil, errors.New("apple: keys unavailable")
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if k.Kty != "RSA" || err1 != nil || err2 != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	appleKeys.keys, appleKeys.fetched = keys, time.Now()
	if k := keys[kid]; k != nil {
		return k, nil
	}
	return nil, errors.New("apple: unknown key")
}

// verifyAppleToken returns the token's claims if Apple signed it for us, for this nonce.
func verifyAppleToken(ctx context.Context, token, nonce string, now time.Time) (userinfo, error) {
	var none userinfo
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return none, errors.New("apple: malformed token")
	}
	var head struct{ Alg, Kid string }
	if b, err := base64.RawURLEncoding.DecodeString(parts[0]); err != nil || json.Unmarshal(b, &head) != nil || head.Alg != "RS256" {
		return none, errors.New("apple: bad header")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return none, errors.New("apple: bad signature")
	}
	key, err := appleKey(ctx, head.Kid)
	if err != nil {
		return none, err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig) != nil {
		return none, errors.New("apple: signature mismatch")
	}
	var c struct {
		Iss, Sub, Email, Nonce string
		Aud                    any
		Exp                    int64
		EmailVerified          any `json:"email_verified"` // Apple sends true or "true"
	}
	if b, err := base64.RawURLEncoding.DecodeString(parts[1]); err != nil || json.Unmarshal(b, &c) != nil {
		return none, errors.New("apple: bad claims")
	}
	want := sha256.Sum256([]byte(nonce))
	switch {
	case c.Iss != appleIssuer:
		return none, errors.New("apple: wrong issuer")
	case !audHas(c.Aud, appleAudience()):
		return none, errors.New("apple: wrong audience")
	case now.Unix() >= c.Exp:
		return none, errors.New("apple: expired")
	case nonce == "" || c.Nonce != hex.EncodeToString(want[:]):
		return none, errors.New("apple: wrong nonce")
	case c.Sub == "":
		return none, errors.New("apple: no subject")
	}
	return userinfo{Sub: c.Sub, Email: c.Email, EmailVerified: c.EmailVerified == true || c.EmailVerified == "true"}, nil
}

func audHas(aud any, want string) bool {
	switch v := aud.(type) {
	case string:
		return v == want
	case []any:
		for _, s := range v {
			if s == want {
				return true
			}
		}
	}
	return false
}

// appleLogin: POST /auth/apple {idToken, nonce, code} → {token}. code: Apple's one-time
// authorization code, swapped in the background for the refresh token deletion revokes.
func (a *App) appleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct{ IDToken, Nonce, Code string }
	if decodeJSON(r, &in) != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	info, err := verifyAppleToken(r.Context(), in.IDToken, in.Nonce, time.Now())
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errBody("invalid token"))
		return
	}
	uid, err := a.signIn(r.Context(), appleProvider(), info)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("db"))
		return
	}
	session, err := randToken(32)
	if err != nil || a.store.createSession(r.Context(), session, uid) != nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("session"))
		return
	}
	go a.keepAppleRefresh(uid, in.Code)
	writeJSON(w, http.StatusOK, map[string]string{"token": session})
}

// Account deletion revokes Sign in with Apple (App Store guideline 5.1.1(v)): at sign-in the
// app's one-time authorization code is swapped for a refresh token, kept on the user row, and
// revoked when the account is deleted. Needs a key with "Sign in with Apple" enabled:
//
//	APPLE_SIGNIN_KEY_ID   the key's id (developer.apple.com → Keys)
//	APPLE_SIGNIN_KEY_FILE path to its AuthKey_<id>.p8 (or APPLE_SIGNIN_KEY_P8 with the PEM text)
//	APPLE_TEAM_ID         the team id (falls back to APNS_TEAM_ID)
//
// Unset: sign-in still works, nothing is kept and nothing is revoked.
var appleSigner = sync.OnceValues(func() (*ecdsa.PrivateKey, error) {
	if os.Getenv("APPLE_SIGNIN_KEY_ID") == "" {
		return nil, nil
	}
	return loadP8("APPLE_SIGNIN_KEY_P8", "APPLE_SIGNIN_KEY_FILE")
})

var appleTokenURL, appleRevokeURL = "https://appleid.apple.com/auth/token", "https://appleid.apple.com/auth/revoke"

// appleSecret is the client_secret Apple's token endpoints want; "" when not configured.
func appleSecret() string {
	key, err := appleSigner()
	if key == nil || err != nil {
		if err != nil {
			log.Printf("apple sign-in key: %v", err)
		}
		return ""
	}
	team := os.Getenv("APPLE_TEAM_ID")
	if team == "" {
		team = os.Getenv("APNS_TEAM_ID")
	}
	now := time.Now()
	jwt, err := es256(key, os.Getenv("APPLE_SIGNIN_KEY_ID"), map[string]any{
		"iss": team, "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "aud": appleIssuer, "sub": appleAudience()})
	if err != nil {
		return ""
	}
	return jwt
}

// appleCall posts a form to one of Apple's token endpoints.
func (a *App) appleCall(ctx context.Context, endpoint string, form url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return a.http.Do(req)
}

// keepAppleRefresh swaps the sign-in's authorization code for a refresh token and keeps it.
func (a *App) keepAppleRefresh(uid, code string) {
	secret := appleSecret()
	if secret == "" || code == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := a.appleCall(ctx, appleTokenURL, url.Values{"client_id": {appleAudience()}, "client_secret": {secret},
		"code": {code}, "grant_type": {"authorization_code"}})
	if err != nil {
		log.Printf("apple token: %v", err)
		return
	}
	defer res.Body.Close()
	var out struct {
		RefreshToken string `json:"refresh_token"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&out) != nil || out.RefreshToken == "" {
		log.Printf("apple token: status %d", res.StatusCode)
		return
	}
	if err := a.store.setAppleRefresh(ctx, uid, out.RefreshToken); err != nil {
		log.Printf("apple token: keep: %v", err)
	}
}

// revokeApple ends the app's Sign in with Apple grant (the account was deleted).
func (a *App) revokeApple(token string) {
	secret := appleSecret()
	if secret == "" || token == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := a.appleCall(ctx, appleRevokeURL, url.Values{"client_id": {appleAudience()}, "client_secret": {secret},
		"token": {token}, "token_type_hint": {"refresh_token"}})
	if err != nil {
		log.Printf("apple revoke: %v", err)
		return
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Printf("apple revoke: status %d", res.StatusCode)
	}
}
