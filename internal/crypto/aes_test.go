package crypto

import (
	"encoding/hex"
	"errors"
	"testing"
)

func validHexKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return hex.EncodeToString(key)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	enc, err := NewEncryptor(validHexKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	plaintext := "hello, world! 你好世界"
	ct, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if got != plaintext {
		t.Errorf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestEncryptProducesDifferentCiphertext(t *testing.T) {
	enc, err := NewEncryptor(validHexKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	plaintext := "same input"
	ct1, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt #1: %v", err)
	}

	ct2, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt #2: %v", err)
	}

	if ct1 == ct2 {
		t.Error("expected different ciphertexts for the same plaintext (random nonce), got identical")
	}
}

func TestDecryptInvalidCiphertext(t *testing.T) {
	enc, err := NewEncryptor(validHexKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	cases := []string{
		"not-valid-base64!!!",
		"aGVsbG8=", // valid base64 but not valid ciphertext
		"",
	}

	for _, input := range cases {
		_, err := enc.Decrypt(input)
		if !errors.Is(err, ErrDecryptFailed) {
			t.Errorf("Decrypt(%q): got err=%v, want %v", input, err, ErrDecryptFailed)
		}
	}
}

func TestNewEncryptorInvalidKey(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{"too short", "aabbccdd"},
		{"not hex", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
		{"empty", ""},
		{"31 bytes", hex.EncodeToString(make([]byte, 31))},
		{"33 bytes", hex.EncodeToString(make([]byte, 33))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewEncryptor(tc.key)
			if !errors.Is(err, ErrInvalidKey) {
				t.Errorf("NewEncryptor(%q): got err=%v, want %v", tc.key, err, ErrInvalidKey)
			}
		})
	}
}

func TestEncryptEmptyString(t *testing.T) {
	enc, err := NewEncryptor(validHexKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	ct, err := enc.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}

	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
