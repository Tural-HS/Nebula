package secretbox

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
)

func newBox(t *testing.T) *Box {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	b, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRoundTrip(t *testing.T) {
	b := newBox(t)
	ct, err := b.Seal([]byte("whsec_abc"), []byte("row-1"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, []byte("whsec_abc")) {
		t.Fatal("plaintext visible in ciphertext")
	}
	pt, err := b.Open(ct, []byte("row-1"))
	if err != nil || string(pt) != "whsec_abc" {
		t.Fatalf("open: %q %v", pt, err)
	}
	ct2, _ := b.Seal([]byte("whsec_abc"), []byte("row-1"))
	if bytes.Equal(ct, ct2) {
		t.Fatal("nonce reuse: identical ciphertexts")
	}
}

func TestOpenRejects(t *testing.T) {
	b := newBox(t)
	ct, _ := b.Seal([]byte("secret"), []byte("row-1"))
	tampered := bytes.Clone(ct)
	tampered[len(tampered)-1] ^= 1

	cases := map[string]struct {
		box *Box
		ct  []byte
		ctx string
	}{
		"other row":   {b, ct, "row-2"},
		"tampered":    {b, tampered, "row-1"},
		"truncated":   {b, ct[:10], "row-1"},
		"empty":       {b, nil, "row-1"},
		"bad version": {b, append([]byte{9}, ct[1:]...), "row-1"},
		"other key":   {newBox(t), ct, "row-1"},
	}
	for name, c := range cases {
		if _, err := c.box.Open(c.ct, []byte(c.ctx)); !errors.Is(err, ErrDecrypt) {
			t.Errorf("%s: want ErrDecrypt, got %v", name, err)
		}
	}
}

func TestKeyValidation(t *testing.T) {
	if _, err := New(make([]byte, 16)); err == nil {
		t.Error("16-byte key accepted")
	}
	if _, err := NewFromBase64("not base64!"); err == nil {
		t.Error("invalid base64 accepted")
	}
	if _, err := NewFromBase64(base64.StdEncoding.EncodeToString(make([]byte, 32))); err != nil {
		t.Errorf("valid key rejected: %v", err)
	}
}

func TestKeyRotation(t *testing.T) {
	keyA := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	keyB := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	a, _ := NewFromBase64(keyA)
	ct, _ := a.Seal([]byte("whsec_old"), []byte("row"))

	b, _ := NewFromBase64(keyB)
	if _, err := b.Open(ct, []byte("row")); !errors.Is(err, ErrDecrypt) {
		t.Fatal("new key alone must not open old data")
	}
	b, err := b.WithOldKeys(keyA)
	if err != nil {
		t.Fatal(err)
	}
	pt, old, err := b.OpenWithInfo(ct, []byte("row"))
	if err != nil || string(pt) != "whsec_old" || !old {
		t.Fatalf("open with old key: %q old=%v err=%v", pt, old, err)
	}
	re, _ := b.Seal(pt, []byte("row"))
	if _, old, err := b.OpenWithInfo(re, []byte("row")); err != nil || old {
		t.Fatalf("re-encrypted value must use the primary key: old=%v err=%v", old, err)
	}
	if _, err := b.WithOldKeys("not-base64"); err == nil {
		t.Fatal("invalid old key accepted")
	}
}
