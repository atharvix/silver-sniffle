package main

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestVerifyAppleToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	b64 := base64.RawURLEncoding.EncodeToString
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "k1", "kty": "RSA", "n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	defer srv.Close()
	appleKeysURL = srv.URL
	appleKeys.keys, appleKeys.fetched = nil, time.Time{}

	now := time.Now()
	h := sha256.Sum256([]byte("raw-nonce"))
	good := map[string]any{"iss": appleIssuer, "aud": "com.kinjo.app", "exp": now.Add(5 * time.Minute).Unix(),
		"sub": "001234.abc", "email": "x@privaterelay.appleid.com", "email_verified": "true", "nonce": hex.EncodeToString(h[:])}
	sign := func(k *rsa.PrivateKey, kid string, claims map[string]any) string {
		hd, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": kid})
		cl, _ := json.Marshal(claims)
		in := b64(hd) + "." + b64(cl)
		sum := sha256.Sum256([]byte(in))
		sig, _ := rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, sum[:])
		return in + "." + b64(sig)
	}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for a, b := range good {
			m[a] = b
		}
		m[k] = v
		return m
	}

	info, err := verifyAppleToken(context.Background(), sign(key, "k1", good), "raw-nonce", now)
	if err != nil || info.Sub != "001234.abc" || !info.EmailVerified || info.Email == "" {
		t.Fatalf("good token: %+v %v", info, err)
	}
	bad := map[string]string{
		"other key":      sign(other, "k1", good),
		"unknown kid":    sign(key, "k2", good),
		"wrong issuer":   sign(key, "k1", with("iss", "https://evil")),
		"wrong audience": sign(key, "k1", with("aud", "com.other.app")),
		"expired":        sign(key, "k1", with("exp", now.Add(-time.Minute).Unix())),
		"wrong nonce":    sign(key, "k1", with("nonce", "00")),
		"no subject":     sign(key, "k1", with("sub", "")),
		"malformed":      "a.b",
	}
	for name, tok := range bad {
		if _, err := verifyAppleToken(context.Background(), tok, "raw-nonce", now); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := verifyAppleToken(context.Background(), sign(key, "k1", good), "", now); err == nil {
		t.Error("missing nonce: accepted")
	}
	if i, _ := verifyAppleToken(context.Background(), sign(key, "k1", with("email_verified", false)), "raw-nonce", now); i.EmailVerified {
		t.Error("email_verified false read as true")
	}
}

// An Apple identity gets its own ap_ account, finds it again, and joins an existing
// account only by an email both sides verified.
func TestAppleAccount(t *testing.T) {
	dsn := os.Getenv("KINJO_TEST_DSN")
	if dsn == "" {
		t.Skip("KINJO_TEST_DSN not set")
	}
	ctx := context.Background()
	s, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	n := strconv.FormatInt(time.Now().UnixNano(), 10)
	p := appleProvider()
	uid, created, err := s.upsertUser(ctx, p, userinfo{Sub: "ap-" + n, Email: "relay-" + n + "@privaterelay.appleid.com", EmailVerified: true})
	if err != nil || !created || !strings.HasPrefix(uid, "ap_") {
		t.Fatalf("new apple account: %q %v %v", uid, created, err)
	}
	defer s.deleteAccount(ctx, uid)
	again, created, err := s.upsertUser(ctx, p, userinfo{Sub: "ap-" + n, EmailVerified: true})
	if err != nil || created || again != uid {
		t.Fatalf("returning apple user: %q %v %v", again, created, err)
	}
	mail := "both-" + n + "@example.com"
	em, _, err := s.emailUser(ctx, mail)
	if err != nil {
		t.Fatal(err)
	}
	defer s.deleteAccount(ctx, em)
	if got, _, err := s.upsertUser(ctx, p, userinfo{Sub: "ap2-" + n, Email: mail, EmailVerified: false}); err != nil || got == em {
		t.Fatalf("unverified apple email must not join: %q %v", got, err)
	} else {
		defer s.deleteAccount(ctx, got)
	}
	if got, _, err := s.upsertUser(ctx, p, userinfo{Sub: "ap3-" + n, Email: mail, EmailVerified: true}); err != nil || got != em {
		t.Fatalf("verified apple email should join the email account: %q %v", got, err)
	}
}

// Signing in keeps Apple's refresh token; deleting the account revokes it, with a
// client_secret Apple can verify.
func TestAppleRevokeOnDelete(t *testing.T) {
	dsn := os.Getenv("KINJO_TEST_DSN")
	if dsn == "" {
		t.Skip("KINJO_TEST_DSN not set")
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	t.Setenv("APPLE_SIGNIN_KEY_ID", "KEY123")
	t.Setenv("APPLE_SIGNIN_KEY_P8", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
	t.Setenv("APPLE_TEAM_ID", "TEAM123")

	var mu sync.Mutex
	got := map[string]url.Values{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		mu.Lock()
		got[r.URL.Path] = r.PostForm
		mu.Unlock()
		if r.URL.Path == "/token" {
			json.NewEncoder(w).Encode(map[string]string{"refresh_token": "rt-1"})
		}
	}))
	defer srv.Close()
	appleTokenURL, appleRevokeURL = srv.URL+"/token", srv.URL+"/revoke"

	ctx := context.Background()
	s, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := &App{store: s, hub: newHub(s, nil), http: srv.Client()}
	uid, _, err := s.upsertUser(ctx, appleProvider(), userinfo{Sub: "rv-" + strconv.FormatInt(time.Now().UnixNano(), 10)})
	if err != nil {
		t.Fatal(err)
	}
	a.keepAppleRefresh(uid, "one-time-code")
	if rt, _ := s.appleRefresh(ctx, uid); rt != "rt-1" {
		t.Fatalf("refresh token not kept: %q", rt)
	}
	if f := got["/token"]; f.Get("code") != "one-time-code" || f.Get("grant_type") != "authorization_code" || f.Get("client_id") != "com.kinjo.app" {
		t.Fatalf("token request: %v", f)
	}
	// the client_secret: ES256 by our key, iss team, sub bundle id, aud Apple
	parts := strings.Split(got["/token"].Get("client_secret"), ".")
	if len(parts) != 3 {
		t.Fatal("client_secret is not a JWT")
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(&key.PublicKey, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("client_secret signature")
	}
	var cl map[string]any
	b, _ := base64.RawURLEncoding.DecodeString(parts[1])
	json.Unmarshal(b, &cl)
	if cl["iss"] != "TEAM123" || cl["sub"] != "com.kinjo.app" || cl["aud"] != appleIssuer {
		t.Fatalf("client_secret claims: %v", cl)
	}

	if err := a.removeAccount(ctx, uid); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ { // revocation runs in the background
		mu.Lock()
		f := got["/revoke"]
		mu.Unlock()
		if f != nil {
			if f.Get("token") != "rt-1" || f.Get("token_type_hint") != "refresh_token" {
				t.Fatalf("revoke request: %v", f)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("deleting the account didn't revoke the Apple token")
}
