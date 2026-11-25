package evidence

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/store"
)

//go:embed report.html.tmpl
var reportSource string

// The report is html/template: every value is escaped for its context, and
// the page has no scripts and loads nothing (CSP default-src 'none').
var reportTemplate = template.Must(template.New("report").Parse(reportSource))

type kv struct{ K, V string }

type viewEvent struct {
	ID                int64
	Time, Who, Action string
	Details           []string
}

type viewDecision struct{ Who, Verb, Time, Comment string }

type viewApproval struct {
	Requested, By, Transition, State, Required, Comment string
	Closed                                              string
	Decisions                                           []viewDecision
}

type viewComment struct {
	Who, Time, Body string
	Deleted, Edited bool
}

type viewAttachment struct{ Name, Size, SHA256, Uploader, Time string }

type viewItem struct {
	Key, Title, Project, Type, Status, Assignee, Reporter, Created, Updated, Due, Deleted, Description string
	Fields                                                                                             []kv
	Approvals                                                                                          []viewApproval
	Comments                                                                                           []viewComment
	Links                                                                                              []string
	Attachments                                                                                        []viewAttachment
	History                                                                                            []viewEvent
}

type reportView struct {
	L             labels
	Lang          string
	M             Manifest
	Header        []kv
	Counts        []kv
	Items         []viewItem
	ProjectEvents []viewEvent
}

// resolver turns ids into names for the report (the JSON keeps the ids).
type resolver struct {
	loc      *time.Location
	l        labels
	people   map[uuid.UUID]store.EvidencePerson
	statuses map[uuid.UUID]string
}

func (r resolver) time(t time.Time) string { return t.In(r.loc).Format("2006-01-02 15:04:05") }

func (r resolver) timePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return r.time(*t)
}

func (r resolver) person(id *uuid.UUID) string {
	if id == nil {
		return r.l.None
	}
	if p, ok := r.people[*id]; ok {
		return p.DisplayName + " <" + p.Email + ">"
	}
	return id.String()
}

func (r resolver) actor(id *uuid.UUID) string {
	if id == nil {
		return r.l.System
	}
	return r.person(id)
}

func (r resolver) status(id *uuid.UUID) string {
	if id == nil {
		return r.l.None
	}
	if n, ok := r.statuses[*id]; ok {
		return n
	}
	return id.String()
}

// value renders a payload value: ids of people and statuses get their names.
func (r resolver) value(v any) string {
	switch x := v.(type) {
	case nil:
		return r.l.None
	case string:
		if id, err := uuid.Parse(x); err == nil {
			if p, ok := r.people[id]; ok {
				return p.DisplayName
			}
			if n, ok := r.statuses[id]; ok {
				return n
			}
		}
		return clip(x, 300)
	case map[string]any:
		// A formula field that failed stores {"error": code} (ADR-0030).
		if code, ok := x["error"].(string); ok && len(x) == 1 {
			return "⚠ " + clip(code, 60)
		}
		b, _ := json.Marshal(x)
		return clip(string(b), 300)
	default:
		b, _ := json.Marshal(x)
		return clip(string(b), 300)
	}
}

// details summarizes an event: "field: from → to" for changes, otherwise
// the other payload keys (the full payload is in events.json).
func (r resolver) details(e store.EvidenceEvent) []string {
	var out []string
	if changes, ok := e.Payload["changes"].(map[string]any); ok {
		for _, k := range sortedKeys(changes) {
			if c, ok := changes[k].(map[string]any); ok {
				out = append(out, fmt.Sprintf("%s: %s → %s", k, r.value(c["from"]), r.value(c["to"])))
			} else {
				out = append(out, k+": "+r.value(changes[k]))
			}
		}
	}
	for _, k := range sortedKeys(e.Payload) {
		if k == "changes" || k == "snapshot" {
			continue
		}
		out = append(out, k+": "+r.value(e.Payload[k]))
	}
	return out
}

func (r resolver) event(e store.EvidenceEvent) viewEvent {
	return viewEvent{ID: e.ID, Time: r.time(e.OccurredAt), Who: r.actor(e.ActorID), Action: e.Action, Details: r.details(e)}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func renderReport(m Manifest, d *store.EvidenceSet) ([]byte, error) {
	lang := NormalizeLanguage(m.Parameters.Language)
	l := allLabels[lang]
	loc, err := time.LoadLocation(d.Workspace.Timezone)
	if err != nil {
		loc = time.UTC
	}
	r := resolver{loc: loc, l: l, people: map[uuid.UUID]store.EvidencePerson{}, statuses: map[uuid.UUID]string{}}
	for _, p := range d.People {
		r.people[p.ID] = p
	}
	for _, s := range d.Statuses {
		r.statuses[s.ID] = s.Name
	}

	v := reportView{L: l, Lang: lang, M: m}
	kind := l.KindScope
	if m.Kind == KindItem {
		kind = l.KindItem + " " + m.Parameters.ItemKey
	}
	projects := l.AllProjects
	if len(m.Parameters.ProjectKeys) > 0 || m.Kind == KindItem {
		keys := make([]string, len(d.Projects))
		for i, p := range d.Projects {
			keys[i] = p.Key + " (" + p.Name + ")"
		}
		projects = strings.Join(keys, ", ")
	}
	period := l.WholeHistory
	if m.Parameters.From != "" {
		period = m.Parameters.From + " … " + m.Parameters.To
	}
	yesNo := map[bool]string{true: l.Yes, false: l.No}
	v.Header = []kv{
		{l.Workspace, d.Workspace.Name + " (" + d.Workspace.Slug + ")"},
		{l.Kind, kind},
		{l.Projects, projects},
		{l.Range, period},
		{l.ApprovalsOnly, yesNo[m.Parameters.ApprovalsOnly]},
		{l.ExportedBy, m.ExportedBy.DisplayName + " <" + m.ExportedBy.Email + "> (" + m.ExportedBy.Role + ")"},
		{l.ExportedAt, r.time(m.ExportedAt)},
		{l.SnapshotAt, r.time(m.SnapshotAt)},
		{l.Timezone, loc.String()},
		{l.ServerVersion, m.ServerVersion},
		{l.ExportID, m.ExportID.String()},
		{l.KeyFingerprint, m.SigningKey.Fingerprint},
	}
	c := m.Counts
	v.Counts = []kv{
		{l.Items, fmt.Sprint(c.Items)}, {l.Events, fmt.Sprint(c.Events)}, {l.Comments, fmt.Sprint(c.Comments)},
		{l.Approvals, fmt.Sprint(c.Approvals)}, {l.Links, fmt.Sprint(c.Links)}, {l.Attachments, fmt.Sprint(c.Attachments)},
	}

	byItem := map[uuid.UUID]*viewItem{}
	for _, it := range d.Items {
		vi := viewItem{
			Key: it.Key, Title: it.Title, Project: it.ProjectKey, Type: it.Type.Name, Status: it.Status.Name,
			Assignee: r.person(it.AssigneeID), Reporter: r.person(it.ReporterID),
			Created: r.time(it.CreatedAt), Updated: r.time(it.UpdatedAt), Deleted: r.timePtr(it.DeletedAt),
		}
		if it.DueDate != nil {
			vi.Due = *it.DueDate
		}
		if it.Description != nil {
			vi.Description = *it.Description
		}
		for _, k := range sortedKeys(it.Fields) {
			vi.Fields = append(vi.Fields, kv{k, r.value(it.Fields[k])})
		}
		v.Items = append(v.Items, vi)
	}
	for i := range v.Items {
		byItem[d.Items[i].ID] = &v.Items[i]
	}
	for _, a := range d.Approvals {
		vi := byItem[a.ItemID]
		if vi == nil {
			continue
		}
		va := viewApproval{
			Requested: r.time(a.CreatedAt), By: r.person(&a.RequestedBy),
			Transition: r.status(a.FromStatusID) + " → " + r.status(a.ToStatusID),
			State:      l.States[a.State], Required: fmt.Sprintf("%d / %d", a.Approvals, a.MinApprovals),
		}
		if a.Comment != nil {
			va.Comment = *a.Comment
		}
		if a.ClosedAt != nil {
			va.Closed = r.time(*a.ClosedAt) + " · " + r.person(a.ClosedBy)
		}
		if a.InvalidatedAt != nil {
			va.State += " · " + l.Invalidated + " (" + r.time(*a.InvalidatedAt) + ")"
		}
		if a.DueOn != nil {
			va.Required += " · " + l.Due + " " + *a.DueOn
		}
		for _, dec := range a.Decisions {
			vd := viewDecision{Who: r.person(&dec.UserID), Verb: l.Approve, Time: r.time(dec.CreatedAt)}
			if dec.OnBehalfOf != nil {
				vd.Who += " (" + fmt.Sprintf(l.OnBehalf, r.person(dec.OnBehalfOf)) + ")"
			}
			if dec.Decision == "reject" {
				vd.Verb = l.Reject
			}
			if dec.Comment != nil {
				vd.Comment = *dec.Comment
			}
			va.Decisions = append(va.Decisions, vd)
		}
		vi.Approvals = append(vi.Approvals, va)
	}
	for _, cm := range d.Comments {
		if vi := byItem[cm.ItemID]; vi != nil {
			vc := viewComment{Who: r.person(cm.AuthorID), Time: r.time(cm.CreatedAt), Deleted: cm.DeletedAt != nil,
				Edited: cm.UpdatedAt.Sub(cm.CreatedAt) > time.Second}
			if cm.Body != nil {
				vc.Body = *cm.Body
			}
			vi.Comments = append(vi.Comments, vc)
		}
	}
	for _, ln := range d.Links {
		text := ln.SourceKey + " " + l.LinkTypes[ln.Type] + " " + ln.TargetKey
		for _, id := range []uuid.UUID{ln.SourceID, ln.TargetID} {
			if vi := byItem[id]; vi != nil && !slices.Contains(vi.Links, text) {
				vi.Links = append(vi.Links, text)
			}
		}
	}
	for _, a := range d.Attachments {
		if vi := byItem[a.ItemID]; vi != nil {
			vi.Attachments = append(vi.Attachments, viewAttachment{Name: a.Filename, Size: humanSize(a.Size), SHA256: a.SHA256,
				Uploader: r.person(a.UploaderID), Time: r.time(a.CreatedAt)})
		}
	}
	for _, e := range d.Events {
		if vi := byItem[e.EntityID]; e.EntityType == "item" && vi != nil {
			vi.History = append(vi.History, r.event(e))
		} else {
			v.ProjectEvents = append(v.ProjectEvents, r.event(e))
		}
	}
	var buf bytes.Buffer
	if err := reportTemplate.Execute(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// readme explains the package to whoever opens it first.
func readme(m Manifest) string {
	return fmt.Sprintf(`Nebula audit sübut paketi / audit evidence package
Format: %s
Export ID: %s
İmza açarı / signing key: %s (Ed25519)

report.html          oxunaqlı hesabat (brauzerdə açın, PDF kimi çap edin) / human-readable report (print to PDF)
data/*.json          maşın üçün məlumat / machine-readable data
data/events.csv      hadisələr cədvəli / events as CSV
manifest.json        bütün faylların SHA-256-sı və ixrac parametrləri / SHA-256 of every file and the export parameters
manifest.sig         manifest.json-un Ed25519 imzası / Ed25519 signature of manifest.json

Yoxlama / verify:
  nebula evidence verify --key %s <zip>

Barmaq izini Nebula-da Parametrlər → Audit log səhifəsindəki ilə müqayisə edin.
Compare the fingerprint with the one shown in Nebula under Settings → Audit log.
`, m.Format, m.ExportID, m.SigningKey.Fingerprint, m.SigningKey.Fingerprint)
}
