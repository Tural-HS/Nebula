// English strings for accepting an invitation (ADR-0016).
import type { Messages } from "../../i18n/locale";

export const ti = {
  title: "Workspace invitation",
  invitedBy: (who: string, ws: string, role: string) => `${who} invited you to the "${ws}" workspace as ${role}.`,
  accept: "Accept",
  accepted: "Welcome!",
  alreadyMember: "You are already a member of this workspace.",
  open: "Open workspace",
  mismatch: (masked: string) => `This invitation is for ${masked}. Sign in with that email.`,
  unverified: "Your email address is not verified in the identity provider, so the invitation cannot be accepted.",
  switchAccount: "Sign in with another account",
  invalid: "Invitation not found: the link is wrong, it was revoked, or a newer one was sent.",
  used: "This invitation has already been used.",
  expired: "This invitation has expired. Ask an admin for a new one.",
} satisfies Messages["invite"];
