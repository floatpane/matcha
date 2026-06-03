package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	salt := make([]byte, 32)

	plaintext := []byte("hello")
	key := DeriveKey("password", salt)

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Errorf("mismatch: got %q, want %q", got, plaintext)
	}
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	salt := make([]byte, 32)
	key := DeriveKey("password", salt)

	ciphertext, err := Encrypt([]byte{}, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("expected empty plaintext, got %q", got)
	}
}

func TestEncryptDecrypt_LongPassword(t *testing.T) {
	salt := make([]byte, 32)

	plaintext := []byte("hello")
	key := DeriveKey(strings.Repeat("x", 10000), salt)

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Errorf("mismatch: got %q, want %q", got, plaintext)
	}
}

func TestEncryptDecrypt_LongPlaintext(t *testing.T) {
	salt := make([]byte, 32)

	plaintext := bytes.Repeat([]byte("hello"), 1<<20)
	key := DeriveKey("password", salt)

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Error("long plaintext round-trip mismatch")
	}
}

func TestEncrypt_NonceIsRandom(t *testing.T) {
	salt := make([]byte, 32)

	plaintext := []byte("hello")
	key := DeriveKey("password", salt)

	c1, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt 1: %v", err)
	}
	c2, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt 2: %v", err)
	}

	if bytes.Equal(c1, c2) {
		t.Error("two Encrypt calls produced identical ciphertext")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	s1 := make([]byte, 32)
	s2 := bytes.Repeat([]byte{1}, 32)

	k1 := DeriveKey("p1", s1)
	k2 := DeriveKey("p2", s2)

	ciphertext, err := Encrypt([]byte("hello"), k1)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if _, err := Decrypt(ciphertext, k2); err == nil {
		t.Error("Decrypt with wrong key should return an error")
	}
}

func TestDecrypt_CorruptedCiphertext(t *testing.T) {
	salt := make([]byte, 32)

	plaintext := []byte("hello")
	key := DeriveKey("password", salt)

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	ciphertext[0] ^= 0xff

	if _, err := Decrypt(ciphertext, key); err == nil {
		t.Error("Decrypt with corrupted ciphertext should return an error")
	}
}

func TestDeriveKey_Deterministic(t *testing.T) {
	salt := make([]byte, 32)

	k1 := DeriveKey("password", salt)
	k2 := DeriveKey("password", salt)

	if !bytes.Equal(k1, k2) {
		t.Error("DeriveKey should be deterministic for the same input")
	}
}

func TestDeriveKey_DifferentPasswords(t *testing.T) {
	salt := make([]byte, 32)

	k1 := DeriveKey("password1", salt)
	k2 := DeriveKey("password2", salt)

	if bytes.Equal(k1, k2) {
		t.Error("different passwords should produce different keys")
	}
}

func TestDeriveKey_DifferentSalts(t *testing.T) {
	s1 := make([]byte, 32)
	s2 := bytes.Repeat([]byte{1}, 32)

	k1 := DeriveKey("password", s1)
	k2 := DeriveKey("password", s2)

	if bytes.Equal(k1, k2) {
		t.Error("different salts should produce different keys")
	}
}
