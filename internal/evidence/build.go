package evidence

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/store"
)

// File names inside the package.
const (
	ManifestFile  = "manifest.json"
	SignatureFile = "manifest.sig"
	ReportFile    = "report.html"
	EventsFile    = "data/events.json"
	EventsCSVFile = "data/events.csv"
)

// Kinds of export.
const (
	KindItem  = "item"
	KindScope = "scope"
)

// MaxPackageBytes bounds the uncompressed content of one package.
const MaxPackageBytes = 100 << 20

// ErrTooLarge means the package would exceed MaxPackageBytes.
var ErrTooLarge = errors.New("the evidence package would exceed 100 MB; choose fewer projects or a shorter date range")

// Params are the export parameters as recorded in the manifest.
type Params struct {
	ItemKey       string   `json:"item_key,omitempty"`
	ProjectKeys   []string `json:"project_keys,omitempty"` // scope: empty = every project the exporter can see
	From          string   `json:"from,omitempty"`
	To            string   `json:"to,omitempty"`
	ApprovalsOnly bool     `json:"approvals_only"`
	Language      string   `json:"language"`
}

// Person identifies the exporter.
type Person struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	Role        string    `json:"workspace_role"`
}

// FileEntry is one hashed file.
type FileEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Counts summarize the content.
type Counts struct {
	Projects    int `json:"projects"`
	Items       int `json:"items"`
	Events      int `json:"events"`
	Comments    int `json:"comments"`
	Approvals   int `json:"approvals"`
	Links       int `json:"links"`
	Attachments int `json:"attachments"`
	People      int `json:"people"`
}

// WorkspaceRef names the workspace.
type WorkspaceRef struct {
	ID       uuid.UUID `json:"id"`
	Slug     string    `json:"slug"`
	Name     string    `json:"name"`
	Timezone string    `json:"timezone"`
}

// Manifest is manifest.json. Everything an auditor needs to know about how
// the package was made, and the hash of every other file.
type Manifest struct {
	Format        string         `json:"format"`
	ExportID      uuid.UUID      `json:"export_id"`
	Kind          string         `json:"kind"`
	Parameters    Params         `json:"parameters"`
	Workspace     WorkspaceRef   `json:"workspace"`
	ExportedBy    Person         `json:"exported_by"`
	ExportedAt    time.Time      `json:"exported_at"`
	SnapshotAt    time.Time      `json:"snapshot_at"`
	RangeStart    *time.Time     `json:"range_start"` // inclusive; nil: the item's whole history
	RangeEnd      *time.Time     `json:"range_end"`   // exclusive
	ServerVersion string         `json:"server_version"`
	Counts        Counts         `json:"counts"`
	Events        EventRange     `json:"events"`
	SigningKey    SigningKeyInfo `json:"signing_key"`
	Files         []FileEntry    `json:"files"`
}

// EventRange records the first and last event id and a digest over every
// exported event digest in id order, so two packages can be compared.
type EventRange struct {
	FirstID int64  `json:"first_id"`
	LastID  int64  `json:"last_id"`
	Digest  string `json:"digest"`
}

// Input is everything Build needs.
type Input struct {
	ExportID      uuid.UUID
	Kind          string
	Params        Params
	Exporter      Person
	ExportedAt    time.Time
	ServerVersion string
	Data          *store.EvidenceSet
}

// Result describes a built package.
type Result struct {
	Manifest       Manifest
	ManifestSHA256 string
	Bytes          int64 // compressed size
}

// exportedEvent is an event as written to events.json: the stored fields
// plus its digest (see EventDigest).
type exportedEvent struct {
	store.EvidenceEvent
	Digest string `json:"digest"`
}

// Build writes a signed package to w.
func Build(w io.Writer, in Input, s *Signer) (Result, error) {
	if s == nil {
		return Result{}, errors.New("evidence signing key is not configured")
	}
	d := in.Data
	events := make([]exportedEvent, len(d.Events))
	chain := sha256.New()
	for i, e := range d.Events {
		raw, err := json.Marshal(e)
		if err != nil {
			return Result{}, err
		}
		digest, err := EventDigest(raw)
		if err != nil {
			return Result{}, err
		}
		events[i] = exportedEvent{EvidenceEvent: e, Digest: digest}
		chain.Write([]byte(digest))
	}
	m := Manifest{
		Format: Format, ExportID: in.ExportID, Kind: in.Kind, Parameters: in.Params,
		Workspace:  WorkspaceRef{ID: d.Workspace.ID, Slug: d.Workspace.Slug, Name: d.Workspace.Name, Timezone: d.Workspace.Timezone},
		ExportedBy: in.Exporter, ExportedAt: in.ExportedAt.UTC(), SnapshotAt: d.AsOf.UTC(),
		RangeStart: utcPtr(d.RangeStart), RangeEnd: utcPtr(d.RangeEnd), ServerVersion: in.ServerVersion,
		Counts: Counts{
			Projects: len(d.Projects), Items: len(d.Items), Events: len(d.Events), Comments: len(d.Comments),
			Approvals: len(d.Approvals), Links: len(d.Links), Attachments: len(d.Attachments), People: len(d.People),
		},
		Events:     EventRange{Digest: hex.EncodeToString(chain.Sum(nil))},
		SigningKey: s.Info(),
	}
	if n := len(d.Events); n > 0 {
		m.Events.FirstID, m.Events.LastID = d.Events[0].ID, d.Events[n-1].ID
	}

	report, err := renderReport(m, d)
	if err != nil {
		return Result{}, err
	}
	csvBytes, err := eventsCSV(d)
	if err != nil {
		return Result{}, err
	}
	files := []struct {
		path string
		data func() ([]byte, error)
	}{
		{ReportFile, func() ([]byte, error) { return report, nil }},
		{"README.txt", func() ([]byte, error) { return []byte(readme(m)), nil }},
		{"data/items.json", jsonFile(d.Items)},
		{EventsFile, jsonFile(events)},
		{EventsCSVFile, func() ([]byte, error) { return csvBytes, nil }},
		{"data/approvals.json", jsonFile(d.Approvals)},
		{"data/comments.json", jsonFile(d.Comments)},
		{"data/links.json", jsonFile(d.Links)},
		{"data/attachments.json", jsonFile(d.Attachments)},
		{"data/people.json", jsonFile(d.People)},
		{"data/projects.json", jsonFile(d.Projects)},
		{"data/statuses.json", jsonFile(d.Statuses)},
	}

	cw := &countingWriter{w: w}
	zw := zip.NewWriter(cw)
	var total int64
	put := func(name string, b []byte) error {
		total += int64(len(b))
		if total > MaxPackageBytes {
			return ErrTooLarge
		}
		// A fixed timestamp: the same content always produces the same bytes.
		f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: m.ExportedAt})
		if err != nil {
			return err
		}
		_, err = f.Write(b)
		return err
	}
	for _, f := range files {
		b, err := f.data()
		if err != nil {
			return Result{}, err
		}
		sum := sha256.Sum256(b)
		m.Files = append(m.Files, FileEntry{Path: f.path, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(b))})
		if err := put(f.path, b); err != nil {
			return Result{}, err
		}
	}
	manifest, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Result{}, err
	}
	manifest = append(manifest, '\n')
	sig, err := json.MarshalIndent(Signature{
		Algorithm: "Ed25519", KeyFingerprint: s.Fingerprint(),
		Signature: base64.StdEncoding.EncodeToString(s.sign(manifest)),
	}, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := put(ManifestFile, manifest); err != nil {
		return Result{}, err
	}
	if err := put(SignatureFile, append(sig, '\n')); err != nil {
		return Result{}, err
	}
	if err := zw.Close(); err != nil {
		return Result{}, err
	}
	sum := sha256.Sum256(manifest)
	return Result{Manifest: m, ManifestSHA256: hex.EncodeToString(sum[:]), Bytes: cw.n}, nil
}

func jsonFile(v any) func() ([]byte, error) {
	return func() ([]byte, error) {
		b, err := json.MarshalIndent(v, "", "  ")
		return append(b, '\n'), err
	}
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// canonicalEvent is the part of an event that can never legitimately change:
// names and keys (which people and projects may rename) are left out.
type canonicalEvent struct {
	ID          int64           `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	OccurredAt  string          `json:"occurred_at"`
	Action      string          `json:"action"`
	EntityType  string          `json:"entity_type"`
	EntityID    string          `json:"entity_id"`
	ActorID     *string         `json:"actor_id"`
	Source      string          `json:"source"`
	RuleID      *string         `json:"automation_rule_id"`
	Payload     json.RawMessage `json:"payload"`
}

// EventDigest is the hex SHA-256 of an event's canonical JSON: fixed field
// order, UTC nanosecond timestamp, payload keys sorted and numbers kept as
// written. Build and Verify compute it the same way from events.json, so an
// event edited in the database shows up as a different digest in a later
// export of the same event id.
func EventDigest(raw []byte) (string, error) {
	var in struct {
		ID          int64           `json:"id"`
		WorkspaceID string          `json:"workspace_id"`
		OccurredAt  time.Time       `json:"occurred_at"`
		Action      string          `json:"action"`
		EntityType  string          `json:"entity_type"`
		EntityID    string          `json:"entity_id"`
		ActorID     *string         `json:"actor_id"`
		Source      string          `json:"source"`
		RuleID      *string         `json:"automation_rule_id"`
		Payload     json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("event: %w", err)
	}
	payload, err := canonicalJSON(in.Payload)
	if err != nil {
		return "", fmt.Errorf("event %d payload: %w", in.ID, err)
	}
	b, err := json.Marshal(canonicalEvent{
		ID: in.ID, WorkspaceID: in.WorkspaceID, OccurredAt: in.OccurredAt.UTC().Format(time.RFC3339Nano),
		Action: in.Action, EntityType: in.EntityType, EntityID: in.EntityID, ActorID: in.ActorID,
		Source: in.Source, RuleID: in.RuleID, Payload: payload,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// canonicalJSON re-encodes JSON with sorted object keys and numbers as written.
func canonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage("null"), nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return json.Marshal(v) // map keys are sorted
}

// eventsCSV is one row per event, for spreadsheets. Cells that a spreadsheet
// would run as a formula are prefixed with an apostrophe (CSV injection).
func eventsCSV(d *store.EvidenceSet) ([]byte, error) {
	names := map[uuid.UUID]store.EvidencePerson{}
	for _, p := range d.People {
		names[p.ID] = p
	}
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF") // UTF-8 BOM: Excel otherwise misreads Azerbaijani and Russian letters
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"id", "occurred_at_utc", "action", "entity_type", "entity_id", "item_key",
		"actor_id", "actor_name", "actor_email", "source", "automation_rule_id", "payload"})
	for _, e := range d.Events {
		var actorID, actorName, actorEmail, rule string
		if e.ActorID != nil {
			actorID = e.ActorID.String()
			if p, ok := names[*e.ActorID]; ok {
				actorName, actorEmail = p.DisplayName, p.Email
			}
		}
		if e.RuleID != nil {
			rule = e.RuleID.String()
		}
		payload, err := json.Marshal(e.Payload)
		if err != nil {
			return nil, err
		}
		row := []string{fmt.Sprint(e.ID), e.OccurredAt.UTC().Format(time.RFC3339Nano), e.Action, e.EntityType,
			e.EntityID.String(), e.ItemKey, actorID, actorName, actorEmail, e.Source, rule, string(payload)}
		for i := range row {
			row[i] = csvSafe(row[i])
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
