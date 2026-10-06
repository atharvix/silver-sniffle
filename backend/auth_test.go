package main

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"
)

func TestRedeem(t *testing.T) {
	sum := sha256.Sum256([]byte("good-verifier"))
	ch := base64.RawURLEncoding.EncodeToString(sum[:])

	putHandoff("c1", handoff{uid: "li_x", challenge: ch, exp: time.Now().Add(time.Minute)})
	if _, ok := redeem("c1", "wrong-verifier"); ok {
		t.Fatal("wrong verifier must fail")
	}
	if _, ok := redeem("c1", "good-verifier"); ok {
		t.Fatal("code must be single-use, even after a failed attempt")
	}

	putHandoff("c2", handoff{uid: "li_x", challenge: ch, exp: time.Now().Add(time.Minute)})
	if uid, ok := redeem("c2", "good-verifier"); !ok || uid != "li_x" {
		t.Fatalf("good verifier: got %q %v", uid, ok)
	}

	putHandoff("c3", handoff{uid: "li_x", challenge: ch, exp: time.Now().Add(-time.Second)})
	if _, ok := redeem("c3", "good-verifier"); ok {
		t.Fatal("expired code must fail")
	}
}
