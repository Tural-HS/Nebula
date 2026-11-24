package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/Tural-HS/Nebula/internal/config"
	"github.com/Tural-HS/Nebula/internal/evidence"
)

const evidenceUsage = `usage:
  nebula evidence verify [--key <fingerprint|public-key>] [--against <older.zip>] <package.zip>
  nebula evidence public-key     print the configured signing key's fingerprint (ADR-0029)`

// evidenceCmd runs the evidence subcommands. verify needs no configuration,
// database or secrets: an auditor can run it on any machine.
// Exit codes: 0 valid, 1 invalid or tampered, 2 usage or unreadable input.
func evidenceCmd(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, evidenceUsage)
		return 2
	}
	switch args[0] {
	case "verify":
		return evidenceVerify(args[1:], stdout, stderr)
	case "public-key":
		cfg, err := config.Load(getenv)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "invalid configuration:\n%v\n", err)
			return 2
		}
		if cfg.EvidenceSigningKey == "" {
			_, _ = fmt.Fprintln(stderr, "NEBULA_EVIDENCE_SIGNING_KEY is not set")
			return 2
		}
		s, err := evidence.ParseSigningKey(cfg.EvidenceSigningKey)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		_, _ = fmt.Fprintf(stdout, "fingerprint: %s\npublic key:  %s\n", s.Fingerprint(), s.PublicKey())
		return 0
	default:
		_, _ = fmt.Fprintln(stderr, evidenceUsage)
		return 2
	}
}

func evidenceVerify(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	key := fs.String("key", "", "trusted signing key: fingerprint (SHA256:…) or base64 public key")
	against := fs.String("against", "", "an older package of the same workspace: detect events edited or deleted since")
	if err := fs.Parse(args); err != nil || fs.NArg() != 1 {
		_, _ = fmt.Fprintln(stderr, evidenceUsage)
		return 2
	}
	pkg, code := openPackage(fs.Arg(0), stderr)
	if pkg == nil {
		return code
	}
	m := pkg.Manifest
	rep := pkg.Verify(*key)
	_, _ = fmt.Fprintf(stdout, "package:     %s\nexport id:   %s\nworkspace:   %s (%s)\nexported:    %s by %s <%s>\nsigned by:   %s\n",
		fs.Arg(0), m.ExportID, m.Workspace.Name, m.Workspace.Slug, m.ExportedAt.Format("2006-01-02 15:04:05 MST"),
		m.ExportedBy.DisplayName, m.ExportedBy.Email, rep.Fingerprint)
	ok := rep.OK
	for _, p := range rep.Problems {
		_, _ = fmt.Fprintln(stdout, "FAIL: "+p)
	}
	if ok && *against != "" {
		older, code := openPackage(*against, stderr)
		if older == nil {
			return code
		}
		if orep := older.Verify(*key); !orep.OK {
			for _, p := range orep.Problems {
				_, _ = fmt.Fprintln(stdout, "FAIL (older package): "+p)
			}
			return 1
		}
		diffs, err := evidence.Compare(older, pkg)
		if err != nil {
			_, _ = fmt.Fprintln(stdout, "FAIL: "+err.Error())
			return 1
		}
		for _, d := range diffs {
			_, _ = fmt.Fprintln(stdout, "FAIL: "+d)
		}
		ok = len(diffs) == 0
		if ok {
			scope := "only edits are checked: the newer package's scope does not include the older one's"
			if evidence.Covers(pkg.Manifest, older.Manifest) || evidence.Covers(older.Manifest, pkg.Manifest) {
				scope = "edits and deletions checked"
			}
			_, _ = fmt.Fprintf(stdout, "compared:    %s (%s)\n", *against, scope)
		}
	}
	if !ok {
		_, _ = fmt.Fprintln(stdout, "result:      INVALID")
		return 1
	}
	if *key == "" {
		_, _ = fmt.Fprintln(stdout, "note:        compare the fingerprint above with Settings → Audit log, or pass --key")
	}
	_, _ = fmt.Fprintln(stdout, "result:      OK")
	return 0
}

func openPackage(path string, stderr io.Writer) (*evidence.Package, int) {
	f, err := os.Open(path) // #nosec G304 -- the operator names the file
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return nil, 2
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return nil, 2
	}
	pkg, err := evidence.Open(f, info.Size())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return nil, 1
	}
	return pkg, 0
}

// evidenceSigner loads the signing key. Without one, production disables
// exports (an unconfigured key must not silently produce packages nobody
// can verify later); development signs with a throwaway key.
func evidenceSigner(cfg config.Config, logger *slog.Logger) (*evidence.Signer, error) {
	if cfg.EvidenceSigningKey != "" {
		s, err := evidence.ParseSigningKey(cfg.EvidenceSigningKey)
		if err != nil {
			return nil, err
		}
		logger.Info("evidence signing key loaded", "fingerprint", s.Fingerprint())
		return s, nil
	}
	if cfg.Env == "production" {
		logger.Warn("NEBULA_EVIDENCE_SIGNING_KEY not set: audit evidence export is disabled")
		return nil, nil
	}
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	s, err := evidence.ParseSigningKey(base64.StdEncoding.EncodeToString(seed))
	if err != nil {
		return nil, err
	}
	logger.Warn("NEBULA_EVIDENCE_SIGNING_KEY not set: signing evidence with a throwaway development key",
		"fingerprint", s.Fingerprint())
	return s, nil
}
