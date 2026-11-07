package main

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
)

// Apple .p8 keys and ES256 JWTs, for Sign in with Apple (apple.go).
// loadP8 reads an Apple .p8 key (EC, PKCS#8 PEM) from the env var holding its text,
// else from the file the other one names.
func loadP8(textVar, fileVar string) (*ecdsa.PrivateKey, error) {
	raw := []byte(os.Getenv(textVar))
	if len(raw) == 0 {
		var err error
		if raw, err = os.ReadFile(os.Getenv(fileVar)); err != nil {
			return nil, err
		}
	}
	blk, _ := pem.Decode(raw)
	if blk == nil {
		return nil, fmt.Errorf("not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, err
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an EC key")
	}
	return ec, nil
}

// es256 signs a JWT the way Apple's APIs want it: ES256, r||s, key id in the header.
func es256(key *ecdsa.PrivateKey, keyID string, claims map[string]any) (string, error) {
	enc := base64.RawURLEncoding
	h, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": keyID})
	c, _ := json.Marshal(claims)
	in := enc.EncodeToString(h) + "." + enc.EncodeToString(c)
	sum := sha256.Sum256([]byte(in))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64) // JWS wants r||s, 32 bytes each, not ASN.1
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return in + "." + enc.EncodeToString(sig), nil
}

// bearer is the ES256 provider token. Apple rejects one older than an hour and
// refreshing more often than every 20 minutes (TooManyProviderTokenUpdates).
