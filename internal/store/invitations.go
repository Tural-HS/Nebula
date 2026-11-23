package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/auth"
)

// Workspace invitations (ADR-0016).

const (
	invitationTTL             = 7 * 24 * time.Hour
	maxPendingInvitations     = 200
	maxInvitationsPerDay      = 100
	invitationTokenBytes      = 32
	invitationAcceptedAction  = "invitation.accepted"
	invitationCreatedAction   = "invitation.created"
	invitationResentAction    = "invitation.resent"
	invitationRevokedAction   = "invitation.revoked"
	invitationEmailDedupeSalt = "inv:"
)

// ErrInvitationEmailMismatch means the signed-in user's verified email is not the
// invited address (a forwarded link, or the wrong account).
var ErrInvitationEmailMismatch = errors.New("invitation email mismatch")

// Invitation is a pending (or past) invitation as admins see it. The token is
// never part of it.
type Invitation struct {
	ID        uuid.UUID  `json:"id"`
	Email     string     `json:"email"`
	Role      string     `json:"role"`
	InvitedBy *string    `json:"invited_by"` // display name
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	Accepted  *time.Time `json:"accepted_at"`
}

// InvitationPreview is what the invited person sees before accepting.
type InvitationPreview struct {
	WorkspaceSlug string `json:"workspace_slug"`
	WorkspaceName string `json:"workspace_name"`
	Role          string `json:"role"`
	InvitedBy     string `json:"invited_by"`
	EmailMasked   string `json:"email"` // the invited address, masked
	EmailMatches  bool   `json:"email_matches"`
	AlreadyMember bool   `json:"already_member"`
}

func newInvitationToken() (string, []byte, error) {
	b := make([]byte, invitationTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(token))
	return token, h[:], nil
}

func hashInvitationToken(token string) ([]byte, bool) {
	// Reject anything that is not a well-formed token before touching the DB.
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(b) != invitationTokenBytes {
		return nil, false
	}
	h := sha256.Sum256([]byte(token))
	return h[:], true
}

// MaskEmail shows enough of an address to recognize it: t***@fintez.az.
func MaskEmail(email string) string {
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" {
		return "***"
	}
	return string([]rune(local)[0]) + "***@" + domain
}

func normalizeInviteEmail(raw string) (string, error) {
	a, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil || a.Name != "" || len(a.Address) > 320 || strings.ContainsAny(a.Address, "\r\n") {
		return "", invalid("email", "must be a single email address")
	}
	return strings.ToLower(a.Address), nil
}

// InviteParams are the inputs of CreateInvitation.
type InviteParams struct {
	WorkspaceID uuid.UUID
	ActorID     uuid.UUID
	ActorRole   string
	Email       string
	Role        string
	// AcceptURL builds the link for a token (public base URL + /invite/{token}).
	AcceptURL func(token string) string
}

// CreateInvitation records an invitation and queues its email in the same
// transaction (outbox). Admin+ is checked by the caller; role limits here.
func (s *Store) CreateInvitation(ctx context.Context, p InviteParams) (Invitation, error) {
	if _, ok := roleRank[p.Role]; !ok {
		return Invitation{}, invalid("role", "must be one of guest, member, admin, owner")
	}
	if p.Role == RoleOwner && p.ActorRole != RoleOwner {
		return Invitation{}, ErrForbidden // same rule as adding a member directly
	}
	if !RoleAtLeast(p.ActorRole, p.Role) {
		return Invitation{}, invalid("role", "cannot invite with a role above your own")
	}
	email, err := normalizeInviteEmail(p.Email)
	if err != nil {
		return Invitation{}, err
	}
	token, hash, err := newInvitationToken()
	if err != nil {
		return Invitation{}, err
	}
	var inv Invitation
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		// Serialize per workspace so the limits hold under concurrent requests.
		var wsName, inviter string
		if err := tx.QueryRow(ctx, `SELECT w.name, u.display_name FROM workspaces w, users u
			WHERE w.id = $1 AND u.id = $2 FOR NO KEY UPDATE OF w`, p.WorkspaceID, p.ActorID).Scan(&wsName, &inviter); err != nil {
			return err
		}
		var member bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspace_members m JOIN users u ON u.id = m.user_id
			WHERE m.workspace_id = $1 AND lower(u.email) = $2)`, p.WorkspaceID, email).Scan(&member); err != nil {
			return err
		}
		if member {
			return &ConflictError{Code: "already_member", Message: "this person is already a member of the workspace"}
		}
		var pending, today int
		if err := tx.QueryRow(ctx, `SELECT
				count(*) FILTER (WHERE accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now()),
				count(*) FILTER (WHERE created_at > now() - interval '1 day')
			FROM workspace_invitations WHERE workspace_id = $1`, p.WorkspaceID).Scan(&pending, &today); err != nil {
			return err
		}
		if pending >= maxPendingInvitations {
			return &ConflictError{Code: "invitation_limit", Message: fmt.Sprintf("at most %d pending invitations", maxPendingInvitations)}
		}
		if today >= maxInvitationsPerDay {
			return &ConflictError{Code: "invitation_rate_limit", Message: fmt.Sprintf("at most %d invitations per day", maxInvitationsPerDay)}
		}
		// An expired pending row would block the unique index: retire it.
		if _, err := tx.Exec(ctx, `UPDATE workspace_invitations SET revoked_at = now()
			WHERE workspace_id = $1 AND email = $2 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at <= now()`,
			p.WorkspaceID, email); err != nil {
			return err
		}
		id := newID()
		err := tx.QueryRow(ctx, `INSERT INTO workspace_invitations (id, workspace_id, email, role, token_hash, invited_by, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)
			RETURNING id, email, role, expires_at, created_at`,
			id, p.WorkspaceID, email, p.Role, hash, p.ActorID, invitationTTL.String()).
			Scan(&inv.ID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.CreatedAt)
		if isUniqueViolation(err, "workspace_invitations_pending_idx") {
			return &ConflictError{Code: "already_invited", Message: "this address already has a pending invitation; resend it instead"}
		}
		if err != nil {
			return err
		}
		inv.InvitedBy = &inviter
		if err := queueInvitationEmail(ctx, tx, id, email, inviter, wsName, p.Role, p.AcceptURL(token)); err != nil {
			return err
		}
		return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "invitation", id, invitationCreatedAction,
			map[string]any{"email": email, "role": p.Role})
	})
	return inv, err
}

var roleNamesAz = map[string]string{RoleGuest: "qonaq", RoleMember: "üzv", RoleAdmin: "admin", RoleOwner: "sahib"}

func queueInvitationEmail(ctx context.Context, q querier, id uuid.UUID, to, inviter, wsName, role, link string) error {
	clean := strings.NewReplacer("\r", " ", "\n", " ")
	subject := fmt.Sprintf("%s sizi Nebula-da \"%s\" workspace-inə dəvət etdi", clean.Replace(inviter), clean.Replace(wsName))
	body := fmt.Sprintf("Salam,\n\n%s sizi Nebula-da \"%s\" workspace-inə %s kimi dəvət etdi.\n\n"+
		"Dəvəti qəbul etmək üçün linkə keçin və bu email ünvanı (%s) ilə daxil olun:\n%s\n\n"+
		"Link 7 gün etibarlıdır və yalnız bir dəfə istifadə oluna bilər.\n"+
		"Bu dəvəti gözləmirdinizsə, sadəcə bu məktubu nəzərə almayın.\n",
		inviter, wsName, roleNamesAz[role], to, link)
	return enqueueJob(ctx, q, JobSendEmail, map[string]any{"to": []string{to}, "subject": subject, "body": body}, 8,
		invitationEmailDedupeSalt+id.String()+":"+time.Now().UTC().Format(time.RFC3339Nano))
}

// ListInvitations returns the workspace's pending invitations, newest first.
func (s *Store) ListInvitations(ctx context.Context, wsID uuid.UUID) ([]Invitation, error) {
	rows, err := s.pool.Query(ctx, `SELECT i.id, i.email, i.role, u.display_name, i.expires_at, i.created_at, i.accepted_at
		FROM workspace_invitations i LEFT JOIN users u ON u.id = i.invited_by
		WHERE i.workspace_id = $1 AND i.accepted_at IS NULL AND i.revoked_at IS NULL
		ORDER BY i.created_at DESC, i.id`, wsID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Invitation, error) {
		var inv Invitation
		return inv, r.Scan(&inv.ID, &inv.Email, &inv.Role, &inv.InvitedBy, &inv.ExpiresAt, &inv.CreatedAt, &inv.Accepted)
	})
}

// RevokeInvitation cancels a pending invitation; its link stops working.
func (s *Store) RevokeInvitation(ctx context.Context, wsID, actorID, id uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var email string
		err := tx.QueryRow(ctx, `UPDATE workspace_invitations SET revoked_at = now()
			WHERE workspace_id = $1 AND id = $2 AND accepted_at IS NULL AND revoked_at IS NULL RETURNING email`, wsID, id).Scan(&email)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "invitation", id, invitationRevokedAction, map[string]any{"email": email})
	})
}

// ResendInvitation issues a new token (the old link stops working), extends
// the expiry and sends the email again.
func (s *Store) ResendInvitation(ctx context.Context, wsID, actorID, id uuid.UUID, acceptURL func(string) string) error {
	token, hash, err := newInvitationToken()
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var email, role, wsName, inviter string
		err := tx.QueryRow(ctx, `UPDATE workspace_invitations i SET token_hash = $3, expires_at = now() + $4::interval
			FROM workspaces w, users u
			WHERE i.workspace_id = $1 AND i.id = $2 AND i.accepted_at IS NULL AND i.revoked_at IS NULL
			  AND w.id = i.workspace_id AND u.id = $5
			RETURNING i.email, i.role, w.name, u.display_name`, wsID, id, hash, invitationTTL.String(), actorID).
			Scan(&email, &role, &wsName, &inviter)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := queueInvitationEmail(ctx, tx, id, email, inviter, wsName, role, acceptURL(token)); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "invitation", id, invitationResentAction, map[string]any{"email": email})
	})
}

type invitationRow struct {
	id, wsID          uuid.UUID
	email, role       string
	slug, wsName      string
	inviter           *string
	expired, accepted bool
}

// pendingInvitation finds a usable invitation by token. Unknown, revoked or
// malformed tokens are ErrNotFound; expired ones say so.
func pendingInvitation(ctx context.Context, q querier, token string, forUpdate bool) (invitationRow, error) {
	hash, ok := hashInvitationToken(token)
	if !ok {
		return invitationRow{}, ErrNotFound
	}
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE OF i"
	}
	var r invitationRow
	err := q.QueryRow(ctx, `SELECT i.id, i.workspace_id, i.email, i.role, w.slug, w.name, u.display_name,
			i.expires_at <= now(), i.accepted_at IS NOT NULL
		FROM workspace_invitations i
		JOIN workspaces w ON w.id = i.workspace_id
		LEFT JOIN users u ON u.id = i.invited_by
		WHERE i.token_hash = $1 AND i.revoked_at IS NULL`+lock, hash).
		Scan(&r.id, &r.wsID, &r.email, &r.role, &r.slug, &r.wsName, &r.inviter, &r.expired, &r.accepted)
	if errors.Is(err, pgx.ErrNoRows) {
		return invitationRow{}, ErrNotFound
	}
	if err != nil {
		return invitationRow{}, err
	}
	if r.accepted {
		return invitationRow{}, &ConflictError{Code: "invitation_used", Message: "this invitation has already been used"}
	}
	if r.expired {
		return invitationRow{}, &ConflictError{Code: "invitation_expired", Message: "this invitation has expired; ask for a new one"}
	}
	return r, nil
}

// PreviewInvitation tells the signed-in user what they were invited to.
func (s *Store) PreviewInvitation(ctx context.Context, token string, who auth.Principal) (InvitationPreview, error) {
	r, err := pendingInvitation(ctx, s.pool, token, false)
	if err != nil {
		return InvitationPreview{}, err
	}
	p := InvitationPreview{WorkspaceSlug: r.slug, WorkspaceName: r.wsName, Role: r.role, EmailMasked: MaskEmail(r.email),
		EmailMatches: who.EmailVerified && strings.EqualFold(who.Email, r.email)}
	if r.inviter != nil {
		p.InvitedBy = *r.inviter
	}
	if !p.EmailMatches {
		// Do not tell a different account which workspace it is.
		p.WorkspaceSlug, p.WorkspaceName, p.InvitedBy = "", "", ""
	}
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspace_members WHERE workspace_id = $1 AND user_id = $2)`,
		r.wsID, who.UserID).Scan(&p.AlreadyMember); err != nil {
		return InvitationPreview{}, err
	}
	return p, nil
}

// AcceptInvitation adds the signed-in user to the workspace if the token is
// valid and their IdP-verified email is the invited address. Returns the
// workspace slug.
func (s *Store) AcceptInvitation(ctx context.Context, token string, who auth.Principal) (string, error) {
	var slug string
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		r, err := pendingInvitation(ctx, tx, token, true)
		if err != nil {
			return err
		}
		if !who.EmailVerified || !strings.EqualFold(who.Email, r.email) {
			return ErrInvitationEmailMismatch
		}
		slug = r.slug
		tag, err := tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, r.wsID, who.UserID, r.role)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE workspace_invitations SET accepted_at = now(), accepted_by = $2 WHERE id = $1`, r.id, who.UserID); err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // already a member: the invitation is just used up
		}
		if err := appendEvent(ctx, tx, r.wsID, who.UserID, "member", who.UserID, "member.added", map[string]any{"role": r.role, "via": "invitation"}); err != nil {
			return err
		}
		return appendEvent(ctx, tx, r.wsID, who.UserID, "invitation", r.id, invitationAcceptedAction, map[string]any{"email": r.email, "role": r.role})
	})
	return slug, err
}
