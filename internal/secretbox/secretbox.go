// Package secretbox encrypts small secrets (e.g. webhook signing keys) at rest
// with AES-256-GCM. Ciphertext format: version byte | 12-byte nonce | sealed data.
// The row ID is bound as additional data, so a ciphertext copied to another
// row fails to decrypt.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

const version byte = 1

// ErrDecrypt is returned for tampered, truncated or foreign ciphertexts.
var ErrDecrypt = errors.New("secretbox: decryption failed")

// Box seals and opens secrets with one 32-byte key.
type Box struct {
	aead cipher.AEAD
	// old keys are used only to decrypt, during key rotation.
	old []cipher.AEAD
}

// New builds a Box from a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secretbox: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// NewFromBase64 decodes a standard base64 key (e.g. `openssl rand -base64 32`).
func NewFromBase64(s string) (*Box, error) {
	key, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, errors.New("secretbox: key is not valid base64")
	}
	return New(key)
}

// Seal encrypts plaintext bound to context (e.g. the owning row's ID).
func (b *Box) Seal(plaintext, context []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+len(nonce)+len(plaintext)+b.aead.Overhead())
	out = append(out, version)
	out = append(out, nonce...)
	return b.aead.Seal(out, nonce, plaintext, context), nil
}

// WithOldKeys adds retired keys (base64) that may still decrypt existing data.
// New data is always sealed with the primary key.
func (b *Box) WithOldKeys(keys ...string) (*Box, error) {
	for i, k := range keys {
		old, err := NewFromBase64(k)
		if err != nil {
			return nil, fmt.Errorf("old key %d: %w", i+1, err)
		}
		b.old = append(b.old, old.aead)
	}
	return b, nil
}

// Open decrypts a ciphertext produced by Seal with the same context, trying
// the primary key first and then any old keys.
func (b *Box) Open(ciphertext, context []byte) ([]byte, error) {
	pt, _, err := b.OpenWithInfo(ciphertext, context)
	return pt, err
}

// OpenWithInfo is Open that also reports whether a retired key was needed,
// i.e. whether the value should be re-encrypted.
func (b *Box) OpenWithInfo(ciphertext, context []byte) (plaintext []byte, usedOldKey bool, err error) {
	ns := b.aead.NonceSize()
	if len(ciphertext) < 1+ns+b.aead.Overhead() || ciphertext[0] != version {
		return nil, false, ErrDecrypt
	}
	for i, a := range append([]cipher.AEAD{b.aead}, b.old...) {
		if pt, err := a.Open(nil, ciphertext[1:1+ns], ciphertext[1+ns:], context); err == nil {
			return pt, i > 0, nil
		}
	}
	return nil, false, ErrDecrypt
}
