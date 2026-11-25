// English strings for archiving, restoring and deleting projects and workspaces (ADR-0039).
import type { Messages } from "../../i18n/locale";

export const tlc = {
  dangerZone: "Danger zone",
  cancel: "Cancel",
  restore: "Restore",
  open: "Open",
  archivedOn: (date: string) => `Archived: ${date}`,
  workspace: {
    archiveTitle: "Archive workspace",
    archiveHint:
      "The workspace becomes read-only and is hidden from the home page and the workspace list. Automations, webhooks, recurring items, reminders and email stop, and forms stop accepting requests. You can restore it at any time.",
    archive: "Archive",
    archiveConfirm: (name: string) => `Archive “${name}”?`,
    archiveConfirmHint: "Members can still see the data but cannot change anything. Until it is restored, the workspace is not listed.",
    archived: "Workspace archived.",
    restored: "Workspace restored.",
    ownerOnly: "Only the workspace owner can archive or delete it.",
    banner: "This workspace is archived: you can view its data but not change it.",
    bannerAdmin: "This workspace is archived: you can view its data but not change it. Restore it to work in it again.",
    deleteTitle: "Delete workspace permanently",
    deleteHint:
      "All projects, items, comments, files, forms, settings and the audit log are deleted and cannot be recovered. Only the server log keeps a record of the deletion.",
    deleteNeedsArchive: "Archive the workspace first to delete it.",
    deleteConfirm: (name: string) => `Delete “${name}” permanently?`,
    evidenceTitle: "Download the audit evidence package first",
    evidenceHint:
      "You may be required to retain financial records. After deletion nobody can recover the history, approvals or files.",
    evidenceLink: "Download the evidence package (Settings → Audit log)",
    evidenceAck: "I downloaded the evidence package, or I am sure it is not needed",
    typeSlug: (slug: string) => `To confirm, type the workspace slug: ${slug}`,
    deleteButton: "Delete permanently",
    deleted: "Workspace deleted.",
    deleting: "The workspace is being deleted. A large workspace can take a few minutes.",
  },
  project: {
    archiveTitle: "Archive project",
    archiveHint:
      "The project is hidden from lists, search, the home page and queues; its forms stop accepting requests; recurring items, automations and webhooks stop. An admin can restore it at any time.",
    archive: "Archive",
    archiveConfirm: (name: string) => `Archive the project “${name}”?`,
    archived: "Project archived.",
    restored: "Project restored.",
    deleteHint: "To delete the project permanently, archive it first. Only the workspace owner can delete an archived project.",
    deleteTitle: "Delete project permanently",
    deleteConsequences:
      "All items, subtasks, comments, files, links, approvals, views, forms and recurring items are deleted and cannot be recovered. The audit log keeps a record of the deletion.",
    deleteConfirm: (name: string) => `Delete the project “${name}” permanently?`,
    typeKey: (key: string) => `To confirm, type the project key: ${key}`,
    deleteButton: "Delete permanently",
    deleted: "Project deleted.",
    deleting: "The project is being deleted. A large project can take a few minutes.",
    stateTitle: "This project is archived",
    stateHint: "The items of an archived project are hidden and cannot be changed.",
    stateAskAdmin: "If you need the project again, ask a workspace admin.",
    deletingTitle: "This project is being deleted",
    archivedList: "Archived projects",
    archivedListHint: "Only admins see these. Open one to restore it or (owner) delete it permanently.",
  },
  home: {
    title: "Archived workspaces",
    hint: "Only owners and admins see these. They are read-only until restored.",
  },
  formClosedArchived: "This form belongs to an archived project and is not accepting requests.",
} satisfies Messages["lifecycle"];
