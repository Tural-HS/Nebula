// Package apitoken generates, parses and verifies workspace service tokens
// (ADR-0043). A token is "nbl_<id>_<secret>": id is the token's UUID as 32
// lowercase hex digits, secret is 32 random bytes as 64 lowercase hex digits.
// Only SHA-256(secret) is stored. The secret has 256 bits of entropy, so a
// fast hash is enough: there is nothing to brute-force, and a slow KDF would
// only add a CPU cost an attacker could trigger with bad tokens.
package apitoken

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// Prefix starts every token; secret scanners can match `nbl_[0-9a-f]{32}_[0-9a-f]{64}`.
const Prefix = "nbl_"

const (
	idLen     = 32
	secretLen = 64
	// Len is the length of a well-formed token.
	Len = len(Prefix) + idLen + 1 + secretLen
)

// ErrMalformed means the string is not a well-formed service token.
var ErrMalformed = errors.New("malformed service token")

// Looks reports whether raw claims to be a service token (has the prefix).
// Such a bearer value is never sent to the OIDC verifier.
func Looks(raw string) bool { return strings.HasPrefix(raw, Prefix) }

// Generate returns a new token string for id and the hash to store.
func Generate(id uuid.UUID) (token string, hash []byte, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", nil, err
	}
	secret := hex.EncodeToString(b[:])
	return Prefix + hex.EncodeToString(id[:]) + "_" + secret, Hash(secret), nil
}

// Parse splits a token into its id and secret. It accepts only the exact
// format (lowercase hex, fixed lengths), so lookups never see odd input.
func Parse(raw string) (uuid.UUID, string, error) {
	if len(raw) != Len || !strings.HasPrefix(raw, Prefix) || raw[len(Prefix)+idLen] != '_' {
		return uuid.Nil, "", ErrMalformed
	}
	idHex := raw[len(Prefix) : len(Prefix)+idLen]
	secret := raw[len(Prefix)+idLen+1:]
	if !isLowerHex(idHex) || !isLowerHex(secret) {
		return uuid.Nil, "", ErrMalformed
	}
	var id uuid.UUID
	if _, err := hex.Decode(id[:], []byte(idHex)); err != nil {
		return uuid.Nil, "", ErrMalformed
	}
	return id, secret, nil
}

// Hash is the stored form of a secret.
func Hash(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// Verify compares secret with a stored hash in constant time.
func Verify(secret string, stored []byte) bool {
	return len(stored) == sha256.Size && subtle.ConstantTimeCompare(Hash(secret), stored) == 1
}

func isLowerHex(s string) bool {
	for i := range len(s) {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
