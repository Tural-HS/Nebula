// Pure helpers for the home page "Mənim işlərim" (ADR-0037).
import type { HomeCounters, HomeItem, HomeShow, HomeWorkspace } from "../api/home";
import { activeLocale, type Locale } from "../i18n/locale";

/** The counter cards, in screen order, with the list each one opens. */
export const COUNTERS = [
  { key: "overdue", show: "overdue" },
  { key: "approvals", show: "approval" },
  { key: "week", show: "week" },
  { key: "assigned", show: "assigned" },
] as const satisfies readonly { key: keyof HomeCounters; show: HomeShow }[];

export type CounterKey = (typeof COUNTERS)[number]["key"];

/** The home page's own list of one kind. */
export const homeListHref = (show: HomeShow) => `/?show=${show}`;

/**
 * Where a counter leads. With one workspace, "awaiting my approval" opens
 * that workspace's inbox: the same requests by the same rule (ADR-0034).
 * Every other counter spans projects, and only the home page lists items
 * across projects, so it opens the home page's own filtered list.
 */
export function counterHref(show: HomeShow, workspaces: readonly Pick<HomeWorkspace, "slug">[]): string {
  const only = workspaces.length === 1 ? workspaces[0] : undefined;
  if (show === "approval" && only) return `/w/${encodeURIComponent(only.slug)}/approvals`;
  return homeListHref(show);
}

/** Opens the item drawer in its project; approval rows open at the approval panel. */
export function itemHref(it: Pick<HomeItem, "workspace" | "project" | "number" | "reason">): string {
  const base = `/w/${encodeURIComponent(it.workspace.slug)}/p/${encodeURIComponent(it.project.key)}?item=${String(it.number)}`;
  return it.reason === "approval" ? `${base}&focus=approval` : base;
}

/** "Alice Developer" → "Alice"; an e-mail-like name → its local part. */
export function firstName(displayName: string | undefined): string {
  const name = (displayName ?? "").trim();
  if (!name) return "";
  const first = name.split(/\s+/)[0] ?? "";
  return first.includes("@") ? (first.split("@")[0] ?? "") : first;
}

const AZ_MONTHS = ["yanvar", "fevral", "mart", "aprel", "may", "iyun", "iyul", "avqust", "sentyabr", "oktyabr", "noyabr", "dekabr"] as const;
const AZ_WEEKDAYS = ["bazar", "bazar ertəsi", "çərşənbə axşamı", "çərşənbə", "cümə axşamı", "cümə", "şənbə"] as const;
const BCP47: Record<Exclude<Locale, "az">, string> = { en: "en-GB", tr: "tr-TR", ru: "ru-RU" };

/**
 * Today's date for the greeting, in the UI language. Azerbaijani is written
 * by hand: browser Intl data for "az" is incomplete (see lib/format.ts).
 */
export function formatToday(d: Date = new Date(), locale: Locale = activeLocale()): string {
  if (locale === "az") {
    return `${String(d.getDate())} ${AZ_MONTHS[d.getMonth()] ?? ""} ${String(d.getFullYear())}, ${AZ_WEEKDAYS[d.getDay()] ?? ""}`;
  }
  return new Intl.DateTimeFormat(BCP47[locale], { weekday: "long", day: "numeric", month: "long", year: "numeric" }).format(d);
}

/** Workspaces that need me (overdue or approvals) first, then by name; the server sends them by name. */
export function orderCards<T extends Pick<HomeWorkspace, "name" | "overdue" | "approvals">>(list: readonly T[]): T[] {
  const urgent = (w: T) => (w.overdue + w.approvals > 0 ? 0 : 1);
  return [...list].sort((a, b) => urgent(a) - urgent(b) || a.name.localeCompare(b.name));
}

/** What a row shows after its title: the approval's age, or the due date. */
export type RowDate = { kind: "approval"; since: string; dueOn: string | null; overdue: boolean } | { kind: "due"; date: string; overdue: boolean } | null;

export function rowDate(it: Pick<HomeItem, "reason" | "approval" | "due_date">): RowDate {
  if (it.reason === "approval" && it.approval) {
    return { kind: "approval", since: it.approval.requested_at, dueOn: it.approval.due_on, overdue: it.approval.overdue };
  }
  if (it.due_date) return { kind: "due", date: it.due_date, overdue: it.reason === "overdue" };
  return null;
}

/** Nothing on the home page asks for action. */
export const isCalm = (c: HomeCounters) => c.overdue + c.approvals + c.week === 0;
