package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

var (
	ErrInvalidCiphertext = errors.New("invalid ciphertext or decryption failure")
	ErrEmptyKey          = errors.New("encryption key cannot be empty")
)

// Encryptor provides AES-256-GCM encryption and decryption capability.
type Encryptor struct {
	key []byte
}

// NewEncryptor creates a new Encryptor using a 32-byte key derived from secret string.
func NewEncryptor(secret string) (*Encryptor, error) {
	if secret == "" {
		return nil, ErrEmptyKey
	}
	// Hash the secret to ensure it produces a 32-byte key for AES-256
	hash := sha256.Sum256([]byte(secret))
	return &Encryptor{key: hash[:]}, nil
}

// EncryptString encrypts a plain text string using AES-256-GCM and returns base64 string.
func (e *Encryptor) EncryptString(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptString decrypts a base64 encoded ciphertext string using AES-256-GCM.
func (e *Encryptor) DecryptString(encryptedBase64 string) (string, error) {
	if encryptedBase64 == "" {
		return "", nil
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encryptedBase64)
	if err != nil {
		return "", ErrInvalidCiphertext
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", ErrInvalidCiphertext
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrInvalidCiphertext
	}

	return string(plaintext), nil
}
