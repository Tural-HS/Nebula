// Pure helpers for absences and approval delegation (ADR-0042). The server
// decides; these mirror its checks so the form explains a 422 before
// sending, and pick what the banners show.
import type { Absence } from "../api/absences";
import type { Member } from "../api/types";

/** Longest absence: ends_on - starts_on (the server's limit). */
export const MAX_ABSENCE_DAYS = 366;
/** How far ahead an absence may start, in days. */
export const MAX_ABSENCE_LEAD = 365;
export const MAX_ABSENCE_NOTE = 500;

const DATE = /^\d{4}-\d{2}-\d{2}$/;

function dayNumber(d: string): number {
  return Date.UTC(Number(d.slice(0, 4)), Number(d.slice(5, 7)) - 1, Number(d.slice(8, 10))) / 86_400_000;
}

export type AbsenceProblem = "invalid" | "order" | "endPast" | "startPast" | "tooLong" | "tooFar" | "selfDelegate";

/**
 * What is wrong with an absence form, or null. today is the workspace-local
 * date; newStart: the start is new or changed (it must not lie in the past).
 */
export function absenceProblem(
  v: { starts_on: string; ends_on: string; user_id?: string; delegate_id: string | null },
  today: string,
  newStart: boolean,
): AbsenceProblem | null {
  if (!DATE.test(v.starts_on) || !DATE.test(v.ends_on) || Number.isNaN(dayNumber(v.starts_on)) || Number.isNaN(dayNumber(v.ends_on))) return "invalid";
  const start = dayNumber(v.starts_on), end = dayNumber(v.ends_on), now = dayNumber(today);
  if (end < start) return "order";
  if (end - start > MAX_ABSENCE_DAYS) return "tooLong";
  if (end < now) return "endPast";
  if (newStart && start < now) return "startPast";
  if (start - now > MAX_ABSENCE_LEAD) return "tooFar";
  if (v.user_id && v.delegate_id === v.user_id) return "selfDelegate";
  return null;
}

/** Who can be a delegate for userId: other members with at least the member role, by name. */
export function delegateCandidates(members: readonly Member[], userId: string): Member[] {
  return members
    .filter((m) => m.user_id !== userId && m.role !== "guest")
    .sort((a, b) => a.display_name.localeCompare(b.display_name) || a.user_id.localeCompare(b.user_id));
}

/** Whether a member can record an absence at all (guests never approve). */
export const canBeAbsent = (m: Pick<Member, "role">): boolean => m.role !== "guest";

/** The member's current absence, else their next upcoming one. */
export function absenceOf(list: readonly Absence[] | undefined, userId: string | undefined): Absence | undefined {
  if (!list || !userId) return undefined;
  const mine = list.filter((a) => a.user.id === userId && !a.ended_at);
  return mine.find((a) => a.active) ?? [...mine].sort((a, b) => a.starts_on.localeCompare(b.starts_on))[0];
}

/**
 * The people userId stands in for today: active absences naming them as
 * delegate. Nobody, while userId is away themselves (no chains).
 */
export function standingInFor(list: readonly Absence[] | undefined, userId: string | undefined): Absence[] {
  if (!list || !userId) return [];
  if (list.some((a) => a.user.id === userId && a.active)) return [];
  return list.filter((a) => a.active && a.delegate?.id === userId);
}

/** "Elmar, Nəzirə": display names joined for a sentence. */
export const joinNames = (people: readonly { display_name: string }[]): string => people.map((p) => p.display_name).join(", ");
