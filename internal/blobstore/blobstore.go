// Package blobstore keeps attachment bytes outside the database (ADR-0022).
//
// Blobs are addressed by random keys the server generates (64 hex characters);
// user-supplied names never reach the file system or the bucket. An upload is
// first written to a staging file (so its size and hash are known and limits
// are enforced before anything is stored for good), then committed under its
// key in one step.
package blobstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"regexp"
)

// ErrNotFound means no blob exists under the key.
var ErrNotFound = errors.New("blob not found")

// Store is a blob backend.
type Store interface {
	// CreateTemp returns an empty staging file for an upload. The caller
	// either passes it to Commit or calls Discard.
	CreateTemp() (*os.File, error)
	// Commit stores the fully written staging file under key. size and sum
	// (SHA-256) describe its content. f is closed and removed in every case.
	Commit(ctx context.Context, key string, f *os.File, size int64, sum []byte) error
	// Open streams a blob. It returns ErrNotFound when it does not exist.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes a blob; a missing blob is not an error.
	Delete(ctx context.Context, key string) error
	// Name is the backend name for logs ("local", "s3").
	Name() string
}

var keyRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// NewKey returns a fresh random storage key (256 bits, hex).
func NewKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // never fails (crypto/rand panics on a broken system source)
	return hex.EncodeToString(b)
}

// ValidKey reports whether key has the form NewKey produces.
func ValidKey(key string) bool { return keyRe.MatchString(key) }

// Discard closes and removes a staging file that will not be committed.
func Discard(f *os.File) {
	if f == nil {
		return
	}
	_ = f.Close()
	_ = os.Remove(f.Name())
}

var errBadKey = errors.New("invalid blob key")
