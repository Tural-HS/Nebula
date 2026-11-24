package evidence

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
	"time"
)

// Bounds for reading an untrusted package (zip bombs, huge manifests).
const (
	maxEntries      = 64
	maxEntryBytes   = MaxPackageBytes
	maxTotalBytes   = MaxPackageBytes + 1<<20
	maxManifestSize = 4 << 20
)

// Package is an opened evidence ZIP: every entry read into memory.
type Package struct {
	files    map[string][]byte
	order    []string
	Manifest Manifest
	// problems found while opening (bad names, duplicates, unreadable manifest).
	problems []string
}

// Open reads a package. A structurally broken ZIP is an error; anything
// else wrong is reported by Verify.
func Open(r io.ReaderAt, size int64) (*Package, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("not a ZIP archive: %w", err)
	}
	if len(zr.File) > maxEntries {
		return nil, fmt.Errorf("too many files in the archive (%d)", len(zr.File))
	}
	p := &Package{files: map[string][]byte{}}
	var total int64
	for _, f := range zr.File {
		name := f.Name
		if !safeName(name) {
			p.problems = append(p.problems, fmt.Sprintf("unsafe file name %q", name))
			continue
		}
		if _, dup := p.files[name]; dup {
			p.problems = append(p.problems, fmt.Sprintf("duplicate file %q", name))
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxEntryBytes+1))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if int64(len(b)) > maxEntryBytes {
			return nil, fmt.Errorf("%s is larger than %d bytes", name, maxEntryBytes)
		}
		if total += int64(len(b)); total > maxTotalBytes {
			return nil, errors.New("the archive is larger than allowed")
		}
		p.files[name] = b
		p.order = append(p.order, name)
	}
	raw, ok := p.files[ManifestFile]
	switch {
	case !ok:
		p.problems = append(p.problems, "manifest.json is missing")
	case len(raw) > maxManifestSize:
		p.problems = append(p.problems, "manifest.json is too large")
	default:
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p.Manifest); err != nil {
			p.problems = append(p.problems, "manifest.json is not valid: "+err.Error())
		}
	}
	return p, nil
}

func safeName(name string) bool {
	return name != "" && !strings.HasPrefix(name, "/") && !strings.Contains(name, `\`) &&
		path.Clean(name) == strings.TrimSuffix(name, "/") && !strings.HasPrefix(name, "../") && name != ".."
}

// File returns an entry's bytes.
func (p *Package) File(name string) ([]byte, bool) {
	b, ok := p.files[name]
	return b, ok
}

// Report is the outcome of Verify.
type Report struct {
	OK          bool
	Fingerprint string   // the key that signed the manifest
	Problems    []string // empty when OK
}

// Verify checks the signature over manifest.json and every file hash. With
// trustedKey (a base64 public key or a "SHA256:…" fingerprint) the signing
// key must also be that key; without it the signature only proves the
// package is unchanged since it was signed by the key it names, so the
// fingerprint must be compared by hand.
func (p *Package) Verify(trustedKey string) Report {
	rep := Report{Problems: slices.Clone(p.problems)}
	bad := func(format string, args ...any) { rep.Problems = append(rep.Problems, fmt.Sprintf(format, args...)) }
	m := p.Manifest
	manifest, haveManifest := p.files[ManifestFile]
	if haveManifest && m.Format != Format {
		bad("unknown format %q (want %s)", m.Format, Format)
	}

	pub, err := base64.StdEncoding.DecodeString(m.SigningKey.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		bad("manifest.json has no valid signing key")
		pub = nil
	} else {
		rep.Fingerprint = Fingerprint(pub)
		if m.SigningKey.Fingerprint != rep.Fingerprint {
			bad("the manifest's key fingerprint does not match its public key")
		}
	}
	var sig Signature
	rawSig, ok := p.files[SignatureFile]
	switch {
	case !ok:
		bad("manifest.sig is missing")
	case json.Unmarshal(rawSig, &sig) != nil:
		bad("manifest.sig is not valid JSON")
	default:
		s, err := base64.StdEncoding.DecodeString(sig.Signature)
		switch {
		case sig.Algorithm != "Ed25519":
			bad("unsupported signature algorithm %q", sig.Algorithm)
		case err != nil:
			bad("manifest.sig: the signature is not base64")
		case sig.KeyFingerprint != rep.Fingerprint:
			bad("manifest.sig names a different key (%s)", sig.KeyFingerprint)
		case pub == nil || !haveManifest || !verifySignature(pub, manifest, s):
			bad("the signature does not match manifest.json: the manifest was changed or not signed by this key")
		}
	}
	if trustedKey != "" && pub != nil {
		want := strings.TrimSpace(trustedKey)
		if k, err := base64.StdEncoding.DecodeString(want); err == nil && len(k) == ed25519.PublicKeySize {
			want = Fingerprint(k)
		}
		if want != rep.Fingerprint {
			bad("signed by %s, not by the trusted key %s", rep.Fingerprint, want)
		}
	}

	listed := map[string]bool{ManifestFile: true, SignatureFile: true}
	for _, f := range m.Files {
		if listed[f.Path] {
			bad("%s is listed twice", f.Path)
			continue
		}
		listed[f.Path] = true
		b, ok := p.files[f.Path]
		if !ok {
			bad("%s is missing", f.Path)
			continue
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != f.SHA256 || int64(len(b)) != f.Size {
			bad("%s was changed (sha256 %s, manifest says %s)", f.Path, got, f.SHA256)
		}
	}
	for _, name := range p.order {
		if !listed[name] {
			bad("%s is not listed in the manifest", name)
		}
	}
	if len(rep.Problems) == 0 {
		if err := p.checkEvents(); err != nil {
			bad("%v", err)
		}
	}
	rep.OK = len(rep.Problems) == 0
	return rep
}

// eventDigests returns the digest of every event in events.json, by id,
// recomputed from the event fields (not the stored "digest").
func (p *Package) eventDigests() (map[int64]string, []int64, error) {
	raw, ok := p.files[EventsFile]
	if !ok {
		return nil, nil, errors.New(EventsFile + " is missing")
	}
	var events []json.RawMessage
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", EventsFile, err)
	}
	out := make(map[int64]string, len(events))
	ids := make([]int64, 0, len(events))
	for _, e := range events {
		var head struct {
			ID     int64  `json:"id"`
			Digest string `json:"digest"`
		}
		if err := json.Unmarshal(e, &head); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", EventsFile, err)
		}
		d, err := EventDigest(e)
		if err != nil {
			return nil, nil, err
		}
		if d != head.Digest {
			return nil, nil, fmt.Errorf("event %d: digest does not match its content", head.ID)
		}
		out[head.ID] = d
		ids = append(ids, head.ID)
	}
	return out, ids, nil
}

// checkEvents: the per-event digests and the manifest's events digest agree.
func (p *Package) checkEvents() error {
	digests, ids, err := p.eventDigests()
	if err != nil {
		return err
	}
	chain := sha256.New()
	for _, id := range ids {
		chain.Write([]byte(digests[id]))
	}
	if hex.EncodeToString(chain.Sum(nil)) != p.Manifest.Events.Digest || len(ids) != p.Manifest.Counts.Events {
		return errors.New("events.json does not match the manifest's events digest")
	}
	return nil
}

// Compare checks a newer package against an older one of the same
// workspace (both already verified). An event id present in both must have
// the same content: otherwise it was edited in between. When the newer
// package's scope covers the older one's, every older event must still be
// there: otherwise it was deleted. It returns the differences found.
func Compare(older, newer *Package) ([]string, error) {
	om, nm := older.Manifest, newer.Manifest
	if om.Workspace.ID != nm.Workspace.ID {
		return nil, errors.New("the packages are from different workspaces")
	}
	if nm.SnapshotAt.Before(om.SnapshotAt) {
		older, newer, om, nm = newer, older, nm, om
	}
	od, oids, err := older.eventDigests()
	if err != nil {
		return nil, err
	}
	nd, _, err := newer.eventDigests()
	if err != nil {
		return nil, err
	}
	covers := Covers(nm, om)
	var diffs []string
	for _, id := range oids {
		got, ok := nd[id]
		switch {
		case ok && got != od[id]:
			diffs = append(diffs, fmt.Sprintf("event %d was changed after %s", id, om.SnapshotAt.Format(time.RFC3339)))
		case !ok && covers:
			diffs = append(diffs, fmt.Sprintf("event %d is missing: deleted after %s", id, om.SnapshotAt.Format(time.RFC3339)))
		}
	}
	return diffs, nil
}

// Covers reports whether every event in package b must also be in package a:
// a's projects, range and filter include b's.
func Covers(a, b Manifest) bool {
	ap, bp := a.Parameters, b.Parameters
	if a.Kind == KindItem || b.Kind == KindItem {
		// An item export has no range: only another export of the same item
		// is known to contain all its events.
		return a.Kind == KindItem && b.Kind == KindItem && ap.ItemKey == bp.ItemKey
	}
	if ap.ApprovalsOnly && !bp.ApprovalsOnly {
		return false
	}
	if len(ap.ProjectKeys) > 0 {
		if len(bp.ProjectKeys) == 0 {
			return false
		}
		for _, k := range bp.ProjectKeys {
			if !slices.Contains(ap.ProjectKeys, k) {
				return false
			}
		}
	}
	if a.RangeStart == nil || b.RangeStart == nil || a.RangeEnd == nil || b.RangeEnd == nil {
		return false
	}
	return !a.RangeStart.After(*b.RangeStart) && !a.RangeEnd.Before(*b.RangeEnd)
}
