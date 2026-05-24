// Package crypto wraps AES-256-GCM as an at-rest cipher for short secrets
// (mostly SMTP passwords) stored in the settings table. The key is derived
// from SESSION_SECRET so the admin does not need to manage a second key.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// DeriveKey returns a 32-byte key from the configured session secret. SHA-256
// uniformizes inputs of arbitrary length (the env helper accepts any string
// the admin pastes) to the exact size AES-256 expects.
func DeriveKey(secret []byte) [32]byte {
	return sha256.Sum256(secret)
}

// Seal encrypts plaintext and returns "v1:<base64(nonce ‖ ciphertext)>". The
// prefix lets us evolve the format later without misreading old rows. Empty
// plaintext seals to "" so an unset value round-trips through the settings
// table without surprises.
func Seal(plaintext string, key [32]byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: read nonce: %w", err)
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "v1:" + base64.StdEncoding.EncodeToString(ct), nil
}

// Open reverses Seal. Returns ErrBadFormat on unknown prefixes (so a row
// rewritten by hand fails loudly instead of decrypting to garbage) and
// passes the underlying GCM error through on auth failure.
func Open(token string, key [32]byte) (string, error) {
	if token == "" {
		return "", nil
	}
	if len(token) < 3 || token[:3] != "v1:" {
		return "", ErrBadFormat
	}
	raw, err := base64.StdEncoding.DecodeString(token[3:])
	if err != nil {
		return "", fmt.Errorf("crypto: base64 decode: %w", err)
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new gcm: %w", err)
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return "", ErrBadFormat
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: gcm open: %w", err)
	}
	return string(pt), nil
}

// ErrBadFormat indicates the stored token does not match the current schema.
// Callers usually surface this as "configuración corrupta — guarde la
// contraseña otra vez".
var ErrBadFormat = fmt.Errorf("crypto: unrecognized cipher format")
