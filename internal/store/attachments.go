package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Attachment is a file on an item, optionally posted with one of its comments
// (ADR-0022). StorageKey is internal and never leaves the server.
type Attachment struct {
	ID          uuid.UUID      `json:"id"`
	ItemID      uuid.UUID      `json:"item_id"`
	CommentID   *uuid.UUID     `json:"comment_id"`
	Uploader    *CommentAuthor `json:"uploader"`
	Filename    string         `json:"filename"`
	ContentType string         `json:"content_type"`
	Size        int64          `json:"size"`
	SHA256      string         `json:"sha256"`
	CreatedAt   time.Time      `json:"created_at"`
	StorageKey  string         `json:"-"`
}

// QuotaError means an upload would exceed a storage limit.
type QuotaError struct{ Message string }

func (e *QuotaError) Error() string { return e.Message }

// AttachmentLimits are enforced in the insert transaction.
type AttachmentLimits struct {
	WorkspaceQuotaBytes int64 // total bytes per workspace; 0 = unlimited
	MaxPerItem          int   // files per item; 0 = unlimited
}

// uploadGrace is how long a reserved key may stay unused before its blob is
// treated as orphaned (an upload that crashed between blob and row).
const uploadGrace = time.Hour

const attachmentSelect = `SELECT a.id, a.item_id, a.comment_id, a.uploader_id, u.display_name, a.filename,
	a.content_type, a.size_bytes, encode(a.sha256, 'hex'), a.created_at, a.storage_key
	FROM attachments a LEFT JOIN users u ON u.id = a.uploader_id`

func scanAttachment(row pgx.Row) (Attachment, error) {
	var a Attachment
	var uploaderID *uuid.UUID
	var uploaderName *string
	err := row.Scan(&a.ID, &a.ItemID, &a.CommentID, &uploaderID, &uploaderName, &a.Filename,
		&a.ContentType, &a.Size, &a.SHA256, &a.CreatedAt, &a.StorageKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	if uploaderID != nil && uploaderName != nil {
		a.Uploader = &CommentAuthor{ID: *uploaderID, DisplayName: *uploaderName}
	}
	return a, err
}

// AttachmentUsage returns the bytes stored by a workspace.
func (s *Store) AttachmentUsage(ctx context.Context, wsID uuid.UUID) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(sum(size_bytes), 0) FROM attachments WHERE workspace_id = $1`, wsID).Scan(&n)
	return n, err
}

// CheckAttachmentTarget fails fast, before any bytes are stored, when the
// item (or comment) cannot take an upload from actorID.
func (s *Store) CheckAttachmentTarget(ctx context.Context, wsID, projectID uuid.UUID, number int64, commentID *uuid.UUID, actorID uuid.UUID) error {
	item, err := liveItem(ctx, s.pool, wsID, projectID, number)
	if err != nil {
		return err
	}
	return checkCommentTarget(ctx, s.pool, wsID, item, commentID, actorID, false)
}

// checkCommentTarget allows attaching to a live comment of the same item, by
// its author only (nobody adds files to someone else's words).
func checkCommentTarget(ctx context.Context, q querier, wsID uuid.UUID, item itemRef, commentID *uuid.UUID, actorID uuid.UUID, lock bool) error {
	if commentID == nil {
		return nil
	}
	sql := `SELECT author_id FROM comments WHERE id = $1 AND workspace_id = $2 AND item_id = $3 AND deleted_at IS NULL`
	if lock {
		sql += ` FOR SHARE`
	}
	var author *uuid.UUID
	err := q.QueryRow(ctx, sql, *commentID, wsID, item.id).Scan(&author)
	if errors.Is(err, pgx.ErrNoRows) {
		return invalid("comment_id", "is not a comment of this item")
	}
	if err != nil {
		return err
	}
	if author == nil || *author != actorID {
		return ErrForbidden
	}
	return nil
}

// ReserveBlobKey registers a storage key before its blob is written. If the
// upload never commits a row, the worker removes the blob after uploadGrace.
func (s *Store) ReserveBlobKey(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO attachment_blob_purges (storage_key, kind, purge_after) VALUES ($1, 'upload', $2)`,
		key, time.Now().Add(uploadGrace))
	return err
}

// ReleaseBlobKey makes a reserved key purgeable now (the upload failed and
// the caller could not delete the blob itself).
func (s *Store) ReleaseBlobKey(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx, `UPDATE attachment_blob_purges SET purge_after = now() WHERE storage_key = $1`, key)
	return err
}

// CancelBlobKey drops a reservation whose blob was never written.
func (s *Store) CancelBlobKey(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM attachment_blob_purges p WHERE p.storage_key = $1
		AND NOT EXISTS (SELECT 1 FROM attachments a WHERE a.storage_key = p.storage_key)`, key)
	return err
}

// CreateAttachmentParams describes a stored blob to register.
type CreateAttachmentParams struct {
	WorkspaceID, ProjectID uuid.UUID
	Number                 int64
	CommentID              *uuid.UUID
	ActorID                uuid.UUID
	Filename, ContentType  string
	Size                   int64
	SHA256                 []byte
	StorageKey             string
	Limits                 AttachmentLimits
}

// CreateAttachment records an uploaded blob and its audit event. Quotas are
// checked under a per-workspace lock, so parallel uploads cannot overshoot.
func (s *Store) CreateAttachment(ctx context.Context, p CreateAttachmentParams) (Attachment, error) {
	name, err := text("filename", p.Filename, 1, 255)
	if err != nil {
		return Attachment{}, err
	}
	var a Attachment
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := liveItem(ctx, tx, p.WorkspaceID, p.ProjectID, p.Number)
		if err != nil {
			return err
		}
		if err := checkCommentTarget(ctx, tx, p.WorkspaceID, item, p.CommentID, p.ActorID, true); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('nebula.attachments:' || $1::text, 0))`,
			p.WorkspaceID); err != nil {
			return err
		}
		var used, onItem int64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(size_bytes), 0),
				count(*) FILTER (WHERE item_id = $2) FROM attachments WHERE workspace_id = $1`,
			p.WorkspaceID, item.id).Scan(&used, &onItem); err != nil {
			return err
		}
		if q := p.Limits.WorkspaceQuotaBytes; q > 0 && used+p.Size > q {
			return &QuotaError{Message: fmt.Sprintf("the workspace storage quota (%d MiB) is full", q>>20)}
		}
		if m := p.Limits.MaxPerItem; m > 0 && onItem >= int64(m) {
			return &QuotaError{Message: fmt.Sprintf("an item can have at most %d attachments", m)}
		}
		id := newID()
		if _, err := tx.Exec(ctx, `INSERT INTO attachments
			(id, workspace_id, item_id, comment_id, uploader_id, filename, content_type, size_bytes, sha256, storage_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			id, p.WorkspaceID, item.id, p.CommentID, p.ActorID, name, p.ContentType, p.Size, p.SHA256, p.StorageKey); err != nil {
			return err
		}
		// The blob now belongs to a row: it must not be purged.
		if _, err := tx.Exec(ctx, `DELETE FROM attachment_blob_purges WHERE storage_key = $1`, p.StorageKey); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "item", item.id, "attachment.added", map[string]any{
			"attachment_id": id, "filename": name, "content_type": p.ContentType, "size": p.Size, "comment_id": p.CommentID,
		}); err != nil {
			return err
		}
		a, err = scanAttachment(tx.QueryRow(ctx, attachmentSelect+` WHERE a.id = $1`, id))
		return err
	})
	return a, err
}

// ListAttachments returns an item's files, oldest first.
func (s *Store) ListAttachments(ctx context.Context, wsID, projectID uuid.UUID, number int64) ([]Attachment, error) {
	item, err := liveItem(ctx, s.pool, wsID, projectID, number)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, attachmentSelect+` WHERE a.workspace_id = $1 AND a.item_id = $2
		ORDER BY a.created_at, a.id`, wsID, item.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attachment{}
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAttachment returns one file of a live item the caller can see (the
// project was already resolved with the caller's visibility).
func (s *Store) GetAttachment(ctx context.Context, wsID, projectID uuid.UUID, number int64, id uuid.UUID) (Attachment, error) {
	item, err := liveItem(ctx, s.pool, wsID, projectID, number)
	if err != nil {
		return Attachment{}, err
	}
	return scanAttachment(s.pool.QueryRow(ctx, attachmentSelect+` WHERE a.id = $1 AND a.workspace_id = $2 AND a.item_id = $3`,
		id, wsID, item.id))
}

// DeleteAttachment removes a file: its uploader may, admins may delete any.
// The blob is queued for removal by the attachments_queue_purge trigger.
func (s *Store) DeleteAttachment(ctx context.Context, wsID, projectID uuid.UUID, number int64, id, actorID uuid.UUID, actorRole string) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := liveItem(ctx, tx, wsID, projectID, number)
		if err != nil {
			return err
		}
		var uploader *uuid.UUID
		var name string
		err = tx.QueryRow(ctx, `SELECT uploader_id, filename FROM attachments
			WHERE id = $1 AND workspace_id = $2 AND item_id = $3 FOR UPDATE`, id, wsID, item.id).Scan(&uploader, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if (uploader == nil || *uploader != actorID) && !RoleAtLeast(actorRole, RoleAdmin) {
			return ErrForbidden
		}
		if _, err := tx.Exec(ctx, `DELETE FROM attachments WHERE id = $1`, id); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "item", item.id, "attachment.removed",
			map[string]any{"attachment_id": id, "filename": name})
	})
}

// deleteCommentAttachments removes the files posted with a comment that is
// being deleted, in the caller's transaction, with one audit event each.
func deleteCommentAttachments(ctx context.Context, tx pgx.Tx, wsID, actorID uuid.UUID, item itemRef, commentID uuid.UUID) error {
	rows, err := tx.Query(ctx, `DELETE FROM attachments WHERE workspace_id = $1 AND comment_id = $2 RETURNING id, filename`,
		wsID, commentID)
	if err != nil {
		return err
	}
	type gone struct {
		id   uuid.UUID
		name string
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (gone, error) {
		var g gone
		return g, r.Scan(&g.id, &g.name)
	})
	if err != nil {
		return err
	}
	for _, g := range list {
		if err := appendEvent(ctx, tx, wsID, actorID, "item", item.id, "attachment.removed",
			map[string]any{"attachment_id": g.id, "filename": g.name, "comment_id": commentID}); err != nil {
			return err
		}
	}
	return nil
}

// DueBlobPurges returns storage keys whose blobs should be removed now:
// abandoned upload reservations, and deleted attachments older than delay.
// A key still referenced by an attachment row is never returned.
func (s *Store) DueBlobPurges(ctx context.Context, limit int, delay time.Duration) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.storage_key FROM attachment_blob_purges p
		WHERE p.purge_after <= now() - CASE p.kind WHEN 'deleted' THEN make_interval(secs => $2) ELSE interval '0' END
		  AND NOT EXISTS (SELECT 1 FROM attachments a WHERE a.storage_key = p.storage_key)
		ORDER BY p.purge_after LIMIT $1`, limit, delay.Seconds())
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// BlobPurged forgets a key whose blob was removed.
func (s *Store) BlobPurged(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM attachment_blob_purges p WHERE p.storage_key = $1
		AND NOT EXISTS (SELECT 1 FROM attachments a WHERE a.storage_key = p.storage_key)`, key)
	return err
}
