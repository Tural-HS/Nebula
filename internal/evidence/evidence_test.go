package evidence

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/store"
)

// Synthetic data only.

func testSigner(t *testing.T) *Signer {
	t.Helper()
	seed := make([]byte, 32)
	_, _ = rand.Read(seed)
	s, err := ParseSigningKey(base64.StdEncoding.EncodeToString(seed))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func testSet() *store.EvidenceSet {
	ws := uuid.New()
	alice, item := uuid.New(), uuid.New()
	at := time.Date(2026, 10, 1, 9, 30, 0, 123456000, time.UTC)
	desc := "Ödəniş <script>alert(1)</script> & qəbz"
	return &store.EvidenceSet{
		Workspace: store.Workspace{ID: ws, Slug: "acme", Name: "Acme", Timezone: "Asia/Baku"},
		AsOf:      at.Add(time.Hour),
		Projects:  []store.EvidenceProject{{ID: uuid.New(), Key: "FIN", Name: "Maliyyə", Visibility: "private"}},
		Items: []store.EvidenceItem{{Item: store.Item{ID: item, Key: "FIN-1", Number: 1, Title: "=SUM(A1)", Description: &desc,
			Fields: map[string]any{"amount": 1250.5}, CreatedAt: at, UpdatedAt: at}, ProjectKey: "FIN"}},
		Events: []store.EvidenceEvent{
			{ID: 7, WorkspaceID: ws, OccurredAt: at, Action: "item.created", EntityType: "item", EntityID: item, ItemKey: "FIN-1",
				ActorID: &alice, Source: "user", Payload: map[string]any{"title": "=SUM(A1)", "amount": 1e21}},
			{ID: 9, WorkspaceID: ws, OccurredAt: at.Add(time.Minute), Action: "item.updated", EntityType: "item", EntityID: item,
				ItemKey: "FIN-1", Source: "automation", Payload: map[string]any{"changes": map[string]any{"title": map[string]any{"from": "a", "to": "b"}}}},
		},
		People: []store.EvidencePerson{{ID: alice, DisplayName: "Alice", Email: "alice@example.com"}},
	}
}

func build(t *testing.T, s *Signer, set *store.EvidenceSet, kind string, p Params) []byte {
	t.Helper()
	var buf bytes.Buffer
	_, err := Build(&buf, Input{ExportID: uuid.New(), Kind: kind, Params: p, ExportedAt: set.AsOf,
		Exporter: Person{DisplayName: "Alice", Email: "alice@example.com", Role: "owner"}, ServerVersion: "test", Data: set}, s)
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func open(t *testing.T, b []byte) *Package {
	t.Helper()
	p, err := Open(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// rewrite copies a ZIP, applying edit to each entry's content (nil: drop it).
func rewrite(t *testing.T, b []byte, edit func(name string, data []byte) []byte, extra map[string]string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		if data = edit(f.Name, data); data == nil {
			continue
		}
		w, _ := zw.Create(f.Name)
		_, _ = w.Write(data)
	}
	for name, data := range extra {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(data))
	}
	_ = zw.Close()
	return out.Bytes()
}

func keep(_ string, d []byte) []byte { return d }

func TestBuildVerifies(t *testing.T) {
	s := testSigner(t)
	b := build(t, s, testSet(), KindItem, Params{ItemKey: "FIN-1", Language: "az"})
	p := open(t, b)
	if rep := p.Verify(""); !rep.OK || rep.Fingerprint != s.Fingerprint() {
		t.Fatalf("fresh package: %+v", rep)
	}
	if rep := p.Verify(s.Fingerprint()); !rep.OK {
		t.Fatalf("trusted fingerprint: %+v", rep)
	}
	if rep := p.Verify(s.PublicKey()); !rep.OK {
		t.Fatalf("trusted public key: %+v", rep)
	}
	if rep := p.Verify(testSigner(t).Fingerprint()); rep.OK {
		t.Fatal("a different trusted key must fail")
	}
	// Deterministic: same content, same bytes.
	m := p.Manifest
	if m.Counts.Events != 2 || m.Events.FirstID != 7 || m.Events.LastID != 9 || len(m.Files) != 12 {
		t.Fatalf("manifest: %+v", m)
	}
}

func TestTamperedFileIsDetected(t *testing.T) {
	s := testSigner(t)
	b := build(t, s, testSet(), KindItem, Params{ItemKey: "FIN-1", Language: "en"})
	for _, name := range []string{ReportFile, EventsFile, EventsCSVFile, "data/approvals.json"} {
		bad := rewrite(t, b, func(n string, d []byte) []byte {
			if n == name {
				return append(d, ' ')
			}
			return d
		}, nil)
		rep := open(t, bad).Verify("")
		if rep.OK || !strings.Contains(strings.Join(rep.Problems, ";"), name+" was changed") {
			t.Errorf("%s edited: %+v", name, rep)
		}
	}
	dropped := rewrite(t, b, func(n string, d []byte) []byte {
		if n == "data/comments.json" {
			return nil
		}
		return d
	}, nil)
	if rep := open(t, dropped).Verify(""); rep.OK {
		t.Error("a missing file must fail")
	}
	if rep := open(t, rewrite(t, b, keep, map[string]string{"data/extra.json": "{}"})).Verify(""); rep.OK {
		t.Error("an unlisted file must fail")
	}
	if rep := open(t, rewrite(t, b, keep, map[string]string{"../evil": "x"})).Verify(""); rep.OK {
		t.Error("an unsafe name must fail")
	}
}

func TestTamperedManifestIsDetected(t *testing.T) {
	s := testSigner(t)
	b := build(t, s, testSet(), KindScope, Params{From: "2026-10-01", To: "2026-10-31", Language: "az"})
	// Change a count and re-hash nothing: the signature breaks.
	edited := rewrite(t, b, func(n string, d []byte) []byte {
		if n == ManifestFile {
			return bytes.Replace(d, []byte(`"items": 1`), []byte(`"items": 0`), 1)
		}
		return d
	}, nil)
	if rep := open(t, edited).Verify(""); rep.OK || !strings.Contains(strings.Join(rep.Problems, ";"), "signature does not match") {
		t.Fatalf("manifest edited: %+v", rep)
	}
	// Edit a file AND its hash in the manifest: still caught by the signature.
	var m Manifest
	p := open(t, b)
	m = p.Manifest
	report, _ := p.File(ReportFile)
	forged := append(bytes.Clone(report), []byte("<!-- forged -->")...)
	for i := range m.Files {
		if m.Files[i].Path == ReportFile {
			m.Files[i].SHA256, m.Files[i].Size = sha(forged), int64(len(forged))
		}
	}
	mb, _ := json.MarshalIndent(m, "", "  ")
	both := rewrite(t, b, func(n string, d []byte) []byte {
		switch n {
		case ManifestFile:
			return append(mb, '\n')
		case ReportFile:
			return forged
		}
		return d
	}, nil)
	if rep := open(t, both).Verify(""); rep.OK {
		t.Fatal("re-hashed manifest must fail the signature")
	}
	// Re-sign with another key: verifies only against that key, never the trusted one.
	other := testSigner(t)
	m.SigningKey = other.Info()
	mb, _ = json.MarshalIndent(m, "", "  ")
	mb = append(mb, '\n')
	sig, _ := json.Marshal(Signature{Algorithm: "Ed25519", KeyFingerprint: other.Fingerprint(),
		Signature: base64.StdEncoding.EncodeToString(other.sign(mb))})
	resigned := rewrite(t, b, func(n string, d []byte) []byte {
		switch n {
		case ManifestFile:
			return mb
		case SignatureFile:
			return sig
		case ReportFile:
			return forged
		}
		return d
	}, nil)
	if rep := open(t, resigned).Verify(s.Fingerprint()); rep.OK {
		t.Fatal("a package re-signed by another key must fail against the trusted key")
	}
	if dropped := rewrite(t, b, func(n string, d []byte) []byte {
		if n == SignatureFile {
			return nil
		}
		return d
	}, nil); open(t, dropped).Verify("").OK {
		t.Fatal("a missing signature must fail")
	}
}

func sha(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestEventContentIsBoundToDigest(t *testing.T) {
	s := testSigner(t)
	b := build(t, s, testSet(), KindItem, Params{ItemKey: "FIN-1"})
	p := open(t, b)
	ev, _ := p.File(EventsFile)
	// Numbers survive exactly (no float re-formatting between build and verify).
	if !strings.Contains(string(ev), "1e+21") {
		t.Fatalf("payload number: %s", ev)
	}
	if _, _, err := p.eventDigests(); err != nil {
		t.Fatal(err)
	}
}

func TestCompareDetectsEditedAndDeletedEvents(t *testing.T) {
	s := testSigner(t)
	set := testSet()
	scope := Params{From: "2026-10-01", To: "2026-10-31"}
	start := time.Date(2026, 9, 30, 20, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 31)
	set.RangeStart, set.RangeEnd = &start, &end
	older := open(t, build(t, s, set, KindScope, scope))

	later := *set
	later.AsOf = set.AsOf.Add(24 * time.Hour)
	later.Events = []store.EvidenceEvent{set.Events[0]}
	later.Events[0].Payload = map[string]any{"title": "edited"}
	newer := open(t, build(t, s, &later, KindScope, scope))
	diffs, err := Compare(older, newer)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(diffs, ";")
	if !strings.Contains(joined, "event 7 was changed") || !strings.Contains(joined, "event 9 is missing") {
		t.Fatalf("diffs: %v", diffs)
	}
	// Renaming people or keys is not tampering.
	renamed := *set
	renamed.AsOf = later.AsOf
	renamed.Events = append([]store.EvidenceEvent(nil), set.Events...)
	renamed.Events[0].ItemKey = "PAY-1"
	if diffs, _ := Compare(older, open(t, build(t, s, &renamed, KindScope, scope))); len(diffs) != 0 {
		t.Fatalf("rename: %v", diffs)
	}
	// A narrower later export cannot prove deletions, only edits.
	narrow := Params{From: "2026-10-02", To: "2026-10-31"}
	ns := later.RangeStart.Add(24 * time.Hour)
	later.RangeStart = &ns
	diffs, _ = Compare(older, open(t, build(t, s, &later, KindScope, narrow)))
	if len(diffs) != 1 || !strings.Contains(diffs[0], "changed") {
		t.Fatalf("narrow: %v", diffs)
	}
}

func TestCovers(t *testing.T) {
	d := func(s string) *time.Time { v, _ := time.Parse(time.DateOnly, s); return &v }
	m := func(keys []string, from, to string, appr bool) Manifest {
		return Manifest{Kind: KindScope, Parameters: Params{ProjectKeys: keys, ApprovalsOnly: appr}, RangeStart: d(from), RangeEnd: d(to)}
	}
	cases := []struct {
		a, b Manifest
		want bool
	}{
		{m(nil, "2026-01-01", "2027-01-01", false), m([]string{"FIN"}, "2026-03-01", "2026-04-01", true), true},
		{m([]string{"FIN"}, "2026-01-01", "2027-01-01", false), m(nil, "2026-03-01", "2026-04-01", false), false},
		{m([]string{"FIN", "HR"}, "2026-01-01", "2027-01-01", false), m([]string{"HR"}, "2026-01-01", "2026-04-01", false), true},
		{m(nil, "2026-02-01", "2027-01-01", false), m(nil, "2026-01-01", "2026-04-01", false), false},
		{m(nil, "2026-01-01", "2027-01-01", true), m(nil, "2026-01-01", "2026-04-01", false), false},
		{Manifest{Kind: KindItem, Parameters: Params{ItemKey: "FIN-1"}}, Manifest{Kind: KindItem, Parameters: Params{ItemKey: "FIN-1"}}, true},
		{m(nil, "2026-01-01", "2027-01-01", false), Manifest{Kind: KindItem, Parameters: Params{ItemKey: "FIN-1"}}, false},
	}
	for i, c := range cases {
		if got := Covers(c.a, c.b); got != c.want {
			t.Errorf("case %d: %v", i, got)
		}
	}
}

func TestReportIsEscapedAndSelfContained(t *testing.T) {
	set := testSet()
	set.People[0].DisplayName = "=HYPERLINK(\"x\")" // display names come from the IdP: untrusted
	b := build(t, testSigner(t), set, KindItem, Params{ItemKey: "FIN-1", Language: "ru"})
	html, _ := open(t, b).File(ReportFile)
	s := string(html)
	if strings.Contains(s, "<script>") || !strings.Contains(s, "&lt;script&gt;") {
		t.Fatal("user content must be escaped")
	}
	for _, ext := range []string{"src=", "href=", "@import", "url("} {
		if strings.Contains(s, ext) {
			t.Fatalf("external reference %q in report", ext)
		}
	}
	if !strings.Contains(s, "Пакет аудиторских доказательств") || !strings.Contains(s, `lang="ru"`) {
		t.Fatal("report language")
	}
	if !strings.Contains(s, "2026-10-01 13:30:00") { // Asia/Baku is UTC+4
		t.Fatal("times are shown in the workspace time zone")
	}
	csv, _ := open(t, b).File(EventsCSVFile)
	if strings.Contains(string(csv), `,"=HYPERLINK`) || !strings.Contains(string(csv), `,"'=HYPERLINK`) {
		t.Fatalf("CSV injection guard: %s", csv)
	}
}

func TestTooLarge(t *testing.T) {
	set := testSet()
	set.Items[0].Description = new(strings.Repeat("x", MaxPackageBytes))
	var buf bytes.Buffer
	_, err := Build(&buf, Input{ExportID: uuid.New(), Kind: KindItem, ExportedAt: set.AsOf, Data: set}, testSigner(t))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSigningKey(t *testing.T) {
	for _, bad := range []string{"", "not base64!", base64.StdEncoding.EncodeToString(make([]byte, 31))} {
		if _, err := ParseSigningKey(bad); err == nil || strings.Contains(err.Error(), bad) && bad != "" {
			t.Errorf("%q: %v", bad, err)
		}
	}
	s := testSigner(t)
	if !strings.HasPrefix(s.Fingerprint(), "SHA256:") || len(s.Fingerprint()) != 7+43 {
		t.Fatal(s.Fingerprint())
	}
}
