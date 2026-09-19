package security

import (
	"strings"
	"testing"
)

const testKey = "test-key-that-is-definitely-long-enough-for-256-bits!!"

func newTestCrypto(t *testing.T) *Crypto {
	t.Helper()
	c, err := New(testKey)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return c
}

func TestNew_RejectsShortKey(t *testing.T) {
	if _, err := New("short"); err == nil {
		t.Error("New() with short key should fail")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c := newTestCrypto(t)
	cases := []string{
		"",
		"hello world",
		"alice@example.com",
		"37.77490000",
		"unicode: héllo wörld 你好",
		strings.Repeat("x", 10000),
	}
	for _, in := range cases {
		enc, err := c.Encrypt(in)
		if err != nil {
			t.Fatalf("Encrypt(%q) error = %v", in, err)
		}
		if in == "" {
			if enc != "" {
				t.Errorf("Encrypt(\"\") = %q, want empty", enc)
			}
			continue
		}
		if !strings.HasPrefix(enc, "v1:") {
			t.Errorf("Encrypt(%q) missing v1 prefix", in)
		}
		got, err := c.Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt() error = %v", err)
		}
		if got != in {
			t.Errorf("round trip = %q, want %q", got, in)
		}
	}
}

func TestDecryptLegacyPlaintext(t *testing.T) {
	c := newTestCrypto(t)
	got, err := c.Decrypt("plain-old-value")
	if err != nil {
		t.Fatalf("Decrypt plaintext error = %v", err)
	}
	if got != "plain-old-value" {
		t.Errorf("Decrypt plaintext = %q, want unchanged", got)
	}
}

func TestEncryptIsNonDeterministic(t *testing.T) {
	c := newTestCrypto(t)
	a1, _ := c.Encrypt("same@example.com")
	a2, _ := c.Encrypt("same@example.com")
	if a1 == a2 {
		t.Error("two encryptions of same plaintext should differ (random nonce)")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	c1, _ := New(testKey)
	c2, _ := New("another-key-also-long-enough-for-256-bits!!!")
	enc, _ := c1.Encrypt("secret")
	if _, err := c2.Decrypt(enc); err == nil {
		t.Error("decrypt with wrong key should fail")
	}
}

func TestEmailHashDeterministic(t *testing.T) {
	c := newTestCrypto(t)
	h1 := c.EmailHash("Alice@Example.COM")
	h2 := c.EmailHash("alice@example.com")
	if h1 != h2 {
		t.Error("EmailHash must be case-insensitive deterministic")
	}
	if len(h1) != 64 {
		t.Errorf("EmailHash length = %d, want 64 hex chars", len(h1))
	}
	if h1 == c.EmailHash("bob@example.com") {
		t.Error("different emails must hash differently")
	}
}

func TestEmailHashKeyDependent(t *testing.T) {
	c1, _ := New(testKey)
	c2, _ := New("different-key-long-enough-for-256-bits-ok!!")
	if c1.EmailHash("x@y.com") == c2.EmailHash("x@y.com") {
		t.Error("same email must hash differently under different keys")
	}
}

func TestEncryptFloatRoundTrip(t *testing.T) {
	c := newTestCrypto(t)
	for _, f := range []float64{37.7749, -122.4194, 0, -90, 180} {
		enc, err := c.EncryptFloat(f)
		if err != nil {
			t.Fatalf("EncryptFloat(%v) error = %v", f, err)
		}
		got, err := c.DecryptFloat(enc)
		if err != nil {
			t.Fatalf("DecryptFloat() error = %v", err)
		}
		if diff := got - f; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("float round trip = %v, want %v", got, f)
		}
	}
}

func TestDecryptGarbageV1(t *testing.T) {
	c := newTestCrypto(t)
	if _, err := c.Decrypt("v1:not-base64!!!"); err == nil {
		t.Error("decrypt of malformed v1 payload should fail")
	}
}
