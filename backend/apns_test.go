package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPNs(t *testing.T) {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	t.Setenv("APNS_KEY_ID", "ABC123DEFG")
	t.Setenv("APNS_TEAM_ID", "TEAM123456")
	t.Setenv("APNS_KEY_P8", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
	a, err := newAPNs()
	if err != nil || a == nil {
		t.Fatal(err)
	}
	jwt, err := a.bearer()
	if err != nil {
		t.Fatal(err)
	}
	p := strings.Split(jwt, ".")
	sig, _ := base64.RawURLEncoding.DecodeString(p[2])
	sum := sha256.Sum256([]byte(p[0] + "." + p[1]))
	if len(sig) != 64 || !ecdsa.Verify(&k.PublicKey, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("bad signature")
	}
	var h map[string]string
	hb, _ := base64.RawURLEncoding.DecodeString(p[0])
	json.Unmarshal(hb, &h)
	if h["alg"] != "ES256" || h["kid"] != "ABC123DEFG" {
		t.Fatal(h)
	}
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.Path+" "+r.Header.Get("apns-push-type")+" "+r.Header.Get("apns-topic"))
		if strings.HasSuffix(r.URL.Path, "dead") {
			w.WriteHeader(410)
			w.Write([]byte(`{"reason":"Unregistered"}`))
		}
	}))
	defer srv.Close()
	a.host = srv.URL
	inv := a.sendLocation(context.Background(), []string{"aa11", "dead"})
	if len(inv) != 1 || inv[0] != "dead" || got[0] != "/3/device/aa11 location com.kinjo.app.location-query" {
		t.Fatal(inv, got)
	}
}
