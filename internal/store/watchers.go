package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Watchers (ADR-0028): people who get in-app notifications about an item's
// changes. Rows are kept when someone loses access to a private project;
// listing and notifying check access again, so they simply go quiet.

// MaxWatchersPerItem caps the watchers of one item, and with it the fan-out
// of one event (ADR-0017: one batched insert per event).
const MaxWatchersPerItem = 100

// Watcher is one person watching an item.
type Watcher struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
}

// Watchers is an item's watcher list as seen by one user.
type Watchers struct {
	Watching bool      `json:"watching"` // the viewer watches the item
	Count    int       `json:"count"`
	Watchers []Watcher `json:"watchers"`
}

// ListWatchers returns the active watchers who can still see the item, by name.
func (s *Store) ListWatchers(ctx context.Context, wsID, projectID uuid.UUID, number int64, viewerID uuid.UUID) (Watchers, error) {
	item, err := liveItem(ctx, s.pool, wsID, projectID, number)
	if err != nil {
		return Watchers{}, err
	}
	return listWatchers(ctx, s.pool, wsID, projectID, item.id, viewerID)
}

func listWatchers(ctx context.Context, q querier, wsID, projectID, itemID, viewerID uuid.UUID) (Watchers, error) {
	rows, err := q.Query(ctx, `SELECT u.id, u.display_name FROM item_watchers iw
		JOIN users u ON u.id = iw.user_id AND u.is_active
		JOIN workspace_members m ON m.workspace_id = iw.workspace_id AND m.user_id = iw.user_id
		JOIN projects p ON p.workspace_id = iw.workspace_id AND p.id = $3
		WHERE iw.workspace_id = $1 AND iw.item_id = $2 AND `+visibleProjectFor("p", "iw.user_id", "m.role IN ('admin', 'owner')")+`
		ORDER BY u.display_name, u.id`, wsID, itemID, projectID)
	if err != nil {
		return Watchers{}, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Watcher, error) {
		var w Watcher
		return w, r.Scan(&w.ID, &w.DisplayName)
	})
	if err != nil {
		return Watchers{}, err
	}
	out := Watchers{Count: len(list), Watchers: list}
	if out.Watchers == nil {
		out.Watchers = []Watcher{}
	}
	for _, w := range list {
		if w.ID == viewerID {
			out.Watching = true
		}
	}
	return out, nil
}

// SetWatching makes userID watch (on=true) or stop watching an item, as
// actorID. Callers check that only admins change someone else's watching.
// The user must be an active member who can see the project. Changes are
// recorded as watcher.added / watcher.removed; a no-op records nothing.
func (s *Store) SetWatching(ctx context.Context, wsID, projectID uuid.UUID, number int64, actorID, userID uuid.UUID, on bool) (Watchers, error) {
	var out Watchers
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := liveItem(ctx, tx, wsID, projectID, number)
		if err != nil {
			return err
		}
		// Serialize watcher changes of one item so the cap holds.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM items WHERE id = $1 FOR NO KEY UPDATE`, item.id); err != nil {
			return err
		}
		action := "watcher.removed"
		var changed bool
		if on {
			ok, err := canSeeProject(ctx, tx, wsID, projectID, userID)
			if err != nil {
				return err
			}
			var active bool
			if ok {
				if err := tx.QueryRow(ctx, `SELECT is_active FROM users WHERE id = $1`, userID).Scan(&active); err != nil &&
					!errors.Is(err, pgx.ErrNoRows) {
					return err
				}
			}
			if !ok || !active {
				return invalid("user_id", "user is not a member of this workspace or cannot see this project")
			}
			var n int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM item_watchers WHERE item_id = $1`, item.id).Scan(&n); err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `INSERT INTO item_watchers (workspace_id, item_id, user_id)
				SELECT $1, $2, $3 WHERE $4::int < $5::int
				ON CONFLICT (item_id, user_id) DO NOTHING`, wsID, item.id, userID, n, MaxWatchersPerItem)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				var already bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM item_watchers WHERE item_id = $1 AND user_id = $2)`,
					item.id, userID).Scan(&already); err != nil {
					return err
				}
				if !already {
					return invalid("user_id", "an item can have at most %d watchers", MaxWatchersPerItem)
				}
			}
			changed, action = tag.RowsAffected() > 0, "watcher.added"
		} else {
			tag, err := tx.Exec(ctx, `DELETE FROM item_watchers WHERE item_id = $1 AND user_id = $2`, item.id, userID)
			if err != nil {
				return err
			}
			changed = tag.RowsAffected() > 0
		}
		if changed {
			if err := appendEvent(ctx, tx, wsID, actorID, "item", item.id, action, map[string]any{"user_id": userID}); err != nil {
				return err
			}
		}
		out, err = listWatchers(ctx, tx, wsID, projectID, item.id, actorID)
		return err
	})
	return out, err
}

// autoWatch adds watchers as a side effect of their own involvement (ADR-0028):
// reporter on create, assignee on assignment, commenters, mentioned people.
// It records no event (the causing event already says it), ignores people
// who are not workspace members, and stops at MaxWatchersPerItem (the count
// is read once per statement, so a few concurrent additions may pass it).
func autoWatch(ctx context.Context, q querier, wsID, itemID uuid.UUID, users ...*uuid.UUID) error {
	var ids []string
	for _, u := range users {
		if u != nil && *u != uuid.Nil {
			ids = append(ids, u.String())
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := q.Exec(ctx, `INSERT INTO item_watchers (workspace_id, item_id, user_id)
		SELECT $1, $2, m.user_id FROM workspace_members m
		WHERE m.workspace_id = $1 AND m.user_id = ANY($3::uuid[])
		  AND (SELECT count(*) FROM item_watchers WHERE item_id = $2) < $4
		ON CONFLICT (item_id, user_id) DO NOTHING`, wsID, itemID, ids, MaxWatchersPerItem)
	return err
}
