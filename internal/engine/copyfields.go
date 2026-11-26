package engine

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/store"
)

// copyFields runs one copy_fields action (ADR-0056): the listed field values
// of src are written to every live item of the target project whose match
// field holds the same value. Only differing values are written, so a
// converged pair (or a reverse rule) writes nothing.
//
// Per-item rejections (validation, approval locks, an item deleted since)
// are logged and skipped; they would fail again on retry. A version
// conflict that persists after a re-read is returned after the other
// targets are done, so the job is retried: targets already in sync are
// no-ops then.
func (e *Engine) copyFields(ctx context.Context, log *slog.Logger, wsID uuid.UUID, src store.Item, a store.Action) error {
	log = log.With("action", store.ActionCopyFields, "target_project", a.Project)
	value := src.Fields[a.Match]
	if isEmpty(value) {
		return nil // nothing to match on
	}
	if projectKey(src.Key) == a.Project {
		return nil // never copy within the source project
	}
	targets, err := e.Store.CopyFieldTargets(ctx, wsID, a.Project, a.Match, value, src.ID, store.MaxCopyTargets)
	if errors.Is(err, store.ErrNotFound) {
		log.Warn("copy_fields target project is missing or archived")
		return nil
	}
	if err != nil {
		return err
	}
	if targets.More {
		log.Warn("copy_fields matched too many items; only the first are updated", "limit", store.MaxCopyTargets)
	}
	var retry error
	for _, t := range targets.Items {
		err := e.copyTo(ctx, wsID, src, t, a.Fields, targets.Bound[t.Type.ID])
		var ve *store.ValidationError
		var ce *store.ConflictError
		switch {
		case err == nil:
		case errors.As(err, &ce) && ce.Code == "version_conflict":
			log.Warn("copy_fields target changed concurrently; will retry", "target", t.Key)
			retry = err
		case errors.As(err, &ve):
			log.Warn("copy_fields target rejected the values", "target", t.Key, "field", ve.Field, "reason", ve.Message)
		case errors.As(err, &ce):
			log.Warn("copy_fields target rejected the values", "target", t.Key, "code", ce.Code)
		default:
			return err
		}
	}
	return retry
}

// copyTo writes the patch to one target, re-reading it once on a version
// conflict with a concurrent edit.
func (e *Engine) copyTo(ctx context.Context, wsID uuid.UUID, src, t store.Item, fields []string, bound map[string]bool) error {
	for attempt := 0; ; attempt++ {
		patch, err := CopyPatch(src.Fields, t.Fields, fields, bound)
		if err != nil || len(patch) == 0 {
			return err
		}
		_, err = e.Store.UpdateItem(ctx, store.UpdateItemParams{
			WorkspaceID: wsID, ProjectID: t.ProjectID, Number: t.Number, Version: t.Version,
			Fields: opt.Of(patch),
		})
		var ce *store.ConflictError
		if errors.As(err, &ce) && ce.Code == "version_conflict" && attempt < 1 {
			if t, err = e.Store.ItemByID(ctx, wsID, t.ID); errors.Is(err, store.ErrNotFound) {
				return nil // deleted since
			} else if err != nil {
				return err
			}
			continue
		}
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
}

// CopyPatch is the field patch that makes dst hold src's values of keys:
// differing values are set, a value missing in src clears dst's (null).
// A value is only set on a field the target's item type uses (bound); a
// clear needs no binding.
func CopyPatch(src, dst map[string]any, keys []string, bound map[string]bool) (map[string]json.RawMessage, error) {
	patch := map[string]json.RawMessage{}
	for _, k := range keys {
		sv, has := src[k]
		dv, present := dst[k]
		if !has || sv == nil {
			if present && dv != nil {
				patch[k] = json.RawMessage("null")
			}
			continue
		}
		if !bound[k] || (present && equal(sv, dv)) {
			continue
		}
		raw, err := json.Marshal(sv)
		if err != nil {
			return nil, err
		}
		patch[k] = raw
	}
	return patch, nil
}
