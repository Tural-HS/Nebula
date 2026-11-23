package store

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	maxCommentLen         = 20_000
	maxMentionsPerComment = 20
	maxCommentPage        = 100
)

// MentionRe matches mention tokens: <@user-uuid>. IDs, not names, because
// display names are neither unique nor stable.
var MentionRe = regexp.MustCompile(`<@([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})>`)

// CommentAuthor is the public view of a comment's author.
type CommentAuthor struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
}

// Comment is a message on an item. Deleted comments stay in threads as
// tombstones (Body nil) so replies and history keep their context.
type Comment struct {
	ID        uuid.UUID      `json:"id"`
	ItemID    uuid.UUID      `json:"item_id"`
	Author    *CommentAuthor `json:"author"`
	Body      *string        `json:"body"`
	Mentions  []uuid.UUID    `json:"mentions"`
	Edited    bool           `json:"edited"`
	Deleted   bool           `json:"deleted"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// parseMentions returns distinct mentioned user IDs in order of appearance.
func parseMentions(body string) []uuid.UUID {
	var out []uuid.UUID
	for _, m := range MentionRe.FindAllStringSubmatch(body, -1) {
		id, err := uuid.Parse(m[1])
		if err == nil && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func validateCommentBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if !utf8.ValidString(body) {
		return "", invalid("body", "must be valid UTF-8")
	}
	n := utf8.RuneCountInString(body)
	if n == 0 {
		return "", invalid("body", "is required")
	}
	if n > maxCommentLen {
		return "", invalid("body", "must be at most %d characters", maxCommentLen)
	}
	return body, nil
}

type mentioned struct {
	id    uuid.UUID
	name  string
	email string
}

// resolveMentions keeps only active workspace members who can see the
// project (ADR-0013); anyone else is ignored silently, so mentions can neither
// probe who exists nor email a private project's content to outsiders.
func resolveMentions(ctx context.Context, q querier, wsID, projectID uuid.UUID, ids []uuid.UUID) ([]mentioned, error) {
	if len(ids) > maxMentionsPerComment {
		return nil, invalid("body", "at most %d people can be mentioned in one comment", maxMentionsPerComment)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	rows, err := q.Query(ctx, `SELECT u.id, u.display_name, u.email FROM users u
		JOIN workspace_members m ON m.user_id = u.id
		JOIN projects p ON p.workspace_id = m.workspace_id AND p.id = $3
		WHERE m.workspace_id = $1 AND u.is_active AND u.id = ANY($2::uuid[])
		  AND (p.visibility = 'workspace' OR m.role IN ('admin', 'owner')
		       OR EXISTS (SELECT 1 FROM project_members pm WHERE pm.project_id = p.id AND pm.user_id = u.id))`, wsID, strs, projectID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (mentioned, error) {
		var m mentioned
		return m, r.Scan(&m.id, &m.name, &m.email)
	})
}

// renderMentions replaces tokens with @Name for plain-text contexts (email).
func renderMentions(body string, names map[uuid.UUID]string) string {
	return MentionRe.ReplaceAllStringFunc(body, func(tok string) string {
		id, err := uuid.Parse(MentionRe.FindStringSubmatch(tok)[1])
		if name, ok := names[id]; err == nil && ok {
			return "@" + name
		}
		return "@?"
	})
}

func excerpt(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// itemRef is the minimum needed to attach a comment to a live item.
type itemRef struct {
	id  uuid.UUID
	key string
}

func liveItem(ctx context.Context, q querier, wsID, projectID uuid.UUID, number int64) (itemRef, error) {
	var r itemRef
	var projectKey string
	err := q.QueryRow(ctx, `SELECT i.id, p.key FROM items i JOIN projects p ON p.id = i.project_id
		WHERE i.workspace_id = $1 AND i.project_id = $2 AND i.number = $3 AND i.deleted_at IS NULL`,
		wsID, projectID, number).Scan(&r.id, &projectKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	r.key = fmt.Sprintf("%s-%d", projectKey, number)
	return r, err
}

// notifyMentions queues one email per newly mentioned member (never the author)
// in the caller's transaction, so notifications exist iff the comment does.
func notifyMentions(ctx context.Context, q querier, people []mentioned, authorID uuid.UUID, authorName string,
	item itemRef, commentID uuid.UUID, body string) error {
	names := map[uuid.UUID]string{}
	for _, p := range people {
		names[p.id] = p.name
	}
	text := excerpt(renderMentions(body, names), 1000)
	for _, p := range people {
		if p.id == authorID {
			continue
		}
		payload := map[string]any{
			"to":      []string{p.email},
			"subject": fmt.Sprintf("[%s] %s mentioned you", item.key, authorName),
			"body":    fmt.Sprintf("%s mentioned you on %s:\n\n%s\n", authorName, item.key, text),
		}
		if err := enqueueJob(ctx, q, JobSendEmail, payload, 8, fmt.Sprintf("mn:%s:%s", commentID, p.id)); err != nil {
			return err
		}
	}
	return nil
}

func authorName(ctx context.Context, q querier, id uuid.UUID) (string, error) {
	var name string
	err := q.QueryRow(ctx, `SELECT display_name FROM users WHERE id = $1`, id).Scan(&name)
	return name, err
}

const commentSelect = `SELECT c.id, c.item_id, c.author_id, u.display_name, c.body, c.created_at, c.updated_at, c.deleted_at,
	COALESCE((SELECT array_agg(cm.user_id ORDER BY cm.user_id) FROM comment_mentions cm WHERE cm.comment_id = c.id), '{}')
	FROM comments c LEFT JOIN users u ON u.id = c.author_id`

func scanComment(row pgx.Row) (Comment, error) {
	var c Comment
	var authorID *uuid.UUID
	var authorName *string
	var body string
	var deletedAt *time.Time
	err := row.Scan(&c.ID, &c.ItemID, &authorID, &authorName, &body, &c.CreatedAt, &c.UpdatedAt, &deletedAt, &c.Mentions)
	if errors.Is(err, pgx.ErrNoRows) {
		return Comment{}, ErrNotFound
	}
	if err != nil {
		return Comment{}, err
	}
	if authorID != nil && authorName != nil {
		c.Author = &CommentAuthor{ID: *authorID, DisplayName: *authorName}
	}
	c.Deleted = deletedAt != nil
	if !c.Deleted {
		c.Body = &body
	} else {
		c.Mentions = []uuid.UUID{}
	}
	c.Edited = !c.Deleted && c.UpdatedAt.Sub(c.CreatedAt) > time.Second
	return c, nil
}

// CommentCursor is the keyset position (created_at, id) of the last comment seen.
type CommentCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// EncodeCommentCursor makes an opaque cursor string.
func EncodeCommentCursor(c Comment) string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()))
}

// DecodeCommentCursor parses a cursor made by EncodeCommentCursor.
func DecodeCommentCursor(s string) (CommentCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return CommentCursor{}, err
	}
	ts, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return CommentCursor{}, errors.New("invalid cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return CommentCursor{}, err
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return CommentCursor{}, err
	}
	return CommentCursor{CreatedAt: t, ID: u}, nil
}

// ListComments returns an item's comments oldest-first, after the cursor.
func (s *Store) ListComments(ctx context.Context, wsID, projectID uuid.UUID, number int64, after *CommentCursor, limit int) ([]Comment, string, error) {
	item, err := liveItem(ctx, s.pool, wsID, projectID, number)
	if err != nil {
		return nil, "", err
	}
	limit = min(max(limit, 1), maxCommentPage)
	args := []any{wsID, item.id, limit + 1}
	cond := ""
	if after != nil {
		cond = ` AND (c.created_at, c.id) > ($4, $5)`
		args = append(args, after.CreatedAt, after.ID)
	}
	rows, err := s.pool.Query(ctx, commentSelect+` WHERE c.workspace_id = $1 AND c.item_id = $2`+cond+
		` ORDER BY c.created_at, c.id LIMIT $3`, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = EncodeCommentCursor(out[len(out)-1])
	}
	return out, next, nil
}

// CreateComment adds a comment and notifies mentioned members.
func (s *Store) CreateComment(ctx context.Context, wsID, projectID uuid.UUID, number int64, actorID uuid.UUID, body string) (Comment, error) {
	body, err := validateCommentBody(body)
	if err != nil {
		return Comment{}, err
	}
	var c Comment
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := liveItem(ctx, tx, wsID, projectID, number)
		if err != nil {
			return err
		}
		people, err := resolveMentions(ctx, tx, wsID, projectID, parseMentions(body))
		if err != nil {
			return err
		}
		id := newID()
		if _, err := tx.Exec(ctx, `INSERT INTO comments (id, workspace_id, item_id, author_id, body) VALUES ($1, $2, $3, $4, $5)`,
			id, wsID, item.id, actorID, body); err != nil {
			return err
		}
		ids := make([]uuid.UUID, 0, len(people))
		for _, p := range people {
			if _, err := tx.Exec(ctx, `INSERT INTO comment_mentions (comment_id, user_id) VALUES ($1, $2)`, id, p.id); err != nil {
				return err
			}
			ids = append(ids, p.id)
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "item", item.id, "comment.created",
			map[string]any{"comment_id": id, "excerpt": excerpt(body, 200), "mentions": ids}); err != nil {
			return err
		}
		name, err := authorName(ctx, tx, actorID)
		if err != nil {
			return err
		}
		if err := notifyMentions(ctx, tx, people, actorID, name, item, id, body); err != nil {
			return err
		}
		c, err = scanComment(tx.QueryRow(ctx, commentSelect+` WHERE c.id = $1`, id))
		return err
	})
	return c, err
}

// commentForUpdate locks a live comment that belongs to the given item.
func commentForUpdate(ctx context.Context, tx pgx.Tx, wsID uuid.UUID, item itemRef, commentID uuid.UUID) (authorID *uuid.UUID, body string, err error) {
	err = tx.QueryRow(ctx, `SELECT author_id, body FROM comments
		WHERE id = $1 AND workspace_id = $2 AND item_id = $3 AND deleted_at IS NULL FOR UPDATE`,
		commentID, wsID, item.id).Scan(&authorID, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return authorID, body, err
}

// UpdateComment lets the author edit a comment. Only newly added mentions are notified.
func (s *Store) UpdateComment(ctx context.Context, wsID, projectID uuid.UUID, number int64, commentID, actorID uuid.UUID, body string) (Comment, error) {
	body, err := validateCommentBody(body)
	if err != nil {
		return Comment{}, err
	}
	var c Comment
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := liveItem(ctx, tx, wsID, projectID, number)
		if err != nil {
			return err
		}
		author, old, err := commentForUpdate(ctx, tx, wsID, item, commentID)
		if err != nil {
			return err
		}
		if author == nil || *author != actorID {
			return ErrForbidden // nobody edits someone else's words, not even admins
		}
		if body != old {
			people, err := resolveMentions(ctx, tx, wsID, projectID, parseMentions(body))
			if err != nil {
				return err
			}
			rows, err := tx.Query(ctx, `SELECT user_id FROM comment_mentions WHERE comment_id = $1`, commentID)
			if err != nil {
				return err
			}
			before, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
			if err != nil {
				return err
			}
			var added []mentioned
			addedIDs := []uuid.UUID{}
			if _, err := tx.Exec(ctx, `DELETE FROM comment_mentions WHERE comment_id = $1`, commentID); err != nil {
				return err
			}
			for _, p := range people {
				if _, err := tx.Exec(ctx, `INSERT INTO comment_mentions (comment_id, user_id) VALUES ($1, $2)`, commentID, p.id); err != nil {
					return err
				}
				if !slices.Contains(before, p.id) {
					added = append(added, p)
					addedIDs = append(addedIDs, p.id)
				}
			}
			if _, err := tx.Exec(ctx, `UPDATE comments SET body = $1 WHERE id = $2`, body, commentID); err != nil {
				return err
			}
			if err := appendEvent(ctx, tx, wsID, actorID, "item", item.id, "comment.updated",
				map[string]any{"comment_id": commentID, "excerpt": excerpt(body, 200), "mentions_added": addedIDs}); err != nil {
				return err
			}
			name, err := authorName(ctx, tx, actorID)
			if err != nil {
				return err
			}
			if err := notifyMentions(ctx, tx, added, actorID, name, item, commentID, body); err != nil {
				return err
			}
		}
		c, err = scanComment(tx.QueryRow(ctx, commentSelect+` WHERE c.id = $1`, commentID))
		return err
	})
	return c, err
}

// DeleteComment soft-deletes a comment. Authors can delete their own; admins any.
// The text is kept for audit but never returned by the API again.
func (s *Store) DeleteComment(ctx context.Context, wsID, projectID uuid.UUID, number int64, commentID, actorID uuid.UUID, actorRole string) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := liveItem(ctx, tx, wsID, projectID, number)
		if err != nil {
			return err
		}
		author, _, err := commentForUpdate(ctx, tx, wsID, item, commentID)
		if err != nil {
			return err
		}
		if (author == nil || *author != actorID) && !RoleAtLeast(actorRole, RoleAdmin) {
			return ErrForbidden
		}
		if _, err := tx.Exec(ctx, `UPDATE comments SET deleted_at = now() WHERE id = $1`, commentID); err != nil {
			return err
		}
		if err := deleteCommentAttachments(ctx, tx, wsID, actorID, item, commentID); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "item", item.id, "comment.deleted", map[string]any{"comment_id": commentID})
	})
}
