package engine

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/store"
)

// ChatMessage is the body of a Slack-compatible incoming webhook, which
// Mattermost accepts unchanged.
type ChatMessage struct {
	Text     string `json:"text"`
	Username string `json:"username,omitempty"`
	IconURL  string `json:"icon_url,omitempty"`
}

// ChatContext is everything RenderChat shows besides the event itself.
// Lookups that fail leave fields empty; rendering never fails.
type ChatContext struct {
	Actor    string           // actor display name ("" for none)
	Item     *store.ItemLabel // the event's item, if any
	ItemURL  string           // link to the item in the web UI ("" = no link)
	Users    map[uuid.UUID]string
	Statuses map[uuid.UUID]string
	// PublicURL is the web UI base URL, for links to items that are not the
	// event's own entity (digest entries). "" = no links.
	PublicURL string
}

const maxChatText = 3000 // well under Mattermost's 16k and Slack's 40k limits

// RenderChat returns a short Azerbaijani, markdown-safe line (or two) for ev.
// All user-controlled text goes through escapeChat.
func RenderChat(ev store.Event, c ChatContext) string {
	who := actorLabel(ev, c)
	ref, title := c.itemRef()
	var lines []string
	switch ev.Action {
	case store.WebhookPingAction:
		lines = append(lines, fmt.Sprintf("✅ Nebula test mesajı: bu kanal %s iş sahəsinin bildirişlərini alacaq — %s",
			quote(ev.WorkspaceSlug), who))
	case "item.created":
		lines = append(lines, fmt.Sprintf("🆕 %s%s yaradıldı — %s", ref, title, who))
	case "item.deleted":
		lines = append(lines, fmt.Sprintf("🗑️ %s%s silindi — %s", ref, title, who))
	case "item.updated":
		lines = append(lines, c.itemUpdated(ev, ref, title, who)...)
	case "comment.created":
		line := fmt.Sprintf("💬 %s %s%s tapşırığına şərh yazdı", who, ref, title)
		if ex, _ := ev.Payload["excerpt"].(string); ex != "" {
			line += ": " + quote(c.expandMentions(ex))
		}
		lines = append(lines, line)
		if names := c.names(uuids(ev.Payload["mentions"])); len(names) > 0 {
			lines = append(lines, "Qeyd edilənlər: "+strings.Join(names, ", "))
		}
	case "comment.updated":
		lines = append(lines, fmt.Sprintf("💬 %s %s%s tapşırığındakı şərhi redaktə etdi", who, ref, title))
	case "comment.deleted":
		lines = append(lines, fmt.Sprintf("💬 %s %s%s tapşırığındakı şərhi sildi", who, ref, title))
	case "item.overdue":
		if t, _ := ev.Payload["title"].(string); t != "" {
			title = " " + quote(t)
		}
		if ref == "" {
			ref = escapeChat(str(ev.Payload["key"]))
		}
		lines = append(lines, fmt.Sprintf("⏰ %s%s — %s", ref, title, overdueTail(ev.Payload)))
	case "workspace.overdue_digest":
		return c.overdueDigest(ev)
	default:
		subject := ref + title
		if subject == "" {
			subject = escapeChat(ev.EntityType)
		}
		lines = append(lines, fmt.Sprintf("ℹ️ %s: %s — %s", escapeChat(ev.Action), strings.TrimSpace(subject), who))
	}
	return truncate(strings.Join(lines, "\n"), maxChatText)
}

func (c ChatContext) itemUpdated(ev store.Event, ref, title, who string) []string {
	changes, _ := ev.Payload["changes"].(map[string]any)
	// The title as of this event, not as of delivery.
	if snap, ok := ev.Payload["snapshot"].(map[string]any); ok {
		if t, ok := snap["title"].(string); ok && t != "" {
			title = " " + quote(t)
		}
	}
	var lines, other []string
	keys := make([]string, 0, len(changes))
	for k := range changes {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		ch, _ := changes[k].(map[string]any)
		switch k {
		case "status_id":
			from, to := c.status(ch["from"]), c.status(ch["to"])
			lines = append(lines, fmt.Sprintf("🔄 %s%s: %s → %s — %s", ref, title, from, to, who))
		case "assignee_id":
			if id, ok := uuidOf(ch["to"]); ok {
				lines = append(lines, fmt.Sprintf("👤 %s%s: icraçı %s — %s", ref, title, c.user(id), who))
			} else {
				lines = append(lines, fmt.Sprintf("👤 %s%s: icraçı götürüldü — %s", ref, title, who))
			}
		case "title":
			other = append(other, "başlıq")
		case "description":
			other = append(other, "təsvir")
		default:
			other = append(other, escapeChat(strings.TrimPrefix(k, "fields.")))
		}
	}
	if len(other) > 0 {
		lines = append(lines, fmt.Sprintf("✏️ %s%s yeniləndi (%s) — %s", ref, title, strings.Join(other, ", "), who))
	}
	if len(lines) == 0 {
		lines = append(lines, fmt.Sprintf("✏️ %s%s yeniləndi — %s", ref, title, who))
	}
	return lines
}

// overdueTail renders "3 gün gecikir (20.11.2026) — Bob Developer".
func overdueTail(p map[string]any) string {
	days, _ := p["days_overdue"].(float64)
	out := fmt.Sprintf("%d gün gecikir", int64(days))
	if d := azDate(str(p["due_date"])); d != "" {
		out += " (" + d + ")"
	}
	if n := str(p["assignee_name"]); n != "" {
		out += " — " + escapeChat(n)
	} else {
		out += " — icraçı yoxdur"
	}
	return out
}

// overdueDigest renders the daily overdue list, stopping before the message
// limit and saying how many entries were left out.
func (c ChatContext) overdueDigest(ev store.Event) string {
	entries, _ := ev.Payload["items"].([]any)
	total, _ := ev.Payload["total"].(float64)
	shownTotal := max(int(total), len(entries))
	var b strings.Builder
	fmt.Fprintf(&b, "⚠️ Gecikmədə olanlar (%d):", shownTotal)
	shown := 0
	for _, x := range entries {
		it, ok := x.(map[string]any)
		if !ok {
			continue
		}
		key := escapeChat(str(it["key"]))
		if u := c.digestURL(ev.WorkspaceSlug, it); u != "" {
			key = "<" + u + "|" + key + ">"
		}
		line := "\n• " + key
		if t := escapeChat(str(it["title"])); t != "" {
			line += " " + t
		}
		line += " — " + overdueTail(it)
		// Reserve room for the "…və daha N" line.
		if b.Len()+len(line) > maxChatText-40 {
			break
		}
		b.WriteString(line)
		shown++
	}
	if rest := shownTotal - shown; rest > 0 {
		fmt.Fprintf(&b, "\n…və daha %d", rest)
	}
	return b.String()
}

func (c ChatContext) digestURL(wsSlug string, it map[string]any) string {
	project, key := str(it["project"]), str(it["key"])
	n, err := strconv.ParseInt(strings.TrimPrefix(key, project+"-"), 10, 64)
	if project == "" || err != nil {
		return ""
	}
	return ItemURL(c.PublicURL, wsSlug, store.ItemLabel{ProjectKey: project, Number: n})
}

// azDate turns 2026-11-20 into 20.11.2026; anything else gives "".
func azDate(iso string) string {
	if len(iso) != 10 || iso[4] != '-' || iso[7] != '-' {
		return ""
	}
	for i, r := range iso {
		if i != 4 && i != 7 && (r < '0' || r > '9') {
			return ""
		}
	}
	return iso[8:10] + "." + iso[5:7] + "." + iso[0:4]
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// itemRef returns the (linked) item key and the quoted title, each with a
// leading space where needed so callers can concatenate them.
func (c ChatContext) itemRef() (ref, title string) {
	if c.Item == nil {
		return "", ""
	}
	ref = escapeChat(c.Item.Key)
	if c.ItemURL != "" {
		// Slack link syntax; Mattermost converts it to a markdown link. Escaped
		// user text contains no raw < > |, so only our own links match.
		ref = "<" + c.ItemURL + "|" + ref + ">"
	}
	if c.Item.Title != "" {
		title = " " + quote(c.Item.Title)
	}
	return ref, title
}

func actorLabel(ev store.Event, c ChatContext) string {
	switch {
	case c.Actor != "":
		return escapeChat(c.Actor)
	case ev.Source == "automation":
		return "Avtomatlaşdırma"
	default:
		return "Sistem"
	}
}

func (c ChatContext) status(v any) string {
	if id, ok := uuidOf(v); ok {
		if n, ok := c.Statuses[id]; ok {
			return escapeChat(n)
		}
	}
	return "?"
}

func (c ChatContext) user(id uuid.UUID) string {
	if n, ok := c.Users[id]; ok {
		return escapeChat(n)
	}
	return "naməlum istifadəçi"
}

func (c ChatContext) names(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if n, ok := c.Users[id]; ok {
			out = append(out, escapeChat(n))
		}
	}
	return out
}

// expandMentions shows <@uuid> tokens as @Name. escapeChat (applied by quote)
// then defuses the @ so the chat server never pings anyone.
func (c ChatContext) expandMentions(s string) string {
	return store.MentionRe.ReplaceAllStringFunc(s, func(tok string) string {
		id, err := uuid.Parse(tok[2 : len(tok)-1])
		if n, ok := c.Users[id]; err == nil && ok {
			return "@" + n
		}
		return "@?"
	})
}

func quote(s string) string { return "\u201c" + escapeChat(s) + "\u201d" }

// wordJoiner is an invisible character that splits tokens chat servers parse.
const wordJoiner = "\u2060"

// escapeChat makes user-controlled text inert in Slack mrkdwn and Mattermost
// markdown:
//   - & < > become entities (Slack requires it); a word joiner after "&lt;"
//     keeps <!channel>, <!here>, <@U123> and <url|text> from being recognised
//     even by a server that decodes entities before parsing them;
//   - "@" is followed by a word joiner: no @channel, @all, @here or @user;
//   - markdown punctuation that builds links or formatting ([ ] ( ) * _ ~ ` | \)
//     is backslash-escaped (Mattermost honours this; Slack shows the backslash);
//   - control and bidi-override characters and newlines become spaces, so user
//     text can neither start a new block nor visually reorder the message.
func escapeChat(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;" + wordJoiner)
		case r == '>':
			b.WriteString("&gt;")
		case r == '@':
			b.WriteString("@" + wordJoiner)
		case strings.ContainsRune("[]()*_~`|\\", r):
			b.WriteByte('\\')
			b.WriteRune(r)
		case unicode.IsControl(r) || isBidiControl(r) || r == '\u2028' || r == '\u2029':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func isBidiControl(r rune) bool {
	return (r >= '\u202A' && r <= '\u202E') || (r >= '\u2066' && r <= '\u2069') || r == '\u200E' || r == '\u200F' || r == '\u061C'
}

func uuidOf(v any) (uuid.UUID, bool) {
	s, ok := v.(string)
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(s)
	return id, err == nil
}

func uuids(v any) []uuid.UUID {
	list, _ := v.([]any)
	var out []uuid.UUID
	for _, x := range list {
		if id, ok := uuidOf(x); ok {
			out = append(out, id)
		}
	}
	return out
}

// ItemURL links to an item in the web UI: <base>/w/<ws>/p/<project>?item=<n>.
func ItemURL(base, wsSlug string, l store.ItemLabel) string {
	if base == "" || wsSlug == "" || l.ProjectKey == "" {
		return ""
	}
	u := base + "/w/" + url.PathEscape(wsSlug) + "/p/" + url.PathEscape(l.ProjectKey) + "?item=" + strconv.FormatInt(l.Number, 10)
	// Keep the link a single token for both link syntaxes.
	return strings.NewReplacer("|", "%7C", "<", "%3C", ">", "%3E", "(", "%28", ")", "%29", " ", "%20").Replace(u)
}

// chatContext loads names for ev. Lookup errors are returned so delivery is
// retried rather than sending a message full of question marks.
func (e *Engine) chatContext(ctx context.Context, ev store.Event) (ChatContext, error) {
	c := ChatContext{PublicURL: e.PublicURL}
	var users, statuses []uuid.UUID
	if ev.ActorID != nil {
		users = append(users, *ev.ActorID)
	}
	if changes, ok := ev.Payload["changes"].(map[string]any); ok {
		if ch, ok := changes["status_id"].(map[string]any); ok {
			for _, v := range []any{ch["from"], ch["to"]} {
				if id, ok := uuidOf(v); ok {
					statuses = append(statuses, id)
				}
			}
		}
		if ch, ok := changes["assignee_id"].(map[string]any); ok {
			if id, ok := uuidOf(ch["to"]); ok {
				users = append(users, id)
			}
		}
	}
	users = append(users, uuids(ev.Payload["mentions"])...)
	if ex, ok := ev.Payload["excerpt"].(string); ok {
		for _, m := range store.MentionRe.FindAllStringSubmatch(ex, 20) {
			if id, err := uuid.Parse(m[1]); err == nil {
				users = append(users, id)
			}
		}
	}

	var err error
	if c.Users, err = e.Store.DisplayNames(ctx, users); err != nil {
		return c, err
	}
	if c.Statuses, err = e.Store.StatusNames(ctx, ev.WorkspaceID, statuses); err != nil {
		return c, err
	}
	if ev.ActorID != nil {
		c.Actor = c.Users[*ev.ActorID]
	}
	if ev.EntityType == "item" {
		l, err := e.Store.ItemLabelByID(ctx, ev.WorkspaceID, ev.EntityID)
		switch {
		case err == nil:
			c.Item = &l
			if ev.Action != "item.deleted" {
				c.ItemURL = ItemURL(e.PublicURL, ev.WorkspaceSlug, l)
			}
		case !errors.Is(err, store.ErrNotFound):
			return c, err
		}
	}
	return c, nil
}
