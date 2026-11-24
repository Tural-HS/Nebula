// English strings for audit evidence export (ADR-0029).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const WHAT = {
  items: "items",
  events: "events",
  comments: "comments",
  approvals: "approval requests",
  attachments: "attachments",
  links: "links",
  bytes: "data (over 100 MB)",
} as const;

const n = (count: number, one: string, other: string) => `${String(count)} ${plural("en", count, { one, other })}`;

export const te: Messages["evidence"] = {
  title: "Audit evidence package",
  intro:
    "A signed ZIP for auditors and regulators: a readable report (open it in a browser and print to PDF), JSON and CSV data, a manifest with the SHA-256 of every file and its Ed25519 signature. The package contains only items you can see; every export is recorded in the audit log.",
  projects: "Projects",
  projectsHint: "None selected: every project you can see.",
  from: "Start date",
  to: "End date",
  approvalsOnly: "Only items that went through approval",
  submit: "Export evidence package",
  exporting: "Preparing the package…",
  downloaded: "Evidence package downloaded",
  limits: (days, items, events) => `A date range is required: at most ${String(days)} days, ${String(items)} items and ${String(events)} events.`,
  key: "Signing key",
  fingerprint: "Key fingerprint",
  keyHint: "The auditor verifies the package with this command. Send them the fingerprint over a separate channel (not email: by phone or a signed letter, for example).",
  copy: "Copy",
  copied: "Copied",
  disabled: "Evidence export is disabled: NEBULA_EVIDENCE_SIGNING_KEY is not set on the server (see deploy/production/README.md).",
  history: "Export history",
  historyEmpty: "No evidence package has been exported yet.",
  cols: { time: "Time", who: "Who", what: "What", content: "Content", manifest: "Manifest SHA-256" },
  kindItem: (key) => `Item ${key}`,
  kindScope: (from, to) => `${from} … ${to}`,
  allProjects: "all projects",
  projectCount: (count) => n(count, "project", "projects"),
  approvalsTag: "approvals only",
  counts: (items, events) => `${n(items, "item", "items")}, ${n(events, "event", "events")}`,
  itemButton: "Export evidence",
  itemButtonHint: "The item's full history, comments, approvals, links and the SHA-256 list of its files: a signed ZIP",
  errors: {
    required: "Choose a start and an end date.",
    order: "The end date cannot be before the start date.",
    tooLong: (days) => `The date range can be at most ${String(days)} days.`,
    tooLarge: (what) => `Too many ${(WHAT as Partial<Record<string, string>>)[what] ?? "records"} for one package: choose fewer projects or a shorter range.`,
    rateLimited: "Too many exports: try again later.",
    busy: "Other exports are running: try again in a few seconds.",
  },
  act: { exported: "Evidence package exported" },
};
