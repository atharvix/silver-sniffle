package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// APNs sends iPhone location pushes straight to Apple: FCM can't, because the token from
// startMonitoringLocationPushes is a raw APNs token, not an FCM registration token, and the
// location push type only accepts token (.p8) auth. Each push starts the app's Location Push
// Service Extension, which posts one fix. Apple caps a device at ~360 a day.
type APNs struct {
	keyID, teamID, topic, host string
	key                        *ecdsa.PrivateKey
	http                       *http.Client // net/http speaks HTTP/2 to Apple by itself (ALPN)

	mu    sync.Mutex
	jwt   string
	jwtAt time.Time
}

// newAPNs returns (nil, nil) when APNS_KEY_ID is unset: iPhones then get only the FCM wake.
//
//	APNS_KEY_ID    the key's id (developer.apple.com → Keys; same key as uploaded to Firebase is fine)
//	APNS_TEAM_ID   the Apple team id
//	APNS_KEY_FILE  path to AuthKey_<id>.p8   (or APNS_KEY_P8 with the PEM text)
//	APNS_BUNDLE_ID defaults to com.kinjo.app
//	APNS_SANDBOX   1 for apps run from Xcode (development tokens); TestFlight/App Store use production
func newAPNs() (*APNs, error) {
	keyID := os.Getenv("APNS_KEY_ID")
	if keyID == "" {
		return nil, nil
	}
	raw := []byte(os.Getenv("APNS_KEY_P8"))
	if len(raw) == 0 {
		var err error
		if raw, err = os.ReadFile(os.Getenv("APNS_KEY_FILE")); err != nil {
			return nil, fmt.Errorf("apns key: %w", err)
		}
	}
	blk, _ := pem.Decode(raw)
	if blk == nil {
		return nil, fmt.Errorf("apns key: not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("apns key: %w", err)
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("apns key: not an EC key")
	}
	teamID := os.Getenv("APNS_TEAM_ID")
	if teamID == "" {
		return nil, fmt.Errorf("apns: APNS_TEAM_ID not set")
	}
	bundle := os.Getenv("APNS_BUNDLE_ID")
	if bundle == "" {
		bundle = "com.kinjo.app"
	}
	host := "https://api.push.apple.com"
	if os.Getenv("APNS_SANDBOX") == "1" {
		host = "https://api.sandbox.push.apple.com"
	}
	return &APNs{keyID: keyID, teamID: teamID, topic: bundle + ".location-query", host: host, key: ec,
		http: &http.Client{Timeout: 10 * time.Second}}, nil
}

// bearer is the ES256 provider token. Apple rejects one older than an hour and
// refreshing more often than every 20 minutes (TooManyProviderTokenUpdates).
func (a *APNs) bearer() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.jwt != "" && time.Since(a.jwtAt) < 40*time.Minute {
		return a.jwt, nil
	}
	enc := base64.RawURLEncoding
	h, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": a.keyID})
	now := time.Now()
	c, _ := json.Marshal(map[string]any{"iss": a.teamID, "iat": now.Unix()})
	in := enc.EncodeToString(h) + "." + enc.EncodeToString(c)
	sum := sha256.Sum256([]byte(in))
	r, s, err := ecdsa.Sign(rand.Reader, a.key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64) // JWS wants r||s, 32 bytes each, not ASN.1
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	a.jwt, a.jwtAt = in+"."+enc.EncodeToString(sig), now
	return a.jwt, nil
}

// sendLocation asks each device's extension for a fix now. Returns the tokens Apple
// says are dead, for the caller to delete.
func (a *APNs) sendLocation(ctx context.Context, tokens []string) (invalid []string) {
	if a == nil || len(tokens) == 0 {
		return nil
	}
	jwt, err := a.bearer()
	if err != nil {
		log.Printf("apns: token: %v", err)
		return nil
	}
	ok := 0
	defer func() { tel.pushed(ok, len(tokens)-ok-len(invalid), len(invalid)) }()
	body := []byte(`{"kind":"wake"}`)                                    // the extension ignores it
	exp := strconv.FormatInt(time.Now().Add(120*time.Second).Unix(), 10) // useless once the card expired
	for _, t := range tokens {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.host+"/3/device/"+t, bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("authorization", "bearer "+jwt)
		req.Header.Set("apns-push-type", "location")
		req.Header.Set("apns-topic", a.topic)
		req.Header.Set("apns-priority", "10") // 5 can wait until the phone wakes for something else
		req.Header.Set("apns-expiration", exp)
		resp, err := a.http.Do(req)
		if err != nil {
			continue
		}
		var e struct{ Reason string }
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<12)).Decode(&e)
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusOK:
			ok++
		case resp.StatusCode == http.StatusGone, e.Reason == "BadDeviceToken", e.Reason == "Unregistered", e.Reason == "DeviceTokenNotForTopic":
			invalid = append(invalid, t)
		case e.Reason == "ExpiredProviderToken", e.Reason == "InvalidProviderToken":
			a.mu.Lock()
			a.jwt = "" // mint a new one next time
			a.mu.Unlock()
			log.Printf("apns location push: %d %s", resp.StatusCode, e.Reason)
		default:
			log.Printf("apns location push: %d %s", resp.StatusCode, e.Reason) // reason only: no token
		}
	}
	return invalid
}
