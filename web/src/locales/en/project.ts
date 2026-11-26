// English strings for project settings and privacy (ADR-0013).
import type { Messages } from "../../i18n/locale";

export const tp = {
  settings: "Project settings",
  reminder: {
    title: "Birthday reminder",
    hint: "When the day and month of the chosen date field fall on today (e.g. a customer's birthday), everyone who can see the project gets an in-app notification. Once a day, at the digest hour (09:00 when the digest is off).",
    off: "Off",
    field: "Date field",
    saved: "Birthday reminder saved",
    noDate: "The workspace has no date field: create one in Settings → Fields first.",
    last: "Last sent",
    saveLabel: "Save",
  },
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
  switchProject: "Switch project",
  privateMark: "private project",
} satisfies Messages["project"];
