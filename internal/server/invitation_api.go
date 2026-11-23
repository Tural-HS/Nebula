package server

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/store"
)

// Invitations (ADR-0016). Managing them is admin+ within a workspace;
// previewing and accepting is for the signed-in invitee and not workspace
// scoped. Tokens travel in request bodies, never in URLs, so they do not end
// up in access or proxy logs.

func (a *api) invitationAdminRoutes(r chi.Router) {
	admin := r.With(requireRole(store.RoleAdmin))
	admin.Get("/invitations", a.listInvitations)
	admin.Post("/invitations", a.createInvitation)
	admin.Post("/invitations/{invitationID}/resend", a.resendInvitation)
	admin.Delete("/invitations/{invitationID}", a.revokeInvitation)
}

func (a *api) invitationPublicRoutes(r chi.Router) {
	r.Post("/invitations/preview", a.previewInvitation)
	r.Post("/invitations/accept", a.acceptInvitation)
}

// acceptURL builds the email link; it fails closed without a public URL so a
// link to the wrong host (e.g. localhost) is never mailed.
func (a *api) acceptURL() (func(string) string, bool) {
	if a.publicURL == "" {
		return nil, false
	}
	base := strings.TrimRight(a.publicURL, "/")
	return func(token string) string { return base + "/invite/" + url.PathEscape(token) }, true
}

func (a *api) listInvitations(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListInvitations(r.Context(), workspaceFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invitations": nonNil(list)})
}

func (a *api) createInvitation(w http.ResponseWriter, r *http.Request) {
	link, ok := a.acceptURL()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "public_url_missing", "invitations need NEBULA_PUBLIC_URL to be configured")
		return
	}
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	ws := workspaceFrom(r)
	inv, err := a.store.CreateInvitation(r.Context(), store.InviteParams{
		WorkspaceID: ws.ID, ActorID: principal(r).UserID, ActorRole: ws.Role, Email: body.Email, Role: body.Role, AcceptURL: link,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

func (a *api) resendInvitation(w http.ResponseWriter, r *http.Request) {
	link, ok := a.acceptURL()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "public_url_missing", "invitations need NEBULA_PUBLIC_URL to be configured")
		return
	}
	id, ok := a.uuidParam(w, r, "invitationID")
	if !ok {
		return
	}
	if err := a.store.ResendInvitation(r.Context(), workspaceFrom(r).ID, principal(r).UserID, id, link); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "invitationID")
	if !ok {
		return
	}
	if err := a.store.RevokeInvitation(r.Context(), workspaceFrom(r).ID, principal(r).UserID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) previewInvitation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	p, err := a.store.PreviewInvitation(r.Context(), body.Token, principal(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *api) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	slug, err := a.store.AcceptInvitation(r.Context(), body.Token, principal(r))
	if errors.Is(err, store.ErrInvitationEmailMismatch) {
		msg := "sign in with the invited email address"
		if !principal(r).EmailVerified {
			msg = "your identity provider has not verified your email address"
		}
		writeError(w, http.StatusForbidden, "invitation_email_mismatch", msg)
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace": slug})
}
