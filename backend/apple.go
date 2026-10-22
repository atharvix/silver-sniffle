package main

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
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

// appleLogin: POST /auth/apple {idToken, nonce} → {token}.
func (a *App) appleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct{ IDToken, Nonce string }
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
	writeJSON(w, http.StatusOK, map[string]string{"token": session})
}
