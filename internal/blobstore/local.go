package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// stagingDir holds uploads in progress, on the same file system as the blobs
// so that committing is an atomic rename.
const stagingDir = ".staging"

// Local stores blobs in a directory: <root>/<k[0:2]>/<k[2:4]>/<key>. All
// access goes through os.Root, so no path can escape the directory even if a
// key check were ever bypassed. Files are 0600, directories 0700.
type Local struct {
	dir  string
	root *os.Root
}

// NewLocal opens (creating if needed) the blob directory and checks that it
// is writable, so a wrong volume mount fails at startup, not on first upload.
// Staging files left behind by a crash are removed.
func NewLocal(dir string) (*Local, error) {
	if dir == "" {
		return nil, errors.New("blob directory is empty")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(abs, stagingDir), 0o700); err != nil {
		return nil, fmt.Errorf("create blob directory: %w", err)
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return nil, fmt.Errorf("open blob directory: %w", err)
	}
	l := &Local{dir: abs, root: root}
	f, err := l.CreateTemp()
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("blob directory %s is not writable: %w", abs, err)
	}
	Discard(f)
	l.sweepStaging(time.Now().Add(-time.Hour))
	return l, nil
}

// Name implements Store.
func (l *Local) Name() string { return "local" }

// Close releases the directory handle.
func (l *Local) Close() error { return l.root.Close() }

func (l *Local) sweepStaging(before time.Time) {
	entries, err := fs.ReadDir(l.root.FS(), stagingDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().Before(before) {
			_ = l.root.Remove(filepath.Join(stagingDir, e.Name()))
		}
	}
}

// CreateTemp implements Store. Staging names are random; the directory is ours.
func (l *Local) CreateTemp() (*os.File, error) {
	for range 3 {
		f, err := os.OpenFile(filepath.Join(l.dir, stagingDir, NewKey()), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, err
	}
	return nil, errors.New("could not create a staging file")
}

func blobPath(key string) string {
	return filepath.Join(key[0:2], key[2:4], key)
}

// Commit implements Store: fsync, then rename into place.
func (l *Local) Commit(_ context.Context, key string, f *os.File, _ int64, _ []byte) error {
	defer Discard(f) // no-op for the file once renamed (Remove fails harmlessly)
	if !ValidKey(key) {
		return errBadKey
	}
	if err := f.Sync(); err != nil {
		return err
	}
	rel := blobPath(key)
	if err := l.root.MkdirAll(filepath.Dir(rel), 0o700); err != nil {
		return err
	}
	staged, err := filepath.Rel(l.dir, f.Name())
	if err != nil || filepath.Dir(staged) != stagingDir {
		return errors.New("not a staging file of this store")
	}
	if _, err := l.root.Stat(rel); err == nil {
		return fmt.Errorf("blob %s already exists", key) // random keys never collide; refuse to overwrite
	}
	return l.root.Rename(staged, rel)
}

// Open implements Store.
func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	if !ValidKey(key) {
		return nil, errBadKey
	}
	f, err := l.root.Open(blobPath(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

// Delete implements Store.
func (l *Local) Delete(_ context.Context, key string) error {
	if !ValidKey(key) {
		return errBadKey
	}
	err := l.root.Remove(blobPath(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
