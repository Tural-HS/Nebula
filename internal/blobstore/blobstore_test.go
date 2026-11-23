package blobstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// AWS's published Signature Version 4 examples for S3 ("Examples: Signature
// Calculations in AWS Signature Version 4").
const (
	awsAccessKey = "AKIAIOSFODNN7EXAMPLE"
	awsSecretKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
)

func TestSignV4MatchesAWSExamples(t *testing.T) {
	when := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)

	get, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	get.Header.Set("Range", "bytes=0-9")
	signV4(get, emptySHA256, "us-east-1", awsAccessKey, awsSecretKey, when)
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := get.Header.Get("Authorization"); got != want {
		t.Fatalf("GET signature:\n got %s\nwant %s", got, want)
	}

	put, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, "https://examplebucket.s3.amazonaws.com/test$file.text",
		strings.NewReader("Welcome to Amazon S3."))
	put.Header.Set("Date", "Fri, 24 May 2013 00:00:00 GMT")
	put.Header.Set("X-Amz-Storage-Class", "REDUCED_REDUNDANCY")
	sum := sha256.Sum256([]byte("Welcome to Amazon S3."))
	signV4(put, hex.EncodeToString(sum[:]), "us-east-1", awsAccessKey, awsSecretKey, when)
	if got := put.Header.Get("Authorization"); !strings.HasSuffix(got,
		"SignedHeaders=date;host;x-amz-content-sha256;x-amz-date;x-amz-storage-class, "+
			"Signature=98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd") {
		t.Fatalf("PUT signature: %s", got)
	}
}

func TestKeys(t *testing.T) {
	a, b := NewKey(), NewKey()
	if !ValidKey(a) || a == b {
		t.Fatalf("keys: %q %q", a, b)
	}
	for _, bad := range []string{"", "../" + a[3:], strings.ToUpper(a), a + "0", a[:63] + "/"} {
		if ValidKey(bad) {
			t.Errorf("ValidKey(%q)", bad)
		}
	}
}

// stage writes data to a staging file of s and returns it with its hash.
func stage(t *testing.T, s Store, data []byte) (*os.File, []byte) {
	t.Helper()
	f, err := s.CreateTemp()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return f, sum[:]
}

// exercise runs the Store contract against a backend.
func exercise(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	data := bytes.Repeat([]byte("nebula "), 10_000)
	key := NewKey()
	f, sum := stage(t, s, data)
	if err := s.Commit(ctx, key, f, int64(len(data)), sum); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err := os.Stat(f.Name()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging file left behind: %v", err)
	}
	rc, err := s.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, data) {
		t.Fatalf("read back %d bytes, want %d", len(got), len(data))
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("deleting a missing blob must succeed: %v", err)
	}
	if _, err := s.Open(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("open after delete: %v", err)
	}
	for _, bad := range []string{"../../etc/passwd", "x", ""} {
		if _, err := s.Open(ctx, bad); !errors.Is(err, errBadKey) {
			t.Errorf("Open(%q): %v", bad, err)
		}
		if err := s.Delete(ctx, bad); !errors.Is(err, errBadKey) {
			t.Errorf("Delete(%q): %v", bad, err)
		}
	}
}

func TestLocal(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	exercise(t, l)

	// Layout and permissions: random key path, 0600 file, nothing else.
	key := NewKey()
	f, sum := stage(t, l, []byte("x"))
	if err := l.Commit(context.Background(), key, f, 1, sum); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, key[:2], key[2:4], key))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("blob mode %v", info.Mode())
	}
	// Commit never overwrites an existing blob.
	f2, sum2 := stage(t, l, []byte("y"))
	if err := l.Commit(context.Background(), key, f2, 1, sum2); err == nil {
		t.Fatal("overwrote an existing blob")
	}
	// A file that is not one of our staging files is refused.
	outside, _ := os.CreateTemp(t.TempDir(), "x")
	if err := l.Commit(context.Background(), NewKey(), outside, 0, sum); err == nil {
		t.Fatal("committed a file outside the staging directory")
	}
}

func TestLocalSweepsOldStagingFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, stagingDir), 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, stagingDir, "old")
	fresh := filepath.Join(dir, stagingDir, "fresh")
	for _, p := range []string{old, fresh} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(old, past, past)
	l, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("stale staging file kept")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("fresh staging file removed (could be an upload in progress)")
	}
}

func TestLocalRejectsUnwritableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	dir := t.TempDir()
	_ = os.Chmod(dir, 0o500)
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if _, err := NewLocal(filepath.Join(dir, "blobs")); err == nil {
		t.Fatal("unwritable directory accepted")
	}
}

// fakeS3 is a minimal in-memory bucket that checks what a real one would:
// a SigV4 Authorization header and the payload hash of PUTs.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	paths   []string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paths = append(f.paths, r.Method+" "+r.URL.Path)
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=test-access/") || !strings.Contains(auth, "/eu-test-1/s3/aws4_request") {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "<Error><Code>AccessDenied</Code></Error>")
		return
	}
	switch r.Method {
	case http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		sum := sha256.Sum256(b)
		if r.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(sum[:]) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "<Error><Code>XAmzContentSHA256Mismatch</Code></Error>")
			return
		}
		f.objects[r.URL.Path] = b
	case http.MethodGet:
		b, ok := f.objects[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
			return
		}
		_, _ = w.Write(b)
	case http.MethodDelete:
		delete(f.objects, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}
}

func TestS3AgainstFake(t *testing.T) {
	fake := &fakeS3{objects: map[string][]byte{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	s, err := NewS3(S3Config{Endpoint: srv.URL, Region: "eu-test-1", Bucket: "files", AccessKey: "test-access",
		SecretKey: "test-secret", PathStyle: true, Prefix: "nebula/", TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	exercise(t, s)
	for _, p := range fake.paths {
		if !strings.HasPrefix(strings.Fields(p)[1], "/files/nebula/") {
			t.Fatalf("object outside bucket prefix: %s", p)
		}
	}

	// A corrupted payload (hash mismatch) is an error, not a silent success.
	f, _ := stage(t, s, []byte("abc"))
	wrong := sha256.Sum256([]byte("abd"))
	if err := s.Commit(context.Background(), NewKey(), f, 3, wrong[:]); err == nil || !strings.Contains(err.Error(), "XAmzContentSHA256Mismatch") {
		t.Fatalf("mismatch: %v", err)
	}
	// Wrong credentials surface the S3 error code only.
	bad, _ := NewS3(S3Config{Endpoint: srv.URL, Region: "eu-test-1", Bucket: "files", AccessKey: "nope", SecretKey: "x", PathStyle: true})
	if _, err := bad.Open(context.Background(), NewKey()); err == nil || !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("bad credentials: %v", err)
	}
}

func TestS3Config(t *testing.T) {
	ok := S3Config{Endpoint: "https://s3.example.test", Region: "r", Bucket: "b", AccessKey: "a", SecretKey: "s"}
	if _, err := NewS3(ok); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*S3Config){
		"path":        func(c *S3Config) { c.Endpoint = "https://s3.example.test/x" },
		"credentials": func(c *S3Config) { c.Endpoint = "https://u:p@s3.example.test" },
		"scheme":      func(c *S3Config) { c.Endpoint = "ftp://s3.example.test" },
		"no bucket":   func(c *S3Config) { c.Bucket = "" },
		"bad bucket":  func(c *S3Config) { c.Bucket = "a/b" },
		"no secret":   func(c *S3Config) { c.SecretKey = "" },
		"prefix":      func(c *S3Config) { c.Prefix = "../x" },
	} {
		c := ok
		mut(&c)
		if _, err := NewS3(c); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	vh, _ := NewS3(ok)
	if u := vh.objectURL("k"); u.String() != "https://b.s3.example.test/k" {
		t.Fatalf("virtual-hosted URL: %s", u)
	}
}

// TestS3Real runs the contract against a real S3-compatible server when
// NEBULA_TEST_S3_ENDPOINT (and _BUCKET, _ACCESS_KEY, _SECRET_KEY, optional
// _REGION) are set, e.g. a throwaway Garage or MinIO container.
func TestS3Real(t *testing.T) {
	endpoint := os.Getenv("NEBULA_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("NEBULA_TEST_S3_ENDPOINT not set")
	}
	region := os.Getenv("NEBULA_TEST_S3_REGION")
	if region == "" {
		region = "us-east-1"
	}
	s, err := NewS3(S3Config{
		Endpoint: endpoint, Region: region, Bucket: os.Getenv("NEBULA_TEST_S3_BUCKET"),
		AccessKey: os.Getenv("NEBULA_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("NEBULA_TEST_S3_SECRET_KEY"),
		PathStyle: true, Prefix: "test/", TempDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	exercise(t, s)
}
