package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// copy_fields (ADR-0056): copy field values of the triggering item to the
// items of another project that share a match field's value.
const (
	maxCopyFields = 30
	// MaxCopyTargets bounds how many matching items one copy_fields action
	// updates; more matches are logged and ignored.
	MaxCopyTargets = 20
)

// copyMatchTypes are the field types a copy_fields action can match on:
// identifiers such as a contract number are text or number fields.
var copyMatchTypes = []string{FieldText, FieldNumber}

// validateCopyFields checks a copy_fields action at rule save time.
func validateCopyFields(ctx context.Context, q querier, wsID uuid.UUID, path string, a Action) error {
	if !projectKeyRe.MatchString(a.Project) {
		return invalid(path+".project", "must be a project key")
	}
	var archived bool
	err := q.QueryRow(ctx, `SELECT archived_at IS NOT NULL OR deletion_requested_at IS NOT NULL
		FROM projects WHERE workspace_id = $1 AND key = $2`, wsID, a.Project).Scan(&archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return invalid(path+".project", "unknown project")
	}
	if err != nil {
		return err
	}
	if archived {
		return invalid(path+".project", "project is archived")
	}

	if !fieldKeyRe.MatchString(a.Match) {
		return invalid(path+".match", "must be a field key")
	}
	if len(a.Fields) == 0 || len(a.Fields) > maxCopyFields {
		return invalid(path+".fields", "must contain 1 to %d fields", maxCopyFields)
	}
	for i, k := range a.Fields {
		if !fieldKeyRe.MatchString(k) {
			return invalid(fmt.Sprintf("%s.fields[%d]", path, i), "must be a field key")
		}
		if k == a.Match {
			return invalid(fmt.Sprintf("%s.fields[%d]", path, i), "the match field cannot be copied")
		}
		if slices.Index(a.Fields, k) != i {
			return invalid(fmt.Sprintf("%s.fields[%d]", path, i), "duplicate field %q", k)
		}
	}

	types := map[string]string{}
	rows, err := q.Query(ctx, `SELECT key, field_type FROM field_definitions
		WHERE workspace_id = $1 AND archived_at IS NULL AND key = ANY($2)`, wsID, append([]string{a.Match}, a.Fields...))
	if err != nil {
		return err
	}
	for rows.Next() {
		var k, typ string
		if err := rows.Scan(&k, &typ); err != nil {
			rows.Close()
			return err
		}
		types[k] = typ
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if typ, ok := types[a.Match]; !ok {
		return invalid(path+".match", "unknown field")
	} else if !slices.Contains(copyMatchTypes, typ) {
		return invalid(path+".match", "must be a text or number field")
	}
	for i, k := range a.Fields {
		typ, ok := types[k]
		switch {
		case !ok:
			return invalid(fmt.Sprintf("%s.fields[%d]", path, i), "unknown field")
		case typ == FieldFormula:
			return invalid(fmt.Sprintf("%s.fields[%d]", path, i), "is a formula field: its value is computed and cannot be set")
		}
	}
	return nil
}

// CopyTargets is what a copy_fields action writes to.
type CopyTargets struct {
	ProjectID uuid.UUID
	Items     []Item
	// More: more than the requested limit of items matched.
	More bool
	// Bound lists, per item type of the returned items, the fields that type
	// uses: a value can only be set on a field of the item's type.
	Bound map[uuid.UUID]map[string]bool
}

// CopyFieldTargets returns the live items of the project with key
// projectKey whose field match equals value, excluding item exclude, at
// most limit of them (by number). ErrNotFound: the project does not exist,
// is archived or is being deleted.
func (s *Store) CopyFieldTargets(ctx context.Context, wsID uuid.UUID, projectKey, match string, value any,
	exclude uuid.UUID, limit int) (CopyTargets, error) {
	var out CopyTargets
	err := s.pool.QueryRow(ctx, `SELECT id FROM projects WHERE workspace_id = $1 AND key = $2
		AND archived_at IS NULL AND deletion_requested_at IS NULL`, wsID, projectKey).Scan(&out.ProjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	// Containment uses the GIN index on items.fields; numbers compare by value.
	rows, err := s.pool.Query(ctx, itemSelect+`
		WHERE i.workspace_id = $1 AND i.project_id = $2 AND i.deleted_at IS NULL AND i.id <> $5
		  AND i.fields @> jsonb_build_object($3::text, $4::jsonb)
		ORDER BY i.number LIMIT $6`, wsID, out.ProjectID, match, string(raw), exclude, limit+1)
	if err != nil {
		return out, err
	}
	out.Items, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Item, error) { return scanItem(r) })
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items, out.More = out.Items[:limit], true
	}

	out.Bound = map[uuid.UUID]map[string]bool{}
	typeIDs := []string{}
	for _, it := range out.Items {
		if _, ok := out.Bound[it.Type.ID]; !ok {
			out.Bound[it.Type.ID] = map[string]bool{}
			typeIDs = append(typeIDs, it.Type.ID.String())
		}
	}
	if len(typeIDs) == 0 {
		return out, nil
	}
	rows, err = s.pool.Query(ctx, `SELECT b.item_type_id, f.key FROM item_type_fields b
		JOIN field_definitions f ON f.id = b.field_id
		WHERE f.workspace_id = $1 AND b.item_type_id = ANY($2::uuid[])`, wsID, typeIDs)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var typeID uuid.UUID
		var key string
		if err := rows.Scan(&typeID, &key); err != nil {
			return out, err
		}
		out.Bound[typeID][key] = true
	}
	return out, rows.Err()
}

// CreateCopyTarget creates the target item of a copy_fields action with
// create (ADR-0056) when none matches yet: title, the match value and the
// listed fields the target type uses, as the system. The type is the one
// most items of the target project have (else the workspace default).
//
// A transaction-scoped advisory lock on (project, match, value) and a
// re-check inside it keep two concurrent jobs (two quick edits of the
// source) from creating it twice. created is false when it already exists.
func (s *Store) CreateCopyTarget(ctx context.Context, wsID, projectID uuid.UUID, title, match string, value any,
	src map[string]any, fields []string) (item Item, created bool, err error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return Item{}, false, err
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || $2 || ':' || $3, 0))`,
			projectID.String(), match, string(raw)); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM items i WHERE i.workspace_id = $1 AND i.project_id = $2
			AND i.deleted_at IS NULL AND i.fields @> jsonb_build_object($3::text, $4::jsonb))`,
			wsID, projectID, match, string(raw)).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
		var typeKey string
		err := tx.QueryRow(ctx, `SELECT t.key FROM items i JOIN item_types t ON t.id = i.item_type_id
			WHERE i.workspace_id = $1 AND i.project_id = $2 AND i.deleted_at IS NULL AND t.archived_at IS NULL
			GROUP BY t.key ORDER BY count(*) DESC, t.key LIMIT 1`, wsID, projectID).Scan(&typeKey)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		bound := map[string]bool{}
		rows, err := tx.Query(ctx, `SELECT f.key FROM item_type_fields b
			JOIN item_types t ON t.id = b.item_type_id JOIN field_definitions f ON f.id = b.field_id
			WHERE t.workspace_id = $1 AND t.key = COALESCE(NULLIF($2, ''), 'task') AND f.archived_at IS NULL`, wsID, typeKey)
		if err != nil {
			return err
		}
		keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, k := range keys {
			bound[k] = true
		}
		values := map[string]json.RawMessage{}
		for _, k := range append([]string{match}, fields...) {
			v, ok := src[k]
			if !ok || v == nil || (typeKey != "" && !bound[k]) {
				continue
			}
			b, err := json.Marshal(v)
			if err != nil {
				return err
			}
			values[k] = b
		}
		item, err = createItemTx(ctx, tx, CreateItemParams{
			WorkspaceID: wsID, ProjectID: projectID, Title: title, TypeKey: typeKey, Fields: values,
		}, nil)
		created = err == nil
		return err
	})
	return item, created, err
}
