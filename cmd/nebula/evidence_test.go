package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/evidence"
	"github.com/Tural-HS/Nebula/internal/store"
)

func writePackage(t *testing.T, dir, name string, s *evidence.Signer, events []store.EvidenceEvent, asOf time.Time) string {
	t.Helper()
	start, end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	set := &store.EvidenceSet{Workspace: store.Workspace{ID: uuid.MustParse("0195f3c4-0000-7000-8000-000000000001"), Slug: "acme", Name: "Acme",
		Timezone: "UTC"}, AsOf: asOf, Events: events, RangeStart: &start, RangeEnd: &end}
	var buf bytes.Buffer
	if _, err := evidence.Build(&buf, evidence.Input{ExportID: uuid.New(), Kind: evidence.KindScope,
		Params: evidence.Params{From: "2026-10-01", To: "2026-10-31"}, ExportedAt: asOf, Data: set}, s); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEvidenceVerifyCommand(t *testing.T) {
	seed := make([]byte, 32)
	_, _ = rand.Read(seed)
	s, _ := evidence.ParseSigningKey(base64.StdEncoding.EncodeToString(seed))
	dir := t.TempDir()
	at := time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC)
	ev := func(id int64, title string) store.EvidenceEvent {
		return store.EvidenceEvent{ID: id, OccurredAt: at, Action: "item.updated", EntityType: "item", EntityID: uuid.Nil,
			Source: "user", Payload: map[string]any{"title": title}}
	}
	good := writePackage(t, dir, "good.zip", s, []store.EvidenceEvent{ev(1, "a"), ev(2, "b")}, at)

	runCmd := func(args ...string) (int, string) {
		var out, errOut bytes.Buffer
		code := run(t.Context(), append([]string{"evidence"}, args...), func(string) string { return "" }, &out, &errOut)
		return code, out.String() + errOut.String()
	}
	if code, out := runCmd("verify", good); code != 0 || !strings.Contains(out, "result:      OK") || !strings.Contains(out, s.Fingerprint()) {
		t.Fatalf("good: %d %s", code, out)
	}
	if code, out := runCmd("verify", "--key", s.Fingerprint(), good); code != 0 {
		t.Fatalf("trusted key: %d %s", code, out)
	}
	other := make([]byte, 32)
	_, _ = rand.Read(other)
	o, _ := evidence.ParseSigningKey(base64.StdEncoding.EncodeToString(other))
	if code, _ := runCmd("verify", "--key", o.Fingerprint(), good); code != 1 {
		t.Fatal("wrong trusted key must fail")
	}

	// Tampered report.html: the file hash no longer matches.
	b, _ := os.ReadFile(good)
	zr, _ := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		if f.Name == evidence.ReportFile {
			data = bytes.ReplaceAll(data, []byte("Acme"), []byte("Acmf"))
		}
		w, _ := zw.Create(f.Name)
		_, _ = w.Write(data)
	}
	_ = zw.Close()
	bad := filepath.Join(dir, "bad.zip")
	_ = os.WriteFile(bad, out.Bytes(), 0o600)
	if code, txt := runCmd("verify", bad); code != 1 || !strings.Contains(txt, "report.html was changed") || !strings.Contains(txt, "INVALID") {
		t.Fatalf("tampered: %d %s", code, txt)
	}

	// A later export where event 1 was edited and event 2 deleted.
	later := writePackage(t, dir, "later.zip", s, []store.EvidenceEvent{ev(1, "EDITED")}, at.Add(time.Hour))
	if code, txt := runCmd("verify", "--against", good, later); code != 1 || !strings.Contains(txt, "event 1 was changed") ||
		!strings.Contains(txt, "event 2 is missing") {
		t.Fatalf("against: %d %s", code, txt)
	}
	same := writePackage(t, dir, "same.zip", s, []store.EvidenceEvent{ev(1, "a"), ev(2, "b"), ev(3, "c")}, at.Add(time.Hour))
	if code, txt := runCmd("verify", "--against", good, same); code != 0 || !strings.Contains(txt, "edits and deletions checked") {
		t.Fatalf("unchanged history: %d %s", code, txt)
	}

	if code, _ := runCmd("verify", filepath.Join(dir, "missing.zip")); code != 2 {
		t.Fatal("unreadable file: exit 2")
	}
	notZip := filepath.Join(dir, "x.zip")
	_ = os.WriteFile(notZip, []byte("not a zip"), 0o600)
	if code, _ := runCmd("verify", notZip); code != 1 {
		t.Fatal("not a zip: exit 1")
	}
	if code, _ := runCmd("verify"); code != 2 {
		t.Fatal("usage: exit 2")
	}
	if code, _ := runCmd(); code != 2 {
		t.Fatal("no subcommand: exit 2")
	}
}
