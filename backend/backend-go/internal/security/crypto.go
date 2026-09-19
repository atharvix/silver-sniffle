// Package security provides at-rest encryption helpers for personally
// identifiable information (PII) and deterministic keyed hashing used for
// lookups on encrypted columns.
//
// Design:
//   - AES-256-GCM for all reversible PII fields (email, name, bio, GPS).
//     Each ciphertext is a versioned envelope: "v1:" + base64(nonce || ct).
//     Nonces are random per record, so equal plaintexts encrypt differently.
//   - Deterministic HMAC-SHA256 for columns we must search by (email).
//     The HMAC output is stable for the same input + key, enabling equality
//     lookups and unique constraints without decrypting the whole table.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	encPrefix      = "v1:"
	KeyLen         = 32 // bytes
	hmacCtxEmail   = "kinjo:email_hash:v1"
	hmacCtxProfile = "kinjo:profile_hash:v1"
)

var ErrBadKey = errors.New("security: encryption key must be at least 32 bytes")

type Crypto struct {
	gcm cipher.AEAD
	key string
}

// New builds a Crypto from a raw key string. The key material must be at
// least 32 bytes (256 bits); anything longer is hashed down to exactly 32.
func New(key string) (*Crypto, error) {
	if len(key) < KeyLen {
		return nil, fmt.Errorf("%w (got %d bytes)", ErrBadKey, len(key))
	}
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("security: failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("security: failed to create GCM: %w", err)
	}
	return &Crypto{gcm: gcm, key: string(sum[:])}, nil
}

// Encrypt seals plaintext into a versioned, base64 envelope.
func (c *Crypto) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("security: nonce generation failed: %w", err)
	}
	sealed := c.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt opens a "v1:" envelope. Values without the prefix are returned
// unchanged so legacy plaintext rows keep working during migration.
func (c *Crypto) Decrypt(value string) (string, error) {
	if value == "" || !strings.HasPrefix(value, encPrefix) {
		return value, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encPrefix))
	if err != nil {
		return "", fmt.Errorf("security: invalid ciphertext: %w", err)
	}
	ns := c.gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("security: ciphertext too short")
	}
	pt, err := c.gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("security: decrypt failed (wrong key?): %w", err)
	}
	return string(pt), nil
}

// EncryptEmail returns a lowercase-normalized encrypted email.
func (c *Crypto) EncryptEmail(email string) (string, error) {
	return c.Encrypt(strings.ToLower(strings.TrimSpace(email)))
}

// DecryptEmail reverses EncryptEmail.
func (c *Crypto) DecryptEmail(value string) (string, error) {
	return c.Decrypt(value)
}

// EmailHash returns the deterministic lookup hash for an email.
// The same email always yields the same hash, which is what backs the
// email_hash unique index.
func (c *Crypto) EmailHash(email string) string {
	return c.hmacHex(hmacCtxEmail, strings.ToLower(strings.TrimSpace(email)))
}

// ProfileHash is used to anonymize emails shown on nearby cards.
func (c *Crypto) ProfileHash(email string) string {
	return c.hmacHex(hmacCtxProfile, strings.ToLower(strings.TrimSpace(email)))
}

func (c *Crypto) hmacHex(context, value string) string {
	mac := hmac.New(sha256.New, []byte(context+":"+c.key))
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// EncryptFloat/DecryptFloat encode GPS coordinates as encrypted strings.
func (c *Crypto) EncryptFloat(f float64) (string, error) {
	return c.Encrypt(fmt.Sprintf("%.8f", f))
}

func (c *Crypto) DecryptFloat(value string) (float64, error) {
	pt, err := c.Decrypt(value)
	if err != nil {
		return 0, err
	}
	if pt == "" {
		return 0, nil
	}
	var f float64
	if _, err := fmt.Sscanf(pt, "%g", &f); err != nil {
		return 0, fmt.Errorf("security: bad float payload: %w", err)
	}
	return f, nil
}

