package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/store"
)

func TestEscapeChat(t *testing.T) {
	wj := wordJoiner
	tests := map[string]string{
		"plain text":                    "plain text",
		"Başlıq — ünvan":                "Başlıq — ünvan",
		"@channel @all @here":           "@" + wj + "channel @" + wj + "all @" + wj + "here",
		"<!here> <!channel> <@U123>":    "&lt;" + wj + "!here&gt; &lt;" + wj + "!channel&gt; &lt;" + wj + "@" + wj + "U123&gt;",
		"<https://evil.example|Login>":  "&lt;" + wj + "https://evil.example\\|Login&gt;",
		"[click](https://evil.example)": "\\[click\\]\\(https://evil.example\\)",
		"![img](https://x/y.png)":       "!\\[img\\]\\(https://x/y.png\\)",
		"*bold* _it_ ~st~ `code`":       "\\*bold\\* \\_it\\_ \\~st\\~ \\`code\\`",
		"a\\b":                          "a\\\\b",
		"Tom & Jerry":                   "Tom &amp; Jerry",
		"line1\nline2\r\n# heading":     "line1 line2  # heading",
		"abc\u202Egpj.exe":              "abc gpj.exe",
		"  padded  ":                    "padded",
	}
	for in, want := range tests {
		if got := escapeChat(in); got != want {
			t.Errorf("escapeChat(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

// No escaped string may contain a raw token a chat server acts on.
func TestEscapeChatNeutralises(t *testing.T) {
	attacks := []string{
		"<!channel>", "<!here|here>", "<!everyone>", "@channel", "@all", "@here", "@alice",
		"[x](javascript:alert(1))", "<http://evil|safe>", "](http://evil)", "<#C123>",
	}
	for _, a := range attacks {
		got := escapeChat(a)
		for _, bad := range []string{"<!", "<@", "<#", "<http", "](", "@channel", "@all", "@here", "@alice"} {
			if strings.Contains(got, bad) {
				t.Errorf("escapeChat(%q) = %q still contains %q", a, got, bad)
			}
		}
	}
}

func TestRenderChat(t *testing.T) {
	alice, bob, carol := uuid.New(), uuid.New(), uuid.New()
	todo, doing := uuid.New(), uuid.New()
	item := &store.ItemLabel{Key: "QA-12", ProjectKey: "QA", Number: 12, Title: "Başlıq"}
	c := ChatContext{
		Actor: "Alice Developer", Item: item, ItemURL: "https://nebula.example.com/w/acme/p/QA?item=12",
		Users:    map[uuid.UUID]string{alice: "Alice Developer", bob: "Bob", carol: "@channel"},
		Statuses: map[uuid.UUID]string{todo: "To Do", doing: "In Progress"},
	}
	link := "<https://nebula.example.com/w/acme/p/QA?item=12|QA-12>"
	ev := func(action string, payload map[string]any) store.Event {
		return store.Event{Action: action, WorkspaceSlug: "acme", EntityType: "item", ActorID: &alice, Source: "user", Payload: payload}
	}

	tests := []struct {
		name string
		ev   store.Event
		c    ChatContext
		want string
	}{
		{"status change", ev("item.updated", map[string]any{
			"changes":  map[string]any{"status_id": map[string]any{"from": todo.String(), "to": doing.String()}},
			"snapshot": map[string]any{"title": "Başlıq"},
		}), c, "🔄 " + link + " “Başlıq”: To Do → In Progress — Alice Developer"},
		{"moved to done", ev("item.updated", map[string]any{
			"changes":  map[string]any{"status_id": map[string]any{"from": todo.String(), "to": doing.String()}},
			"snapshot": map[string]any{"title": "Başlıq"},
		}), func() ChatContext {
			d := c
			d.StatusCategories = map[uuid.UUID]string{doing: "done"}
			return d
		}(), "✅ " + link + " “Başlıq”: To Do → In Progress — Alice Developer"},
		{"created", ev("item.created", map[string]any{"key": "QA-12"}), c, "🆕 " + link + " “Başlıq” yaradıldı — Alice Developer"},
		{"assigned", ev("item.updated", map[string]any{
			"changes": map[string]any{"assignee_id": map[string]any{"from": nil, "to": bob.String()}},
		}), c, "👤 " + link + " “Başlıq”: icraçı Bob — Alice Developer"},
		{"unassigned", ev("item.updated", map[string]any{
			"changes": map[string]any{"assignee_id": map[string]any{"from": bob.String(), "to": nil}},
		}), c, "👤 " + link + " “Başlıq”: icraçı götürüldü — Alice Developer"},
		{"other fields", ev("item.updated", map[string]any{
			"changes": map[string]any{"title": map[string]any{}, "fields.sev_level": map[string]any{}},
		}), c, "✏️ " + link + " “Başlıq” yeniləndi (sev\\_level, başlıq) — Alice Developer"},
		{"comment with mention", ev("comment.created", map[string]any{
			"excerpt":  fmt.Sprintf("Hey <@%s> and <@%s>, see [x](http://evil)", bob, carol),
			"mentions": []any{bob.String(), carol.String()},
		}), c, "💬 Alice Developer " + link + " “Başlıq” tapşırığına şərh yazdı: “Hey @" + wordJoiner + "Bob and @" + wordJoiner + "@" + wordJoiner +
			"channel, see \\[x\\]\\(http://evil\\)”\nQeyd edilənlər: Bob, @" + wordJoiner + "channel"},
		{"deleted item has no link", ev("item.deleted", nil), ChatContext{Actor: "Alice", Item: item},
			"🗑️ QA-12 “Başlıq” silindi — Alice"},
		{"automation actor", store.Event{Action: "item.created", EntityType: "item", Source: "automation"},
			ChatContext{Item: item}, "🆕 QA-12 “Başlıq” yaradıldı — Avtomatlaşdırma"},
		{"ping", store.Event{Action: store.WebhookPingAction, WorkspaceSlug: "acme", EntityType: "webhook"},
			ChatContext{Actor: "Alice"}, "✅ Nebula test mesajı: bu kanal “acme” iş sahəsinin bildirişlərini alacaq — Alice"},
		{"unknown event", store.Event{Action: "project.created", EntityType: "project"}, ChatContext{},
			"ℹ️ project.created: project — Sistem"},
		{"hostile names", ev("item.created", nil), ChatContext{
			Actor: "<!channel> [boss](http://evil)", Item: &store.ItemLabel{Key: "QA-1", Title: "@all *urgent*"},
		}, "🆕 QA-1 “@" + wordJoiner + "all \\*urgent\\*” yaradıldı — &lt;" + wordJoiner + "!channel&gt; \\[boss\\]\\(http://evil\\)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RenderChat(tc.ev, tc.c); got != tc.want {
				t.Errorf("\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestRenderChatTruncates(t *testing.T) {
	got := RenderChat(store.Event{Action: "item.created", EntityType: "item"},
		ChatContext{Item: &store.ItemLabel{Key: "QA-1", Title: strings.Repeat("ə", 5000)}})
	if len(got) > maxChatText {
		t.Fatalf("len %d > %d", len(got), maxChatText)
	}
}

func TestItemURL(t *testing.T) {
	l := store.ItemLabel{ProjectKey: "QA", Number: 7}
	if got := ItemURL("https://n.example.com", "acme", l); got != "https://n.example.com/w/acme/p/QA?item=7" {
		t.Errorf("got %q", got)
	}
	if got := ItemURL("", "acme", l); got != "" {
		t.Errorf("no base must give no link, got %q", got)
	}
	if got := ItemURL("https://n.example.com/a(b)", "a|b c", l); strings.ContainsAny(got, "|() <>") {
		t.Errorf("link must stay one token: %q", got)
	}
}

func TestRedactURL(t *testing.T) {
	secretURL := "https://chat.example.com/hooks/supersecrettoken"
	err := redactURL(&url.Error{Op: "Post", URL: secretURL, Err: errors.New("connection refused")})
	if strings.Contains(err.Error(), "supersecret") || err.Error() != "Post: connection refused" {
		t.Fatalf("got %q", err)
	}
	plain := errors.New("x")
	if !errors.Is(redactURL(plain), plain) {
		t.Fatal("non-URL errors pass through")
	}
}

// payload decodes JSON exactly as events come out of the database.
func payload(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRenderChatOverdue(t *testing.T) {
	bob := uuid.New()
	ev := store.Event{Action: "item.overdue", EntityType: "item", Source: "system", WorkspaceSlug: "acme", Payload: payload(t, fmt.Sprintf(`{
		"snapshot": {"title": "Başlıq", "status_category": "todo"},
		"key": "QA-12", "title": "Başlıq", "due_date": "2026-11-20", "days_overdue": 3,
		"assignee_id": %q, "assignee_name": "Bob Developer"}`, bob))}
	c := ChatContext{Item: &store.ItemLabel{Key: "QA-12", ProjectKey: "QA", Number: 12, Title: "Başlıq (indi)"},
		ItemURL: "https://n.example.com/w/acme/p/QA?item=12"}
	want := "⏰ <https://n.example.com/w/acme/p/QA?item=12|QA-12> “Başlıq” — 3 gün gecikir (20.11.2026) — Bob Developer"
	if got := RenderChat(ev, c); got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}

	// Unassigned, item no longer found, hostile title.
	ev.Payload = payload(t, `{"snapshot": {}, "key": "QA-13", "title": "@all [x](http://evil)", "due_date": "2026-11-22",
		"days_overdue": 1, "assignee_id": null, "assignee_name": null}`)
	want = "⏰ QA-13 “@" + wordJoiner + "all \\[x\\]\\(http://evil\\)” — 1 gün gecikir (22.11.2026) — icraçı yoxdur"
	if got := RenderChat(ev, ChatContext{}); got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
}

func TestRenderChatOverdueDigest(t *testing.T) {
	ev := store.Event{Action: "workspace.overdue_digest", EntityType: "workspace", Source: "system", WorkspaceSlug: "acme",
		Payload: payload(t, `{"date": "2026-11-23", "timezone": "Asia/Baku", "total": 7, "truncated": true, "items": [
			{"key": "QA-12", "title": "Başlıq", "project": "QA", "due_date": "2026-11-20", "days_overdue": 3,
			 "assignee_id": null, "assignee_name": "Bob Developer"},
			{"key": "OPS-4", "title": "<!channel> *təcili*", "project": "OPS", "due_date": "2026-11-22", "days_overdue": 1,
			 "assignee_id": null, "assignee_name": null}]}`)}
	want := "⚠️ Gecikmədə olanlar (7):\n" +
		"• <https://n.example.com/w/acme/p/QA?item=12|QA-12> Başlıq — 3 gün gecikir (20.11.2026) — Bob Developer\n" +
		"• <https://n.example.com/w/acme/p/OPS?item=4|OPS-4> &lt;" + wordJoiner + "!channel&gt; \\*təcili\\* — 1 gün gecikir (22.11.2026) — icraçı yoxdur\n" +
		"…və daha 5"
	if got := RenderChat(ev, ChatContext{PublicURL: "https://n.example.com"}); got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}

	// Not truncated, no public URL: plain keys, no "…və daha".
	ev.Payload = payload(t, `{"date": "2026-11-23", "timezone": "Asia/Baku", "total": 1, "truncated": false, "items": [
		{"key": "QA-12", "title": "Başlıq", "project": "QA", "due_date": "2026-11-20", "days_overdue": 3,
		 "assignee_id": null, "assignee_name": "Bob Developer"}]}`)
	want = "⚠️ Gecikmədə olanlar (1):\n• QA-12 Başlıq — 3 gün gecikir (20.11.2026) — Bob Developer"
	if got := RenderChat(ev, ChatContext{}); got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
}

// 100 long entries must stay under the message limit and count what was cut.
func TestRenderChatOverdueDigestLimit(t *testing.T) {
	items := make([]any, 100)
	for i := range items {
		items[i] = map[string]any{"key": fmt.Sprintf("QA-%d", i+1), "title": strings.Repeat("ə", 200), "project": "QA",
			"due_date": "2026-11-20", "days_overdue": 3.0, "assignee_name": "Bob"}
	}
	got := RenderChat(store.Event{Action: "workspace.overdue_digest", EntityType: "workspace",
		Payload: map[string]any{"total": 250.0, "truncated": true, "items": items}}, ChatContext{})
	lines := strings.Split(got, "\n")
	shown := len(lines) - 2 // header and "…və daha"
	if len(got) > maxChatText || shown < 1 || lines[len(lines)-1] != fmt.Sprintf("…və daha %d", 250-shown) {
		t.Fatalf("len %d, %d entries, last line %q", len(got), shown, lines[len(lines)-1])
	}
}

func TestRenderChatUnknownEventFallback(t *testing.T) {
	got := RenderChat(store.Event{Action: "item.archived", EntityType: "item", Source: "system"},
		ChatContext{Item: &store.ItemLabel{Key: "QA-1", Title: "X"}})
	if got != "ℹ️ item.archived: QA-1 “X” — Sistem" {
		t.Fatalf("got %q", got)
	}
}

func TestAzDate(t *testing.T) {
	for in, want := range map[string]string{"2026-11-20": "20.11.2026", "": "", "2026-1-20": "", "20xx-11-20": ""} {
		if got := azDate(in); got != want {
			t.Errorf("azDate(%q) = %q", in, got)
		}
	}
}
