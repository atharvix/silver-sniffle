package main

import (
	"strings"
	"testing"
)

func TestSealRoundTrip(t *testing.T) {
	defer setDataKey("")
	if err := setDataKey("short"); err == nil {
		t.Fatal("a key that isn't 32 bytes must be refused")
	}
	if err := setDataKey("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="); err != nil {
		t.Fatal(err)
	}
	a, _ := seal("r.secret")
	b, _ := seal("r.secret")
	if !strings.HasPrefix(a, sealPrefix) || strings.Contains(a, "secret") || a == b {
		t.Fatalf("sealed value must be prefixed, opaque and randomised: %q %q", a, b)
	}
	if got, err := unseal(a); err != nil || got != "r.secret" {
		t.Fatalf("unseal = %q, %v", got, err)
	}
	if got, _ := unseal("legacy-plaintext"); got != "legacy-plaintext" {
		t.Fatal("rows stored before the key must stay readable")
	}
	if _, err := unseal(a[:len(a)-4] + "AAAA"); err == nil {
		t.Fatal("a tampered value must fail")
	}
	setDataKey("")
	if _, err := unseal(a); err == nil {
		t.Fatal("a sealed value without the key must fail, not pass through")
	}
}
