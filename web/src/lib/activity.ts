import type { Activity } from "../api/types";
import { t } from "../i18n";
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

/** Turns one audit event into human-readable lines (one per changed field). */
export function describe(a: Activity, lookup: Lookup): Line[] {
  switch (a.action) {
    case "item.created":
      return [{ text: t.act.created }];
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

export function actorName(a: Activity): string {
  if (a.actor) return a.actor.display_name;
  if (a.source === "automation") return t.automation;
  return a.source === "system" ? t.system : t.someone;
}
