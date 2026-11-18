package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Field encryption for credentials kept in Postgres (Apple refresh tokens), so a
// database dump or a leaked backup alone can't use them. AES-256-GCM under DATA_KEY
// (base64 of 32 random bytes: openssl rand -base64 32). Sealed values read
// "v1:" + base64(nonce | ciphertext). Rows written before the key was set stay
// readable as they are and are sealed on their next write. Losing the key makes
// sealed values unreadable: keep it with the other secrets, never in the database.
var dataKey cipher.AEAD

const sealPrefix = "v1:"

func setDataKey(b64 string) error {
	if b64 == "" {
		dataKey = nil
		return nil
	}
	k, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(k) != 32 {
		return errors.New("DATA_KEY must be base64 of 32 bytes (openssl rand -base64 32)")
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return err
	}
	dataKey, err = cipher.NewGCM(block)
	return err
}

// seal encrypts s; without DATA_KEY it returns s unchanged.
func seal(s string) (string, error) {
	if dataKey == nil || s == "" {
		return s, nil
	}
	nonce := make([]byte, dataKey.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return sealPrefix + base64.StdEncoding.EncodeToString(dataKey.Seal(nonce, nonce, []byte(s), nil)), nil
}

// unseal reverses seal; values without the prefix were stored before encryption.
func unseal(s string) (string, error) {
	if !strings.HasPrefix(s, sealPrefix) {
		return s, nil
	}
	if dataKey == nil {
		return "", errors.New("sealed value but DATA_KEY is not set")
	}
	b, err := base64.StdEncoding.DecodeString(s[len(sealPrefix):])
	n := dataKey.NonceSize()
	if err != nil || len(b) < n {
		return "", errors.New("sealed value is malformed")
	}
	out, err := dataKey.Open(nil, b[:n], b[n:], nil)
	if err != nil {
		return "", fmt.Errorf("unseal: %w", err)
	}
	return string(out), nil
}
