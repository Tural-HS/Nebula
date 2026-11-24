package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Item links (ADR-0028): typed links between two items of one workspace, in
// any of its projects. A link is visible only to people who can see both
// items (ADR-0013); everything here filters the other side by the viewer, so
// a hidden item never shows up, not even as a count.

// Stored link types (item_links.type).
const (
	LinkBlocks     = "blocks"
	LinkRelates    = "relates"
	LinkDuplicates = "duplicates"
)

// Link types as seen from one item. The inverse names exist only in the API:
// "A is blocked by B" is stored as "B blocks A".
const (
	LinkBlockedBy    = "blocked_by"
	LinkDuplicatedBy = "duplicated_by"
)

// MaxLinksPerItem caps the links of one item (counting both sides and links
// to items the caller cannot see).
const MaxLinksPerItem = 100

// LinkTypes lists the directional link types the API accepts, in display order.
var LinkTypes = []string{LinkBlocks, LinkBlockedBy, LinkRelates, LinkDuplicates, LinkDuplicatedBy}

// linkDirection maps a directional type to the stored type and whether the
// item it is said of is the stored source.
func linkDirection(t string) (stored string, isSource bool, ok bool) {
	switch t {
	case LinkBlocks, LinkRelates, LinkDuplicates:
		return t, true, true
	case LinkBlockedBy:
		return LinkBlocks, false, true
	case LinkDuplicatedBy:
		return LinkDuplicates, false, true
	}
	return "", false, false
}

// seenFrom is the directional type of a stored link as seen from one side.
func seenFrom(stored string, isSource bool) string {
	if isSource {
		return stored
	}
	switch stored {
	case LinkBlocks:
		return LinkBlockedBy
	case LinkDuplicates:
		return LinkDuplicatedBy
	}
	return stored // relates is symmetric
}

// LinkedItem is the other side of a link.
type LinkedItem struct {
	Key        string `json:"key"`
	ProjectKey string `json:"project_key"`
	Number     int64  `json:"number"`
	Title      string `json:"title"`
	Status     Status `json:"status"`
}

// ItemLink is a link as seen from one item: Type is directional ("blocked_by").
type ItemLink struct {
	ID        uuid.UUID  `json:"id"`
	Type      string     `json:"type"`
	Item      LinkedItem `json:"item"`
	CreatedAt time.Time  `json:"created_at"`
}

// visibleProjectFor is the ADR-0013 predicate on a projects row aliased pa for
// a user given by a parameter ($user) and an admin flag ($admin), both SQL
// expressions. Keep in sync with canSeeProject.
func visibleProjectFor(pa, user, admin string) string {
	return fmt.Sprintf(`(%[1]s.visibility = 'workspace' OR %[3]s
		OR EXISTS (SELECT 1 FROM project_members pm WHERE pm.project_id = %[1]s.id AND pm.user_id = %[2]s))`, pa, user, admin)
}

const linkSelect = `SELECT l.id, l.type, l.source_id = $2, o.number, op.key, o.title, os.id, os.name, os.category, l.created_at
	FROM item_links l
	JOIN items o ON o.id = CASE WHEN l.source_id = $2 THEN l.target_id ELSE l.source_id END AND o.deleted_at IS NULL
	JOIN projects op ON op.id = o.project_id
	JOIN statuses os ON os.id = o.status_id
	WHERE l.workspace_id = $1 AND (l.source_id = $2 OR l.target_id = $2)
	  AND `

func scanLink(r pgx.Row) (ItemLink, error) {
	var l ItemLink
	var stored string
	var isSource bool
	err := r.Scan(&l.ID, &stored, &isSource, &l.Item.Number, &l.Item.ProjectKey, &l.Item.Title,
		&l.Item.Status.ID, &l.Item.Status.Name, &l.Item.Status.Category, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ItemLink{}, ErrNotFound
	}
	l.Type = seenFrom(stored, isSource)
	l.Item.Key = itemKey(l.Item.ProjectKey, l.Item.Number)
	return l, err
}

// ListItemLinks returns the links of an item whose other side the viewer can
// see, oldest first. At most MaxLinksPerItem exist, so there is no paging.
func (s *Store) ListItemLinks(ctx context.Context, wsID, projectID uuid.UUID, number int64, v Viewer) ([]ItemLink, error) {
	item, err := liveItem(ctx, s.pool, wsID, projectID, number)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, linkSelect+visibleProjectFor("op", "$3", "$4")+` ORDER BY l.created_at, l.id`,
		wsID, item.id, v.UserID, RoleAtLeast(v.Role, RoleAdmin))
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ItemLink, error) { return scanLink(r) })
	if out == nil {
		out = []ItemLink{}
	}
	return out, err
}

// AddLinkParams describe a new link from the item (ProjectID, Number).
type AddLinkParams struct {
	WorkspaceID uuid.UUID
	ProjectID   uuid.UUID
	Number      int64
	Viewer      Viewer // the actor; must be able to see the target
	Type        string // directional, one of LinkTypes
	Target      string // item key, e.g. "QA-12"
}

// parseItemKey splits "QA-12" (case-insensitive project key).
func parseItemKey(s string) (string, int64, bool) {
	m := itemKeyRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", 0, false
	}
	n, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return strings.ToUpper(m[1]), n, true
}

// AddItemLink links two items and records link.added on both. The target is
// resolved by key in the same workspace; an item the actor cannot see is
// reported exactly like one that does not exist.
func (s *Store) AddItemLink(ctx context.Context, p AddLinkParams) (ItemLink, error) {
	stored, isSource, ok := linkDirection(p.Type)
	if !ok {
		return ItemLink{}, invalid("type", "must be one of %s", strings.Join(LinkTypes, ", "))
	}
	tKey, tNum, ok := parseItemKey(p.Target)
	if !ok {
		return ItemLink{}, invalid("target", "must be an item key, e.g. OPS-12")
	}
	var link ItemLink
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := liveItem(ctx, tx, p.WorkspaceID, p.ProjectID, p.Number)
		if err != nil {
			return err
		}
		var target uuid.UUID
		err = tx.QueryRow(ctx, `SELECT i.id FROM items i JOIN projects op ON op.id = i.project_id
			WHERE i.workspace_id = $1 AND op.workspace_id = $1 AND op.key = $2 AND i.number = $3 AND i.deleted_at IS NULL
			  AND `+visibleProjectFor("op", "$4", "$5"),
			p.WorkspaceID, tKey, tNum, p.Viewer.UserID, RoleAtLeast(p.Viewer.Role, RoleAdmin)).Scan(&target)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("target", "no such item")
		}
		if err != nil {
			return err
		}
		if target == item.id {
			return invalid("target", "an item cannot be linked to itself")
		}
		// Lock both items (in one order, so two opposite links cannot
		// deadlock): neither can be deleted meanwhile (deleteItemTx locks
		// FOR UPDATE too), and concurrent links see each other's counts.
		ids := []uuid.UUID{item.id, target}
		slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		rows, err := tx.Query(ctx, `SELECT id FROM items WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL
			ORDER BY id::text FOR UPDATE`, []string{ids[0].String(), ids[1].String()})
		if err != nil {
			return err
		}
		locked, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		if len(locked) != 2 {
			return invalid("target", "no such item")
		}
		var full bool
		if err := tx.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM item_links WHERE source_id = $1 OR target_id = $1) >= $3 OR
				(SELECT count(*) FROM item_links WHERE source_id = $2 OR target_id = $2) >= $3`,
			item.id, target, MaxLinksPerItem).Scan(&full); err != nil {
			return err
		}
		if full {
			return invalid("target", "an item can have at most %d links", MaxLinksPerItem)
		}
		src, dst := item.id, target
		if !isSource {
			src, dst = target, item.id
		}
		id := newID()
		if _, err := tx.Exec(ctx, `INSERT INTO item_links (id, workspace_id, source_id, target_id, type, created_by)
			VALUES ($1, $2, $3, $4, $5, $6)`, id, p.WorkspaceID, src, dst, stored, p.Viewer.UserID); err != nil {
			if isUniqueViolation(err, "item_links_pair_key") {
				return &ConflictError{Code: "link_exists", Message: "these items are already linked with this type"}
			}
			return err
		}
		if err := linkEvents(ctx, tx, p.WorkspaceID, p.Viewer.UserID, "link.added", id, stored, src, dst, nil); err != nil {
			return err
		}
		link, err = scanLink(tx.QueryRow(ctx, linkSelect+`l.id = $3`, p.WorkspaceID, item.id, id))
		return err
	})
	return link, err
}

// linkEvents records one event per side of a link, each with the type as
// seen from that side, the other item's ID and whether this side is the
// stored source (someone watching both items is notified once, from the
// source side). Keys and titles are not
// copied into the payload: events reach webhooks and feeds of people who
// may not see the other item; feeds resolve them per viewer (ItemActivity).
func linkEvents(ctx context.Context, tx pgx.Tx, wsID, actorID uuid.UUID, action string, linkID uuid.UUID,
	stored string, src, dst uuid.UUID, extra map[string]any) error {
	for _, side := range []struct {
		item, other uuid.UUID
		isSource    bool
	}{{src, dst, true}, {dst, src, false}} {
		payload := map[string]any{"link_id": linkID, "type": seenFrom(stored, side.isSource), "other_item_id": side.other,
			"source": side.isSource}
		for k, v := range extra {
			payload[k] = v
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "item", side.item, action, payload); err != nil {
			return err
		}
	}
	return nil
}

// RemoveItemLink deletes a link of the item. A link whose other side the
// viewer cannot see is not found.
func (s *Store) RemoveItemLink(ctx context.Context, wsID, projectID uuid.UUID, number int64, linkID uuid.UUID, v Viewer) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := liveItem(ctx, tx, wsID, projectID, number)
		if err != nil {
			return err
		}
		if _, err := scanLink(tx.QueryRow(ctx, linkSelect+`l.id = $3 AND `+visibleProjectFor("op", "$4", "$5"),
			wsID, item.id, linkID, v.UserID, RoleAtLeast(v.Role, RoleAdmin))); err != nil {
			return err
		}
		var stored string
		var src, dst uuid.UUID
		err = tx.QueryRow(ctx, `DELETE FROM item_links WHERE id = $1 RETURNING type, source_id, target_id`, linkID).
			Scan(&stored, &src, &dst)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound // removed concurrently
		}
		if err != nil {
			return err
		}
		return linkEvents(ctx, tx, wsID, v.UserID, "link.removed", linkID, stored, src, dst, nil)
	})
}

// removeItemLinks deletes every link of a (just soft-deleted) item and
// records link.removed on the other, live side.
func removeItemLinks(ctx context.Context, tx pgx.Tx, wsID, actorID, itemID uuid.UUID) error {
	rows, err := tx.Query(ctx, `DELETE FROM item_links WHERE source_id = $1 OR target_id = $1
		RETURNING id, type, source_id, target_id`, itemID)
	if err != nil {
		return err
	}
	type gone struct {
		id, src, dst uuid.UUID
		stored       string
	}
	links, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (gone, error) {
		var g gone
		return g, r.Scan(&g.id, &g.stored, &g.src, &g.dst)
	})
	if err != nil {
		return err
	}
	for _, l := range links {
		other, isSource := l.dst, false // the other side is the target: seen from it, not the source
		if l.dst == itemID {
			other, isSource = l.src, true
		}
		var live bool
		if err := tx.QueryRow(ctx, `SELECT deleted_at IS NULL FROM items WHERE id = $1`, other).Scan(&live); err != nil {
			return err
		}
		if !live {
			continue
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "item", other, "link.removed", map[string]any{
			"link_id": l.id, "type": seenFrom(l.stored, isSource), "other_item_id": itemID, "reason": "item_deleted",
		}); err != nil {
			return err
		}
	}
	return nil
}

// blockedItems returns which of the given items have an open blocker (a
// "blocks" link from a live todo/in-progress item) the viewer can see.
// One query per page, whatever its size.
func blockedItems(ctx context.Context, q querier, wsID uuid.UUID, ids []uuid.UUID, v Viewer) (map[uuid.UUID]bool, error) {
	out := map[uuid.UUID]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	rows, err := q.Query(ctx, `SELECT DISTINCT l.target_id FROM item_links l
		JOIN items b ON b.id = l.source_id AND b.deleted_at IS NULL
		JOIN statuses bs ON bs.id = b.status_id AND bs.category IN ('todo', 'in_progress')
		JOIN projects op ON op.id = b.project_id
		WHERE l.workspace_id = $1 AND l.type = 'blocks' AND l.target_id = ANY($2::uuid[]) AND `+
		visibleProjectFor("op", "$3", "$4"), wsID, strs, v.UserID, RoleAtLeast(v.Role, RoleAdmin))
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	for _, id := range list {
		out[id] = true
	}
	return out, err
}

// blockedFilter is the ?blocked=true condition on items i; user and admin
// are the SQL parameters of the viewer.
func blockedFilter(user, admin string) string {
	return `EXISTS (SELECT 1 FROM item_links l
		JOIN items b ON b.id = l.source_id AND b.deleted_at IS NULL
		JOIN statuses bs ON bs.id = b.status_id AND bs.category IN ('todo', 'in_progress')
		JOIN projects op ON op.id = b.project_id
		WHERE l.target_id = i.id AND l.type = 'blocks' AND ` + visibleProjectFor("op", user, admin) + `)`
}
