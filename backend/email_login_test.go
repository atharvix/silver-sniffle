package main

import (
	"testing"
	"time"
)

func TestEmailCode(t *testing.T) {
	now := time.Now()
	const e = "ann@acme.example"
	code, ok := reserveCode(e, now)
	if !ok || len(code) != 6 {
		t.Fatalf("first code: %q %v", code, ok)
	}
	if _, ok := reserveCode(e, now.Add(30*time.Second)); ok {
		t.Fatal("second send within a minute should be refused")
	}
	if checkCode(e, "x"+code[1:], now) != "wrong" {
		t.Fatal("wrong code accepted")
	}
	if checkCode(e, code, now) != "" {
		t.Fatal("right code refused")
	}
	if checkCode(e, code, now) != "expired" {
		t.Fatal("a code must work once")
	}
	// a new code after a minute; it expires after codeTTL
	code, _ = reserveCode(e, now.Add(time.Minute))
	if checkCode(e, code, now.Add(time.Minute+codeTTL+time.Second)) != "expired" {
		t.Fatal("expired code accepted")
	}
	// guessing burns the code
	code, _ = reserveCode(e, now.Add(2*time.Minute))
	for i := 1; i < codeTries; i++ {
		checkCode(e, "000000x", now.Add(2*time.Minute))
	}
	if checkCode(e, "000000x", now.Add(2*time.Minute)) != "too_many" || checkCode(e, code, now.Add(2*time.Minute)) != "expired" {
		t.Fatal("code should be burned after too many guesses")
	}
	// hourly cap: 3 sends so far, 2 more allowed, then refused
	reserveCode(e, now.Add(3*time.Minute))
	reserveCode(e, now.Add(4*time.Minute))
	if _, ok := reserveCode(e, now.Add(5*time.Minute)); ok {
		t.Fatal("6th code in an hour should be refused")
	}
	if _, ok := reserveCode(e, now.Add(61*time.Minute)); !ok {
		t.Fatal("sends should be allowed again after an hour")
	}
}

func TestNormEmail(t *testing.T) {
	for in, want := range map[string]string{" Ann@Acme.Example ": "ann@acme.example", "Ann <ann@acme.example>": "", "nope": "", "a@b": "a@b"} {
		if got, _ := normEmail(in); got != want {
			t.Errorf("normEmail(%q) = %q, want %q", in, got, want)
		}
	}
}
