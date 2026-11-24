// English strings for project settings and privacy (ADR-0013).
import type { Messages } from "../../i18n/locale";

export const tp = {
  settings: "Project settings",
  private: "Private",
  privateProject: "Private project",
  privateHint: "Only project members and workspace admins can see it.",
  visibility: "Visibility",
  visibilityOptions: { workspace: "All workspace members", private: "Project members only (private)" },
  members: "Project members",
  membersHint: "Workspace admins and owners can always see private projects; they may not be listed here.",
  addMember: "Add member",
  noMembers: "No one has been added yet.",
  remove: "Remove",
  webhookWarning:
    "Events from a private project are only sent to webhooks that explicitly select it (Settings → Webhooks → Projects). For a team channel, select this project in the webhook.",
  makePrivateConfirm: "Make the project private? Regular members who are not project members will no longer see it.",
  adminOnly: "Only admins can change project settings.",
} satisfies Messages["project"];
