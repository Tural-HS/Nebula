package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Archive, restore and permanent deletion of projects and workspaces
// (ADR-0039, migration 00031).
//
// Archiving is reversible: the row stays, lists and background work skip it,
// and the HTTP layer refuses writes. Deletion is only possible for an
// archived project or workspace: the request marks it (deletion_requested_at)
// and records the audit event; PurgeDeletions then removes the rows in
// bounded batches, each in its own transaction, so a crash or a second
// replica only repeats harmless work.

// Lifecycle event actions.
const (
	ActionProjectArchived            = "project.archived"
	ActionProjectRestored            = "project.restored"
	ActionProjectDeleted             = "project.deleted"
	ActionWorkspaceArchived          = "workspace.archived"
	ActionWorkspaceRestored          = "workspace.restored"
	ActionWorkspaceDeletionRequested = "workspace.deletion_requested"
)

const (
	purgeItemBatch  = 500  // items per DELETE: each cascades to comments, attachments, links, …
	purgeEventBatch = 5000 // events per DELETE (notifications cascade)
)

// GetProjectAnyState returns a project the viewer can see, archived or not.
// Callers decide what an archived or deleting project allows.
func (s *Store) GetProjectAnyState(ctx context.Context, wsID uuid.UUID, key string, v Viewer) (Project, error) {
	rows, err := s.pool.Query(ctx, projectSelect+` WHERE workspace_id = $1 AND key = $4 AND `+visibleToViewer,
		wsID, v.UserID, RoleAtLeast(v.Role, RoleAdmin), strings.ToUpper(key))
	if err != nil {
		return Project{}, err
	}
	p, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Project])
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

// ListArchivedProjects returns the workspace's archived projects (admins
// see every project, ADR-0013), most recently archived first.
func (s *Store) ListArchivedProjects(ctx context.Context, wsID uuid.UUID) ([]Project, error) {
	rows, err := s.pool.Query(ctx, projectSelect+` WHERE workspace_id = $1 AND archived_at IS NOT NULL
		ORDER BY archived_at DESC, key`, wsID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Project])
}

// ArchiveProject hides a project (admin+, checked by the caller).
func (s *Store) ArchiveProject(ctx context.Context, wsID, projectID, actorID uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var key, name string
		err := tx.QueryRow(ctx, `UPDATE projects SET archived_at = now()
			WHERE workspace_id = $1 AND id = $2 AND archived_at IS NULL RETURNING key, name`, wsID, projectID).Scan(&key, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			return &ConflictError{Code: "project_archived", Message: "the project is already archived"}
		}
		if err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "project", projectID, ActionProjectArchived,
			map[string]any{"project_id": projectID, "key": key, "name": name})
	})
}

// RestoreProject makes an archived project active again. A project whose
// deletion was requested cannot be restored.
func (s *Store) RestoreProject(ctx context.Context, wsID, projectID, actorID uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var key, name string
		var archived, deleting bool
		err := tx.QueryRow(ctx, `SELECT key, name, archived_at IS NOT NULL, deletion_requested_at IS NOT NULL
			FROM projects WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, wsID, projectID).Scan(&key, &name, &archived, &deleting)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		switch {
		case deleting:
			return &ConflictError{Code: "project_deleting", Message: "the project is being deleted and cannot be restored"}
		case !archived:
			return &ConflictError{Code: "project_not_archived", Message: "the project is not archived"}
		}
		if _, err := tx.Exec(ctx, `UPDATE projects SET archived_at = NULL WHERE id = $1`, projectID); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "project", projectID, ActionProjectRestored,
			map[string]any{"project_id": projectID, "key": key, "name": name})
	})
}

// DeletionSummary counts what a deletion removes, for the audit record.
type DeletionSummary struct {
	Projects    int64 `json:"projects,omitempty"`
	Items       int64 `json:"items"`
	Attachments int64 `json:"attachments"`
	Members     int64 `json:"members,omitempty"`
	Events      int64 `json:"events,omitempty"`
	// EvidenceExported: an evidence package (ADR-0029) was exported after
	// the workspace was archived (workspace deletions only).
	EvidenceExported *bool `json:"evidence_exported_since_archive,omitempty"`
}

func (d DeletionSummary) payload() map[string]any {
	m := map[string]any{"items": d.Items, "attachments": d.Attachments}
	if d.Projects > 0 {
		m["projects"] = d.Projects
	}
	if d.Members > 0 {
		m["members"] = d.Members
	}
	if d.Events > 0 {
		m["events"] = d.Events
	}
	if d.EvidenceExported != nil {
		m["evidence_exported_since_archive"] = *d.EvidenceExported
	}
	return m
}

// RequestProjectDeletion marks an archived project for permanent deletion
// (owner, checked by the caller). confirm must be the project key. Asking
// again while the purge runs is a no-op.
func (s *Store) RequestProjectDeletion(ctx context.Context, wsID, projectID, actorID uuid.UUID, confirm string) (DeletionSummary, error) {
	var sum DeletionSummary
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var key, name string
		var archived, deleting bool
		err := tx.QueryRow(ctx, `SELECT key, name, archived_at IS NOT NULL, deletion_requested_at IS NOT NULL
			FROM projects WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, wsID, projectID).Scan(&key, &name, &archived, &deleting)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !archived {
			return &ConflictError{Code: "project_not_archived", Message: "archive the project before deleting it"}
		}
		if strings.TrimSpace(confirm) != key {
			return invalid("confirm", "type the project key %s to confirm", key)
		}
		if deleting {
			return nil
		}
		if err := tx.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM items WHERE workspace_id = $1 AND project_id = $2),
				(SELECT count(*) FROM attachments a JOIN items i ON i.workspace_id = a.workspace_id AND i.id = a.item_id
				 WHERE i.workspace_id = $1 AND i.project_id = $2)`, wsID, projectID).Scan(&sum.Items, &sum.Attachments); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE projects SET deletion_requested_at = now() WHERE id = $1`, projectID); err != nil {
			return err
		}
		p := sum.payload()
		p["project_id"], p["key"], p["name"] = projectID, key, name
		return appendEvent(ctx, tx, wsID, actorID, "project", projectID, ActionProjectDeleted, p)
	})
	return sum, err
}

// ListArchivedWorkspaces returns the archived workspaces in which userID is
// an admin or owner (the "archived workspaces" section of the home page).
func (s *Store) ListArchivedWorkspaces(ctx context.Context, userID uuid.UUID) ([]Workspace, error) {
	rows, err := s.pool.Query(ctx, workspaceSelect+` WHERE m.user_id = $1 AND w.archived_at IS NOT NULL
		AND m.role IN ('admin', 'owner') ORDER BY w.archived_at DESC, w.id`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Workspace])
}

// ArchiveWorkspace makes a workspace read-only and hides it (owner, checked
// by the caller).
func (s *Store) ArchiveWorkspace(ctx context.Context, wsID, actorID uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var slug, name string
		err := tx.QueryRow(ctx, `UPDATE workspaces SET archived_at = now() WHERE id = $1 AND archived_at IS NULL
			RETURNING slug, name`, wsID).Scan(&slug, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			return &ConflictError{Code: "workspace_archived", Message: "the workspace is already archived"}
		}
		if err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "workspace", wsID, ActionWorkspaceArchived, map[string]any{"slug": slug, "name": name})
	})
}

// RestoreWorkspace makes an archived workspace active again (admin+).
func (s *Store) RestoreWorkspace(ctx context.Context, wsID, actorID uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var slug, name string
		var archived, deleting bool
		err := tx.QueryRow(ctx, `SELECT slug, name, archived_at IS NOT NULL, deletion_requested_at IS NOT NULL
			FROM workspaces WHERE id = $1 FOR UPDATE`, wsID).Scan(&slug, &name, &archived, &deleting)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		switch {
		case deleting:
			return &ConflictError{Code: "workspace_deleting", Message: "the workspace is being deleted and cannot be restored"}
		case !archived:
			return &ConflictError{Code: "workspace_not_archived", Message: "the workspace is not archived"}
		}
		if _, err := tx.Exec(ctx, `UPDATE workspaces SET archived_at = NULL WHERE id = $1`, wsID); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "workspace", wsID, ActionWorkspaceRestored, map[string]any{"slug": slug, "name": name})
	})
}

// RequestWorkspaceDeletion marks an archived workspace for permanent
// deletion (owner, checked by the caller). confirm must be the slug and the
// owner must acknowledge the evidence package (ADR-0039). The audit record
// goes to workspace_deletions, which outlives the workspace. Asking again
// while the purge runs is a no-op.
func (s *Store) RequestWorkspaceDeletion(ctx context.Context, wsID, actorID uuid.UUID, confirm string, evidenceAck bool) (DeletionSummary, error) {
	var sum DeletionSummary
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var slug, name string
		var archivedAt *time.Time
		var deleting bool
		err := tx.QueryRow(ctx, `SELECT slug, name, archived_at, deletion_requested_at IS NOT NULL
			FROM workspaces WHERE id = $1 FOR UPDATE`, wsID).Scan(&slug, &name, &archivedAt, &deleting)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if archivedAt == nil {
			return &ConflictError{Code: "workspace_not_archived", Message: "archive the workspace before deleting it"}
		}
		if strings.TrimSpace(strings.ToLower(confirm)) != slug {
			return invalid("confirm", "type the workspace slug %s to confirm", slug)
		}
		if !evidenceAck {
			return invalid("evidence_acknowledged", "confirm that the audit evidence package was downloaded or is not needed")
		}
		if deleting {
			return nil
		}
		var exported bool
		if err := tx.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM projects WHERE workspace_id = $1),
				(SELECT count(*) FROM items WHERE workspace_id = $1),
				(SELECT count(*) FROM attachments WHERE workspace_id = $1),
				(SELECT count(*) FROM workspace_members WHERE workspace_id = $1),
				(SELECT count(*) FROM events WHERE workspace_id = $1),
				EXISTS (SELECT 1 FROM events WHERE workspace_id = $1 AND action = 'evidence.exported' AND occurred_at >= $2)`,
			wsID, *archivedAt).Scan(&sum.Projects, &sum.Items, &sum.Attachments, &sum.Members, &sum.Events, &exported); err != nil {
			return err
		}
		sum.EvidenceExported = &exported
		if _, err := tx.Exec(ctx, `UPDATE workspaces SET deletion_requested_at = now() WHERE id = $1`, wsID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workspace_deletions (workspace_id, slug, name, requested_by, summary)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (workspace_id) DO NOTHING`, wsID, slug, name, actorID, sum.payload()); err != nil {
			return err
		}
		p := sum.payload()
		p["slug"], p["name"] = slug, name
		return appendEvent(ctx, tx, wsID, actorID, "workspace", wsID, ActionWorkspaceDeletionRequested, p)
	})
	return sum, err
}

// WorkspaceDeletion is the audit record of a deleted (or deleting) workspace.
type WorkspaceDeletion struct {
	WorkspaceID uuid.UUID       `json:"workspace_id"`
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	RequestedBy *uuid.UUID      `json:"requested_by"`
	RequestedAt time.Time       `json:"requested_at"`
	CompletedAt *time.Time      `json:"completed_at"`
	Summary     DeletionSummary `json:"summary"`
}

// GetWorkspaceDeletion returns the deletion record of a workspace.
func (s *Store) GetWorkspaceDeletion(ctx context.Context, wsID uuid.UUID) (WorkspaceDeletion, error) {
	var d WorkspaceDeletion
	err := s.pool.QueryRow(ctx, `SELECT workspace_id, slug, name, requested_by, requested_at, completed_at, summary
		FROM workspace_deletions WHERE workspace_id = $1`, wsID).
		Scan(&d.WorkspaceID, &d.Slug, &d.Name, &d.RequestedBy, &d.RequestedAt, &d.CompletedAt, &d.Summary)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// WorkspaceActive reports whether a workspace exists and is not archived:
// background work (email) for anything else is dropped.
func (s *Store) WorkspaceActive(ctx context.Context, wsID uuid.UUID) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspaces WHERE id = $1 AND archived_at IS NULL)`, wsID).Scan(&ok)
	return ok, err
}

// ---------------------------------------------------------------------------
// Purge
// ---------------------------------------------------------------------------

// Purged is one finished deletion, for the server log.
type Purged struct {
	Kind        string // "project" or "workspace"
	WorkspaceID uuid.UUID
	ProjectID   uuid.UUID // projects only
	Name        string    // project key or workspace slug
}

// PurgeDeletions works off requested deletions until they are done or the
// deadline passes, and returns the deletions it finished. Projects of a
// workspace that is itself being deleted go with the workspace.
func (s *Store) PurgeDeletions(ctx context.Context, deadline time.Time) ([]Purged, error) {
	var done []Purged
	rows, err := s.pool.Query(ctx, `SELECT p.workspace_id, p.id, p.key FROM projects p JOIN workspaces w ON w.id = p.workspace_id
		WHERE p.deletion_requested_at IS NOT NULL AND w.deletion_requested_at IS NULL
		ORDER BY p.deletion_requested_at, p.id LIMIT 20`)
	if err != nil {
		return nil, err
	}
	type target struct {
		ws, id uuid.UUID
		name   string
	}
	projects, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (target, error) {
		var t target
		return t, r.Scan(&t.ws, &t.id, &t.name)
	})
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		finished, err := s.PurgeProject(ctx, p.ws, p.id, deadline)
		if err != nil {
			return done, err
		}
		if !finished {
			return done, nil
		}
		done = append(done, Purged{Kind: "project", WorkspaceID: p.ws, ProjectID: p.id, Name: p.name})
	}
	rows, err = s.pool.Query(ctx, `SELECT id, id, slug FROM workspaces WHERE deletion_requested_at IS NOT NULL
		ORDER BY deletion_requested_at, id LIMIT 5`)
	if err != nil {
		return done, err
	}
	workspaces, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (target, error) {
		var t target
		return t, r.Scan(&t.ws, &t.id, &t.name)
	})
	if err != nil {
		return done, err
	}
	for _, w := range workspaces {
		finished, err := s.PurgeWorkspace(ctx, w.id, deadline)
		if err != nil {
			return done, err
		}
		if !finished {
			return done, nil
		}
		done = append(done, Purged{Kind: "workspace", WorkspaceID: w.id, Name: w.name})
	}
	return done, nil
}

// purgeBatches repeats one bounded DELETE until it removes fewer than batch
// rows (done) or the deadline passes (not done yet).
func (s *Store) purgeBatches(ctx context.Context, deadline time.Time, batch int, sql string, args ...any) (bool, error) {
	for {
		if !time.Now().Before(deadline) {
			return false, nil
		}
		tag, err := s.pool.Exec(ctx, sql, append(args, batch)...)
		if err != nil {
			return false, err
		}
		if tag.RowsAffected() < int64(batch) {
			return true, nil
		}
	}
}

// PurgeProject deletes a project whose deletion was requested: its items in
// batches (cascading to comments, attachments and their blob purges,
// links, watchers, approvals, notifications, search documents, overdue
// marks, recurrence runs and form submissions), then the project row
// (cascading to forms, saved views, recurrences, members and the remaining
// notifications). Webhook project filters drop the project; a webhook left
// without a project is disabled rather than widened to every project.
// It reports whether the project is gone. Safe to call again at any point.
func (s *Store) PurgeProject(ctx context.Context, wsID, projectID uuid.UUID, deadline time.Time) (bool, error) {
	// The EXISTS guard: never touch a project nobody asked to delete.
	done, err := s.purgeBatches(ctx, deadline, purgeItemBatch, `DELETE FROM items WHERE id IN (
			SELECT i.id FROM items i WHERE i.workspace_id = $1 AND i.project_id = $2 LIMIT $3)
		AND EXISTS (SELECT 1 FROM projects p WHERE p.id = $2 AND p.deletion_requested_at IS NOT NULL)`, wsID, projectID)
	if err != nil || !done {
		return false, err
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var key string
		err := tx.QueryRow(ctx, `SELECT key FROM projects WHERE workspace_id = $1 AND id = $2 AND deletion_requested_at IS NOT NULL
			FOR UPDATE`, wsID, projectID).Scan(&key)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // already gone (another replica finished it)
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE webhooks SET project_ids = array_remove(project_ids, $2),
				is_active = is_active AND cardinality(array_remove(project_ids, $2)) > 0,
				disabled_reason = CASE WHEN cardinality(array_remove(project_ids, $2)) = 0
					THEN 'disabled: its only project was deleted' ELSE disabled_reason END
			WHERE workspace_id = $1 AND $2 = ANY(project_ids)`, wsID, projectID); err != nil {
			return err
		}
		// A token restricted to this project only would be left with an
		// empty allow-list: revoke it (and deactivate its principal) rather
		// than leave it to the empty-list rule alone (ADR-0043).
		rows, err := tx.Query(ctx, `UPDATE service_tokens t SET revoked_at = now()
			WHERE t.workspace_id = $1 AND t.revoked_at IS NULL AND t.project_restricted
			  AND EXISTS (SELECT 1 FROM service_token_projects sp WHERE sp.token_id = t.id AND sp.project_id = $2)
			  AND NOT EXISTS (SELECT 1 FROM service_token_projects sp WHERE sp.token_id = t.id AND sp.project_id <> $2)
			RETURNING t.id, t.user_id, t.name`, wsID, projectID)
		if err != nil {
			return err
		}
		type revokedToken struct {
			ID, UserID uuid.UUID
			Name       string
		}
		revoked, err := pgx.CollectRows(rows, pgx.RowToStructByPos[revokedToken])
		if err != nil {
			return err
		}
		for _, rt := range revoked {
			if _, err := tx.Exec(ctx, `UPDATE users SET is_active = false WHERE id = $1`, rt.UserID); err != nil {
				return err
			}
			if err := appendEvent(ctx, tx, wsID, uuid.Nil, serviceTokenEntity, rt.ID, ServiceTokenRevokedAction, map[string]any{
				"name": rt.Name, "reason": "its only project was deleted", "project_key": key,
			}); err != nil {
				return err
			}
		}
		// Recurrences reference item types without a cascade; delete them
		// explicitly before the project (which would cascade to them anyway).
		if _, err := tx.Exec(ctx, `DELETE FROM recurrences WHERE workspace_id = $1 AND project_id = $2`, wsID, projectID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM projects WHERE workspace_id = $1 AND id = $2`, wsID, projectID)
		return err
	})
	return err == nil, err
}

// PurgeWorkspace deletes a workspace whose deletion was requested: items and
// events in batches, then (in one transaction) the rows that reference item
// types without a cascade and the workspace itself, which cascades to
// everything else. The deletion record is completed in the same
// transaction. Safe to call again at any point.
func (s *Store) PurgeWorkspace(ctx context.Context, wsID uuid.UUID, deadline time.Time) (bool, error) {
	const guard = ` AND EXISTS (SELECT 1 FROM workspaces w WHERE w.id = $1 AND w.deletion_requested_at IS NOT NULL)`
	done, err := s.purgeBatches(ctx, deadline, purgeItemBatch, `DELETE FROM items WHERE id IN (
			SELECT i.id FROM items i WHERE i.workspace_id = $1 LIMIT $2)`+guard, wsID)
	if err != nil || !done {
		return false, err
	}
	done, err = s.purgeBatches(ctx, deadline, purgeEventBatch, `DELETE FROM events WHERE id IN (
			SELECT e.id FROM events e WHERE e.workspace_id = $1 LIMIT $2)`+guard, wsID)
	if err != nil || !done {
		return false, err
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var one int
		err := tx.QueryRow(ctx, `SELECT 1 FROM workspaces WHERE id = $1 AND deletion_requested_at IS NOT NULL FOR UPDATE`, wsID).Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		// Same order as web/e2e/cleanup.sql: item types, statuses and
		// workflows are referenced without a cascade on purpose.
		for _, table := range []string{"recurrences", "items", "forms", "item_types"} {
			if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE workspace_id = $1`, wsID); err != nil {
				return err
			}
		}
		// Token principals outlive the workspace (users rows are kept for
		// history); make sure none of them can sign in as anything again.
		if _, err := tx.Exec(ctx, `UPDATE users SET is_active = false
			WHERE id IN (SELECT user_id FROM service_tokens WHERE workspace_id = $1)`, wsID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, wsID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE workspace_deletions SET completed_at = now() WHERE workspace_id = $1 AND completed_at IS NULL`, wsID)
		return err
	})
	return err == nil, err
}
