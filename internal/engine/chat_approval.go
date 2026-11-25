package engine

import (
	"fmt"
	"strings"

	"github.com/Tural-HS/Nebula/internal/store"
)

// Approval cards in chat channels (ADR-0031, step 1). The card shows the
// request and links to the item's approval panel; the decision itself is
// only ever made in Nebula, so a chat message carries no authority.

// ChatAttachment is a Slack "secondary attachment", which Mattermost renders
// as a message attachment (coloured card). Only fields both servers document
// are used.
type ChatAttachment struct {
	Fallback  string      `json:"fallback"`
	Color     string      `json:"color"`
	Title     string      `json:"title"`
	TitleLink string      `json:"title_link,omitempty"`
	Text      string      `json:"text,omitempty"`
	Fields    []ChatField `json:"fields,omitempty"`
	Footer    string      `json:"footer,omitempty"`
	// MrkdwnIn makes Slack format text and fields; Mattermost always does.
	MrkdwnIn []string `json:"mrkdwn_in,omitempty"`
}

// ChatField is one label/value pair of an attachment.
type ChatField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// Card colours (Tailwind amber-500, emerald-600, red-600, zinc-500).
const (
	colorPending  = "#F59E0B"
	colorApproved = "#059669"
	colorRejected = "#DC2626"
	colorCanceled = "#71717A"
)

// maxChatApprovers caps the names on a request card.
const maxChatApprovers = 10

// approvalFocus is appended to an item link so the web UI scrolls to the
// approval panel. A query parameter, not a #fragment: the sign-in redirect
// keeps the path and query only (web/src/auth/oidc.ts safeReturnPath).
const approvalFocus = "&focus=approval"

func isApprovalEvent(action string) bool {
	switch action {
	case store.EventApprovalRequested, store.EventApprovalApproved, store.EventApprovalRejected,
		store.EventApprovalCanceled, store.EventApprovalApplied,
		store.EventApprovalInvalidated, store.EventApprovalOverdue:
		return true
	}
	return false
}

// RenderChatMessage builds the chat body for ev: the plain line of
// RenderChat as text (the fallback and the mobile notification), plus a
// coloured card for approval events.
func RenderChatMessage(ev store.Event, c ChatContext) ChatMessage {
	msg := ChatMessage{Text: RenderChat(ev, c), Username: "Nebula"}
	if isApprovalEvent(ev.Action) && c.Item != nil {
		msg.Attachments = []ChatAttachment{approvalCard(ev, c, msg.Text)}
	}
	return msg
}

func approvalCard(ev store.Event, c ChatContext, fallback string) ChatAttachment {
	p := ev.Payload
	approvals, minApprovals := count(p["approvals"]), count(p["min_approvals"])
	progress := approvals + "/" + minApprovals
	a := ChatAttachment{Fallback: fallback, MrkdwnIn: []string{"text", "fields"}}
	var heading string
	switch ev.Action {
	case store.EventApprovalRequested:
		heading, a.Color = "Təsdiq gözləyir", colorPending
	case store.EventApprovalApproved:
		heading, a.Color = "Təsdiq verildi ("+progress+")", colorPending
		if done, ok := reached(p); ok && done {
			a.Color = colorApproved
		}
	case store.EventApprovalRejected:
		heading, a.Color = "Rədd edildi", colorRejected
	case store.EventApprovalCanceled:
		heading, a.Color = "Təsdiq sorğusu ləğv edildi", colorCanceled
	case store.EventApprovalApplied:
		heading, a.Color = "Təsdiqləndi, status dəyişdi", colorApproved
	case store.EventApprovalInvalidated:
		heading, a.Color = "Məzmun dəyişdi, yenidən təsdiq lazımdır", colorPending
	case store.EventApprovalOverdue:
		heading, a.Color = "Təsdiq müddəti keçib", colorRejected
	}
	// The title holds only our own text and the item key (project keys are
	// [A-Z0-9]), so it needs no markdown escaping on servers that show it raw.
	a.Title = escapeChat(c.Item.Key) + " · " + heading

	var text []string
	if c.Item.Title != "" {
		text = append(text, quote(c.Item.Title))
	}
	if c.ItemURL != "" {
		link := c.ItemURL + approvalFocus
		a.TitleLink = link
		// Same link syntax as the rest of the chat text (ADR-0010); the title
		// link is the second way in.
		text = append(text, "<"+link+"|🔗 Nebula-da aç>")
	}
	a.Text = strings.Join(text, "\n")

	field := func(title, value string, short bool) {
		if value != "" {
			a.Fields = append(a.Fields, ChatField{Title: title, Value: value, Short: short})
		}
	}
	if id, ok := uuidOf(p["requested_by"]); ok {
		field("Təsdiq istəyən", c.user(id), true)
	}
	field("Status", c.status(p["from_status_id"])+" → "+c.status(p["to_status_id"]), true)
	field("Təsdiqlər", progress, true)
	switch ev.Action {
	case store.EventApprovalRequested:
		field("Şərh", optQuote(p["comment"]), false)
		field("Təsdiq edə bilənlər", c.approverList(), false)
	case store.EventApprovalApproved:
		field("Təsdiqləyən", actorLabel(ev, c), true)
		field("Şərh", optQuote(p["comment"]), false)
	case store.EventApprovalRejected:
		field("Rədd edən", actorLabel(ev, c), true)
		field("Səbəb", optQuote(p["reason"]), false)
	case store.EventApprovalCanceled:
		field("Ləğv edən", actorLabel(ev, c), true)
	case store.EventApprovalInvalidated:
		field("Dəyişdirən", actorLabel(ev, c), true)
		field("Dəyişən", changedList(p["changed"]), false)
	case store.EventApprovalOverdue:
		field("Son gün", escapeChat(str(p["due_on"])), true)
		field("Təsdiq edə bilənlər", c.approverList(), false)
	}
	a.Footer = "Nebula · qərar yalnız Nebula-da verilir"
	return a
}

// changedList names the changed content of approval.invalidated:
// "başlıq, məbləğ" (custom fields by key).
func changedList(v any) string {
	list, _ := v.([]any)
	names := make([]string, 0, len(list))
	for _, x := range list {
		switch k := str(x); k {
		case "title":
			names = append(names, "başlıq")
		case "description":
			names = append(names, "təsvir")
		default:
			names = append(names, escapeChat(strings.TrimPrefix(k, "fields.")))
		}
	}
	return strings.Join(names, ", ")
}

// reached reports whether the approvals count reached the minimum.
func reached(p map[string]any) (bool, bool) {
	n, ok1 := p["approvals"].(float64)
	m, ok2 := p["min_approvals"].(float64)
	return n >= m, ok1 && ok2
}

func optQuote(v any) string {
	if s := str(v); strings.TrimSpace(s) != "" {
		return quote(s)
	}
	return ""
}

// withDelegates adds the delegates of absent approvers to the names on a
// request card, "Nəzirə (Elmar adına)", keeping at most maxChatApprovers
// names (the rest count in total).
func withDelegates(names []string, total int, delegates []store.PendingDelegate) ([]string, int) {
	for _, d := range delegates {
		total++
		if len(names) < maxChatApprovers {
			names = append(names, d.Name+" ("+strings.Join(d.For, ", ")+" adına)")
		}
	}
	return names, total
}

// approverList names who may still decide, "Ad1, Ad2 və daha 3 nəfər".
func (c ChatContext) approverList() string {
	if len(c.Approvers) == 0 {
		return ""
	}
	names := make([]string, 0, len(c.Approvers))
	for _, n := range c.Approvers {
		names = append(names, escapeChat(n))
	}
	out := strings.Join(names, ", ")
	if rest := c.ApproversTotal - len(c.Approvers); rest > 0 {
		out += fmt.Sprintf(" və daha %d nəfər", rest)
	}
	return out
}
