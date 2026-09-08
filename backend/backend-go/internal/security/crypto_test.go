package security

import (
	"testing"
)

func TestAES256EncryptDecrypt(t *testing.T) {
	secret := "super-secret-master-key-1234567890"
	enc, err := NewEncryptor(secret)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	plainText := "User private bio with sensitive data: +1-555-0199"
	encrypted, err := enc.EncryptString(plainText)
	if err != nil {
		t.Fatalf("EncryptString failed: %v", err)
	}

	if encrypted == plainText {
		t.Fatalf("Encrypted string matches plain text, expected ciphertext")
	}

	decrypted, err := enc.DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString failed: %v", err)
	}

	if decrypted != plainText {
		t.Errorf("Expected decrypted %q, got %q", plainText, decrypted)
	}
}

func TestEmptySecret(t *testing.T) {
	_, err := NewEncryptor("")
	if err == nil {
		t.Errorf("Expected error for empty secret key, got nil")
	}
}

func TestEmptyString(t *testing.T) {
	enc, _ := NewEncryptor("key")
	res, err := enc.EncryptString("")
	if err != nil || res != "" {
		t.Errorf("Expected empty string response, got res=%q err=%v", res, err)
	}
}
