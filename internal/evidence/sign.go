// Package evidence builds and verifies signed audit evidence packages
// (ADR-0029): a ZIP with a human-readable report, machine-readable data and
// a manifest of SHA-256 hashes signed with the server's Ed25519 key.
package evidence

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// Format identifies the package layout; Verify rejects anything else.
const Format = "nebula-evidence/1"

// signContext is prepended to the manifest before signing (domain
// separation): a signature over a manifest can never be replayed as a
// signature over anything else this key might sign later.
const signContext = "nebula-evidence-v1\n"

// Signer holds the server's signing key.
type Signer struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

// ParseSigningKey reads NEBULA_EVIDENCE_SIGNING_KEY: a base64 Ed25519 seed
// (32 bytes, `openssl rand -base64 32`). Errors never contain the value.
func ParseSigningKey(b64 string) (*Signer, error) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("NEBULA_EVIDENCE_SIGNING_KEY must be 32 bytes, base64 encoded (openssl rand -base64 32)")
	}
	priv := ed25519.NewKeyFromSeed(seed)
	return &Signer{priv: priv, pub: priv.Public().(ed25519.PublicKey)}, nil
}

// PublicKey is the base64 public key.
func (s *Signer) PublicKey() string { return base64.StdEncoding.EncodeToString(s.pub) }

// Fingerprint is the key's short, comparable form ("SHA256:…", as in ssh-keygen).
func (s *Signer) Fingerprint() string { return Fingerprint(s.pub) }

// Fingerprint of a raw Ed25519 public key.
func Fingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

func (s *Signer) sign(manifest []byte) []byte {
	return ed25519.Sign(s.priv, append([]byte(signContext), manifest...))
}

func verifySignature(pub ed25519.PublicKey, manifest, sig []byte) bool {
	return len(pub) == ed25519.PublicKeySize && ed25519.Verify(pub, append([]byte(signContext), manifest...), sig)
}

// SigningKeyInfo is what Settings shows and the manifest records.
type SigningKeyInfo struct {
	Algorithm   string `json:"algorithm"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
}

// Info describes the key.
func (s *Signer) Info() SigningKeyInfo {
	return SigningKeyInfo{Algorithm: "Ed25519", PublicKey: s.PublicKey(), Fingerprint: s.Fingerprint()}
}

// Signature is the content of manifest.sig.
type Signature struct {
	Algorithm      string `json:"algorithm"`
	KeyFingerprint string `json:"key_fingerprint"`
	Signature      string `json:"signature"` // base64 Ed25519 over signContext + manifest.json bytes
}
