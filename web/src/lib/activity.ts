import type { Activity } from "../api/types";
import { t } from "../i18n";
import { tai } from "../i18n-approval-inbox";
import { tap } from "../i18n-approvals";
import { activeLocale } from "../i18n/locale";
import { ta } from "../i18n-attach";
import { te } from "../i18n-evidence";
import { tf } from "../i18n-forms";
import { tl } from "../i18n-links";
import { ts } from "../i18n-subtasks";
import { formatDate } from "./format";

export interface Lookup {
  status: (id: string) => string | undefined;
  person: (id: string) => string | undefined;
  field: (key: string) => string | undefined;
}

export interface Line {
  text: string;
  from?: string;
  to?: string;
}

interface Change {
  from?: unknown;
  to?: unknown;
}

function str(v: unknown): string {
  if (v === null || v === undefined || v === "") return "—";
  if (Array.isArray(v)) return v.map(str).join(", ");
  if (typeof v === "string" || typeof v === "number" || typeof v === "boolean") return String(v);
  return "…";
}

const num = (v: unknown) => (typeof v === "number" ? v : 0);
/** A free-text comment or reason as its own quoted line, if present. */
const note = (v: unknown): Line[] => (typeof v === "string" && v.trim() ? [{ text: `“${v.trim()}”` }] : []);

/** Turns one audit event into human-readable lines (one per changed field). */
export function describe(a: Activity, lookup: Lookup): Line[] {
  switch (a.action) {
    case "item.created":
      // A form submission (ADR-0032) carries the form's id.
      return [{ text: typeof a.data.form_id === "string" ? tf.act.fromForm : t.act.created }];
    case "request.message_set": {
      const msg = (a.data.message as Change | undefined)?.to;
      return typeof msg === "string" ? [{ text: tf.act.message }, ...note(msg)] : [{ text: tf.act.messageCleared }];
    }
    case "item.deleted":
      return [{ text: t.act.deleted }];
    case "item.overdue":
      return [{ text: t.act.overdue(typeof a.data.days_overdue === "number" ? a.data.days_overdue : 0) }];
    case "comment.created":
      return [{ text: t.act.commented }];
    case "comment.updated":
      return [{ text: t.act.commentEdited }];
    case "comment.deleted":
      return [{ text: t.act.commentDeleted }];
    case "attachment.added":
      return [{ text: ta.act.added(str(a.data.filename)) }];
    case "attachment.removed":
      return [{ text: ta.act.removed(str(a.data.filename)) }];
    case "approval.requested":
      return [{ text: tap.act.requested, from: lookup.status(str(a.data.from_status_id)) ?? "?", to: lookup.status(str(a.data.to_status_id)) ?? "?" },
        ...note(a.data.comment)];
    case "approval.approved":
      return [{ text: tap.act.approved(num(a.data.approvals), num(a.data.min_approvals)) }, ...note(a.data.comment)];
    case "approval.rejected":
      return [{ text: tap.act.rejected }, ...note(a.data.reason)];
    case "approval.canceled":
      return [{ text: tap.act.canceled }];
    case "approval.applied":
      return [{ text: tap.act.applied, to: lookup.status(str(a.data.to_status_id)) ?? "?" }];
    case "approval.invalidated":
      return [{ text: tai.act.invalidated, to: lookup.status(str(a.data.reverted_to_status_id)) ?? "?" }];
    case "approval.overdue":
      return [{ text: tai.act.overdue }];
    case "evidence.exported":
      return [{ text: te.act.exported }];
    case "link.added":
    case "link.removed": {
      const other = a.data.other_item as { key?: unknown } | undefined;
      const key = typeof other?.key === "string" ? other.key : "?";
      const type = linkTypeName(a.data.type);
      if (a.action === "link.removed" && a.data.reason === "item_deleted") return [{ text: tl.act.removedByDelete(key) }];
      return [{ text: a.action === "link.added" ? tl.act.added(type, key) : tl.act.removed(type, key) }];
    }
    case "item.updated": {
      const changes = (a.data.changes ?? {}) as Record<string, Change>;
      const lines: Line[] = [];
      for (const [k, c] of Object.entries(changes)) {
        switch (k) {
          case "title":
            lines.push({ text: t.act.changedTitle, from: str(c.from), to: str(c.to) });
            break;
          case "description":
            lines.push({ text: t.act.changedDescription });
            break;
          case "status_id":
            lines.push({
              text: t.act.changedStatus,
              from: lookup.status(str(c.from)) ?? "?",
              to: lookup.status(str(c.to)) ?? "?",
            });
            break;
          case "due_date":
            lines.push({
              text: t.act.changedDue,
              from: typeof c.from === "string" ? formatDate(c.from) : "—",
              to: typeof c.to === "string" ? formatDate(c.to) : "—",
            });
            break;
          case "parent":
            lines.push({ text: ts.changedParent, from: str(c.from), to: str(c.to) });
            break;
          case "assignee_id":
            lines.push({
              text: t.act.changedAssignee,
              from: c.from ? (lookup.person(str(c.from)) ?? t.unknownPerson) : t.unassigned,
              to: c.to ? (lookup.person(str(c.to)) ?? t.unknownPerson) : t.unassigned,
            });
            break;
          default:
            if (k.startsWith("fields.")) {
              const key = k.slice("fields.".length);
              lines.push({ text: t.act.changedField(lookup.field(key) ?? key) });
            } else {
              lines.push({ text: t.act.other(k) });
            }
        }
      }
      return lines.length > 0 ? lines : [{ text: t.act.other(a.action) }];
    }
    default:
      return [{ text: t.act.other(a.action) }];
  }
}

function linkTypeName(v: unknown): string {
  const types: Partial<Record<string, string>> = tl.types;
  const name = typeof v === "string" ? types[v] : undefined;
  return name ? name.toLocaleLowerCase(activeLocale()) : "?";
}

export function actorName(a: Activity): string {
  if (a.actor) return a.actor.display_name;
  if (a.source === "automation") return t.automation;
  return a.source === "system" ? t.system : t.someone;
}
