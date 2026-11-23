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

// Open decrypts a ciphertext produced by Seal with the same context.
func (b *Box) Open(ciphertext, context []byte) ([]byte, error) {
	ns := b.aead.NonceSize()
	if len(ciphertext) < 1+ns+b.aead.Overhead() || ciphertext[0] != version {
		return nil, ErrDecrypt
	}
	pt, err := b.aead.Open(nil, ciphertext[1:1+ns], ciphertext[1+ns:], context)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}
