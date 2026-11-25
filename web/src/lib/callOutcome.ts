import { tlg } from "../i18n-ledger";
import { formatDate } from "./format";

export type Outcome = "answered" | "no_answer" | "promised" | "wrong_number";
export const OUTCOMES: readonly Outcome[] = ["answered", "no_answer", "promised", "wrong_number"];

/** The comment a call outcome writes (ADR-0050): an emoji, the outcome and, for a promise, the date. */
export function outcomeComment(o: Outcome, promisedOn?: string): string {
  const label = tlg.outcome[o];
  return o === "promised" && promisedOn ? `📞 ${label}: ${formatDate(promisedOn)}` : `📞 ${label}`;
}
