package apitoken

import (
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestGenerateParseVerify(t *testing.T) {
	id := uuid.New()
	tok, hash, err := Generate(id)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^nbl_[0-9a-f]{32}_[0-9a-f]{64}$`).MatchString(tok) || len(tok) != Len {
		t.Fatalf("format: %q", tok)
	}
	if !Looks(tok) || Looks("eyJhbGciOi") {
		t.Fatal("Looks")
	}
	gotID, secret, err := Parse(tok)
	if err != nil || gotID != id {
		t.Fatalf("parse: %v %v", gotID, err)
	}
	if !Verify(secret, hash) {
		t.Fatal("verify own secret")
	}
	if strings.Contains(string(hash), secret) || len(hash) != 32 {
		t.Fatal("hash must be a 32-byte digest")
	}
	// One flipped character fails.
	bad := []byte(secret)
	bad[0] ^= 1
	if Verify(string(bad), hash) {
		t.Fatal("verify wrong secret")
	}
	if Verify(secret, hash[:31]) || Verify(secret, nil) {
		t.Fatal("verify short hash")
	}
	// Two tokens never share a secret.
	tok2, _, _ := Generate(id)
	if tok2 == tok {
		t.Fatal("secrets repeat")
	}
}

func TestParseRejects(t *testing.T) {
	id := strings.Repeat("a", 32)
	sec := strings.Repeat("b", 64)
	for _, raw := range []string{
		"",
		"nbl_",
		"nbl_" + id + sec,                        // no separator
		"nbl_" + id + "-" + sec,                  // wrong separator
		"nbl_" + strings.ToUpper(id) + "_" + sec, // uppercase
		"nbl_" + id + "_" + sec[:63],             // short
		"nbl_" + id + "_" + sec + "c",            // long
		"nbx_" + id + "_" + sec,                  // prefix
		"nbl_" + id[:31] + "g" + "_" + sec,       // non-hex id
		"nbl_" + id + "_" + sec[:63] + "_",       // non-hex secret
		" nbl_" + id + "_" + sec[:63],            // leading space
	} {
		if _, _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) accepted", raw)
		}
	}
	if _, _, err := Parse("nbl_" + id + "_" + sec); err != nil {
		t.Errorf("valid rejected: %v", err)
	}
}
