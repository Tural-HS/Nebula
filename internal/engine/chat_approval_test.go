package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/store"
)

// approvalFixture is a synthetic workspace: Carol asks, Eve decides.
func approvalFixture() (ChatContext, map[string]any, uuid.UUID) {
	carol, eve := uuid.New(), uuid.New()
	review, done := uuid.New(), uuid.New()
	c := ChatContext{
		Actor:          "Eve",
		Item:           &store.ItemLabel{Key: "FIN-7", ProjectKey: "FIN", Number: 7, Title: "Ödəniş (USD)"},
		ItemURL:        ItemURL("https://nebula.example.com", "acme", store.ItemLabel{ProjectKey: "FIN", Number: 7}),
		Users:          map[uuid.UUID]string{carol: "Carol", eve: "Eve"},
		Statuses:       map[uuid.UUID]string{review: "Yoxlamada", done: "Ödənildi"},
		Approvers:      []string{"Erin", "Eve"},
		ApproversTotal: 2,
	}
	p := map[string]any{
		"request_id": uuid.NewString(), "requested_by": carol.String(),
		"from_status_id": review.String(), "to_status_id": done.String(),
		"approvals": 0.0, "min_approvals": 2.0,
	}
	return c, p, eve
}

func withPayload(base map[string]any, extra map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func fieldMap(a ChatAttachment) map[string]string {
	m := map[string]string{}
	for _, f := range a.Fields {
		m[f.Title] = f.Value
	}
	return m
}

const wantApprovalLink = "https://nebula.example.com/w/acme/p/FIN?item=7&focus=approval"

func TestRenderChatMessageApprovalCards(t *testing.T) {
	c, p, eve := approvalFixture()
	tests := []struct {
		action, color, title, textPrefix string
		payload                          map[string]any
		fields                           map[string]string
	}{
		{store.EventApprovalRequested, colorPending, "FIN-7 · Təsdiq gözləyir", "🔏 Eve ",
			withPayload(p, map[string]any{"comment": "Qəbz əlavə olunub"}),
			map[string]string{"Təsdiq istəyən": "Carol", "Status": "Yoxlamada → Ödənildi", "Təsdiqlər": "0/2",
				"Şərh": "“Qəbz əlavə olunub”", "Təsdiq edə bilənlər": "Erin, Eve"}},
		{store.EventApprovalApproved, colorPending, "FIN-7 · Təsdiq verildi (1/2)", "👍 Eve ",
			withPayload(p, map[string]any{"approvals": 1.0, "comment": nil}),
			map[string]string{"Təsdiqləyən": "Eve", "Təsdiqlər": "1/2"}},
		{store.EventApprovalApproved, colorApproved, "FIN-7 · Təsdiq verildi (2/2)", "👍 Eve ",
			withPayload(p, map[string]any{"approvals": 2.0, "comment": "OK"}),
			map[string]string{"Təsdiqləyən": "Eve", "Şərh": "“OK”"}},
		{store.EventApprovalRejected, colorRejected, "FIN-7 · Rədd edildi", "⛔ Eve ",
			withPayload(p, map[string]any{"reason": "Məbləğ uyğun deyil"}),
			map[string]string{"Rədd edən": "Eve", "Səbəb": "“Məbləğ uyğun deyil”"}},
		{store.EventApprovalCanceled, colorCanceled, "FIN-7 · Təsdiq sorğusu ləğv edildi", "↩️ Eve ",
			p, map[string]string{"Ləğv edən": "Eve"}},
		{store.EventApprovalApplied, colorApproved, "FIN-7 · Təsdiqləndi, status dəyişdi", "✅ <" + c.ItemURL + "|FIN-7>",
			withPayload(p, map[string]any{"approvals": 2.0}), map[string]string{"Təsdiqlər": "2/2"}},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			ev := store.Event{Action: tt.action, ActorID: &eve, EntityType: "item", WorkspaceSlug: "acme", Payload: tt.payload}
			msg := RenderChatMessage(ev, c)
			if !strings.HasPrefix(msg.Text, tt.textPrefix) || msg.Username != "Nebula" {
				t.Fatalf("text %q (want prefix %q)", msg.Text, tt.textPrefix)
			}
			if len(msg.Attachments) != 1 {
				t.Fatalf("attachments: %+v", msg.Attachments)
			}
			a := msg.Attachments[0]
			if a.Color != tt.color || a.Title != tt.title || a.TitleLink != wantApprovalLink || a.Fallback != msg.Text {
				t.Fatalf("card: %+v", a)
			}
			if a.Text != "“Ödəniş \\(USD\\)”\n<"+wantApprovalLink+"|🔗 Nebula-da aç>" {
				t.Fatalf("card text %q", a.Text)
			}
			if strings.Join(a.MrkdwnIn, ",") != "text,fields" || !strings.Contains(a.Footer, "Nebula") {
				t.Fatalf("card: %+v", a)
			}
			got := fieldMap(a)
			for k, v := range tt.fields {
				if got[k] != v {
					t.Errorf("field %q = %q, want %q (all: %v)", k, got[k], v, got)
				}
			}
			if _, ok := got["Şərh"]; ok && tt.fields["Şərh"] == "" {
				t.Errorf("empty comment shown: %v", got)
			}
		})
	}
}

// Both servers take the same body: Slack secondary attachments are what
// Mattermost renders as message attachments.
func TestRenderChatMessageJSONShape(t *testing.T) {
	c, p, eve := approvalFixture()
	ev := store.Event{Action: store.EventApprovalRequested, ActorID: &eve, EntityType: "item", Payload: p}
	b, err := json.Marshal(RenderChatMessage(ev, c))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	att := m["attachments"].([]any)[0].(map[string]any)
	for _, k := range []string{"fallback", "color", "title", "title_link", "text", "fields", "footer", "mrkdwn_in"} {
		if _, ok := att[k]; !ok {
			t.Errorf("attachment has no %q: %s", k, b)
		}
	}
	if _, ok := m["text"].(string); !ok {
		t.Fatalf("no plain-text fallback: %s", b)
	}
	// Non-approval events stay plain text.
	plain, _ := json.Marshal(RenderChatMessage(store.Event{Action: "item.created", EntityType: "item"}, c))
	if strings.Contains(string(plain), "attachments") {
		t.Fatalf("plain event got a card: %s", plain)
	}
	// No item (deleted since): no card, the line still goes out.
	c.Item = nil
	if msg := RenderChatMessage(ev, c); len(msg.Attachments) != 0 || msg.Text == "" {
		t.Fatalf("message without item: %+v", msg)
	}
}

func TestRenderChatMessageApproverOverflow(t *testing.T) {
	c, p, eve := approvalFixture()
	c.Approvers = []string{"A", "B"}
	c.ApproversTotal = 7
	ev := store.Event{Action: store.EventApprovalRequested, ActorID: &eve, EntityType: "item", Payload: p}
	if got := fieldMap(RenderChatMessage(ev, c).Attachments[0])["Təsdiq edə bilənlər"]; got != "A, B və daha 5 nəfər" {
		t.Fatalf("approvers: %q", got)
	}
	c.Approvers, c.ApproversTotal = nil, 0
	if _, ok := fieldMap(RenderChatMessage(ev, c).Attachments[0])["Təsdiq edə bilənlər"]; ok {
		t.Fatal("empty approver field shown")
	}
}

// User-controlled text (title, comment, reason, names) must not create links
// or mentions anywhere in the message, card included.
func TestRenderChatMessageApprovalInjection(t *testing.T) {
	c, p, eve := approvalFixture()
	evil := "@channel @all <!here> <!channel> [Giriş](https://evil.example) <https://evil.example|Giriş> *x*\n# h \u202Eabc"
	c.Item.Title = evil
	c.Actor = "@here Eve"
	for id := range c.Users {
		c.Users[id] = "<!everyone> Mallory"
	}
	c.Statuses = map[uuid.UUID]string{}
	c.Approvers = []string{"@all", "[x](http://evil)"}
	c.ApproversTotal = 2
	for _, action := range []string{store.EventApprovalRequested, store.EventApprovalApproved, store.EventApprovalRejected} {
		ev := store.Event{Action: action, ActorID: &eve, EntityType: "item",
			Payload: withPayload(p, map[string]any{"comment": evil, "reason": evil})}
		b, _ := json.Marshal(RenderChatMessage(ev, c))
		var msg ChatMessage
		_ = json.Unmarshal(b, &msg)
		parts := []string{msg.Text}
		for _, a := range msg.Attachments {
			parts = append(parts, a.Title, a.Text, a.Fallback, a.Footer)
			for _, f := range a.Fields {
				parts = append(parts, f.Title, f.Value)
			}
		}
		for _, s := range parts {
			// Our own links are the only <…|…> tokens allowed.
			s = strings.ReplaceAll(s, "<"+c.ItemURL+"|FIN-7>", "")
			s = strings.ReplaceAll(s, "<"+wantApprovalLink+"|🔗 Nebula-da aç>", "")
			for _, bad := range []string{"<!", "<@", "<http", "](", "@channel", "@all", "@here", "@everyone", "\n#", "\u202E"} {
				if strings.Contains(s, bad) {
					t.Errorf("%s: %q contains %q", action, s, bad)
				}
			}
		}
	}
}

func TestApprovalLinkSurvivesHostileKeys(t *testing.T) {
	// Project keys are [A-Z0-9], but the link must stay a single token anyway.
	u := ItemURL("https://nebula.example.com", "a|b", store.ItemLabel{ProjectKey: "X>Y", Number: 1}) + approvalFocus
	if strings.ContainsAny(u, "|<> ") || !strings.HasSuffix(u, "?item=1&focus=approval") {
		t.Fatalf("link %q", u)
	}
}

// ADR-0034: re-approval and deadline events get their own card and line.
func TestRenderChatMessageReapprovalAndOverdue(t *testing.T) {
	c, p, eve := approvalFixture()
	inv := store.Event{Action: store.EventApprovalInvalidated, EntityType: "item", ActorID: &eve,
		Payload: withPayload(p, map[string]any{"changed": []any{"fields.amount", "title"}})}
	msg := RenderChatMessage(inv, c)
	if len(msg.Attachments) != 1 {
		t.Fatalf("invalidated: no card: %+v", msg)
	}
	a := msg.Attachments[0]
	if a.Color != colorPending || !strings.Contains(a.Title, "yenidən təsdiq") || fieldMap(a)["Dəyişən"] != "amount, başlıq" {
		t.Fatalf("invalidated card: %+v", a)
	}
	if !strings.HasPrefix(msg.Text, "♻️ Eve <") || !strings.Contains(msg.Text, "yenidən təsdiq lazımdır") {
		t.Fatalf("invalidated text: %q", msg.Text)
	}

	over := store.Event{Action: store.EventApprovalOverdue, EntityType: "item",
		Payload: withPayload(p, map[string]any{"due_on": "2031-03-20"})}
	msg = RenderChatMessage(over, c)
	a = msg.Attachments[0]
	if a.Color != colorRejected || fieldMap(a)["Son gün"] != "2031-03-20" || fieldMap(a)["Təsdiq edə bilənlər"] != "Erin, Eve" {
		t.Fatalf("overdue card: %+v", a)
	}
	if !strings.Contains(msg.Text, "təsdiq müddəti keçib (son gün 2031-03-20, 0/2)") || a.TitleLink != wantApprovalLink {
		t.Fatalf("overdue text: %q %q", msg.Text, a.TitleLink)
	}
	if _, err := json.Marshal(msg); err != nil {
		t.Fatal(err)
	}
}
