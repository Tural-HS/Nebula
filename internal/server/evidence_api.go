package server

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/evidence"
	"github.com/Tural-HS/Nebula/internal/ratelimit"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Audit evidence export (ADR-0029). Exports are synchronous and bounded
// (store.DefaultEvidenceLimits, evidence.MaxPackageBytes): nothing is stored
// on the server, so no copy of private data outlives the download.

// evidenceExports bounds the work evidence exports can cause.
type evidenceExports struct {
	signer  *evidence.Signer // nil: exports disabled
	limiter *ratelimit.Limiter
	slots   chan struct{} // concurrent exports per process
	limits  store.EvidenceLimits
}

func newEvidenceExports(s *evidence.Signer) *evidenceExports {
	return &evidenceExports{
		signer: s,
		// 20 per hour per user, at most 5 back to back.
		limiter: ratelimit.New(20.0/3600, 5),
		slots:   make(chan struct{}, 2),
		limits:  store.DefaultEvidenceLimits,
	}
}

func (a *api) evidenceRoutes(r chi.Router) {
	admin := r.With(requireRole(store.RoleAdmin))
	admin.Get("/evidence/key", a.evidenceKey)
	admin.Post("/evidence/exports", a.exportScopeEvidence)
}

func (a *api) itemEvidenceRoutes(r chi.Router) {
	r.With(requireRole(store.RoleAdmin)).Post("/items/{number}/evidence", a.exportItemEvidence)
}

// evidenceKey is what Settings shows: the fingerprint auditors compare.
func (a *api) evidenceKey(w http.ResponseWriter, _ *http.Request) {
	if a.evidence.signer == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	info := a.evidence.signer.Info()
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "algorithm": info.Algorithm,
		"public_key": info.PublicKey, "fingerprint": info.Fingerprint,
		"limits": map[string]int{"max_days": a.evidence.limits.MaxDays, "max_items": a.evidence.limits.MaxItems,
			"max_events": a.evidence.limits.MaxEvents}})
}

type evidenceBody struct {
	ProjectKeys   []string `json:"project_keys"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	ApprovalsOnly bool     `json:"approvals_only"`
	Language      string   `json:"language"`
}

func (a *api) exportScopeEvidence(w http.ResponseWriter, r *http.Request) {
	var body evidenceBody
	if !a.decode(w, r, &body) {
		return
	}
	ws := workspaceFrom(r)
	q := store.EvidenceQuery{WorkspaceID: ws.ID, Viewer: viewer(r), ProjectKeys: body.ProjectKeys,
		From: body.From, To: body.To, ApprovalsOnly: body.ApprovalsOnly}
	if err := store.CheckEvidenceScope(q); err != nil {
		a.fail(w, r, err)
		return
	}
	params := evidence.Params{ProjectKeys: body.ProjectKeys, From: strings.TrimSpace(body.From), To: strings.TrimSpace(body.To),
		ApprovalsOnly: body.ApprovalsOnly, Language: evidence.NormalizeLanguage(body.Language)}
	a.exportEvidence(w, r, q, evidence.KindScope, params)
}

func (a *api) exportItemEvidence(w http.ResponseWriter, r *http.Request) {
	var body evidenceBody
	if !a.decode(w, r, &body) {
		return
	}
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	p := projectFrom(r)
	q := store.EvidenceQuery{WorkspaceID: workspaceFrom(r).ID, Viewer: viewer(r), ProjectID: p.ID, Number: n}
	params := evidence.Params{ItemKey: p.Key + "-" + strconv.FormatInt(n, 10), Language: evidence.NormalizeLanguage(body.Language)}
	a.exportEvidence(w, r, q, evidence.KindItem, params)
}

func (a *api) exportEvidence(w http.ResponseWriter, r *http.Request, q store.EvidenceQuery, kind string, params evidence.Params) {
	ex := a.evidence
	if ex.signer == nil {
		writeError(w, http.StatusServiceUnavailable, "evidence_disabled",
			"evidence export is disabled: NEBULA_EVIDENCE_SIGNING_KEY is not configured")
		return
	}
	me := principal(r)
	if ok, wait := ex.limiter.Allow(me.UserID.String()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(wait.Seconds()+0.999))))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many evidence exports; try again later")
		return
	}
	select {
	case ex.slots <- struct{}{}:
		defer func() { <-ex.slots }()
	default:
		w.Header().Set("Retry-After", "10")
		writeError(w, http.StatusTooManyRequests, "export_busy", "other exports are running; try again shortly")
		return
	}
	q.Limits = ex.limits
	set, err := a.store.CollectEvidence(r.Context(), q)
	var tooLarge *store.EvidenceTooLargeError
	if errors.As(err, &tooLarge) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{
			"code": "export_too_large", "message": tooLarge.Error(), "field": tooLarge.What, "limit": tooLarge.Limit}})
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	ws := workspaceFrom(r)
	in := evidence.Input{
		ExportID: uuid.New(), Kind: kind, Params: params, ExportedAt: time.Now(), ServerVersion: a.version, Data: set,
		Exporter: evidence.Person{ID: me.UserID, DisplayName: me.DisplayName, Email: me.Email, Role: ws.Role},
	}
	var buf bytes.Buffer
	res, err := evidence.Build(&buf, in, ex.signer)
	if errors.Is(err, evidence.ErrTooLarge) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{
			"code": "export_too_large", "message": err.Error(), "field": "bytes", "limit": evidence.MaxPackageBytes}})
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}

	// The export is recorded before a single byte leaves: no unaudited export.
	payload := map[string]any{
		"export_id": in.ExportID, "kind": kind, "manifest_sha256": res.ManifestSHA256,
		"key_fingerprint": ex.signer.Fingerprint(), "language": params.Language, "counts": res.Manifest.Counts,
		"bytes": res.Bytes, "approvals_only": params.ApprovalsOnly,
	}
	var itemID *uuid.UUID
	if kind == evidence.KindItem {
		itemID = &set.Items[0].ID
		payload["item_key"] = params.ItemKey
	} else {
		// Project ids, not keys: a workspace-level event reaches every webhook,
		// and the key of a private project must not (ADR-0013).
		ids := make([]uuid.UUID, len(set.Projects))
		for i, p := range set.Projects {
			ids[i] = p.ID
		}
		payload["project_ids"], payload["all_projects"] = ids, len(params.ProjectKeys) == 0
		payload["from"], payload["to"] = params.From, params.To
	}
	if err := a.store.RecordEvidenceExport(r.Context(), ws.ID, me.UserID, itemID, payload); err != nil {
		a.fail(w, r, err)
		return
	}

	name := "nebula-evidence-" + ws.Slug + "-"
	if kind == evidence.KindItem {
		name += params.ItemKey
	} else {
		name += params.From + "_" + params.To
	}
	h := w.Header()
	h.Set("Content-Type", "application/zip")
	h.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, asciiFileName(name)))
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	h.Set("Cache-Control", "no-store")
	h.Set("X-Evidence-Export-Id", in.ExportID.String())
	h.Set("X-Evidence-Manifest-Sha256", res.ManifestSHA256)
	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
}

// asciiFileName keeps [A-Za-z0-9._-]: slugs, item keys and dates are
// already ASCII; anything else must not reach a header.
func asciiFileName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			return r
		}
		return '_'
	}, s)
}
