import { useId, useState } from "react";

import { useCommentMutations } from "../api/hooks";
import { tlg } from "../i18n-ledger";
import { OUTCOMES, outcomeComment, type Outcome } from "../lib/callOutcome";
import { Button, Input } from "./ui";
import { useToast } from "./toast-context";

/**
 * One-click call results for ledger records (ADR-0050): each writes a
 * comment, so the record keeps a call history in Comments and Activity;
 * "promised to pay" also moves the due date to the promised day.
 */
export function CallOutcome({ ws, project, n, onPromise }: {
  ws: string;
  project: string;
  n: number;
  /** Sets the due date to the promised day. */
  onPromise: (date: string) => void;
}) {
  const m = useCommentMutations(ws, project, n);
  const toast = useToast();
  const [promising, setPromising] = useState(false);
  const [date, setDate] = useState("");
  const headingId = useId();
  const dateId = useId();
  const post = (o: Outcome, on?: string) => {
    m.create.mutate(outcomeComment(o, on), {
      onSuccess: () => { toast("success", tlg.outcomeSaved(tlg.outcome[o])); },
      onError: () => { toast("error", tlg.outcomeFailed); },
    });
  };
  return (
    <section aria-labelledby={headingId} className="rounded-lg border border-zinc-200 p-3 dark:border-zinc-800">
      <h3 id={headingId} className="mb-2 text-xs font-semibold text-zinc-500 dark:text-zinc-400">{tlg.callOutcome}</h3>
      <div className="flex flex-wrap gap-2">
        {OUTCOMES.map((o) => (
          <Button
            key={o}
            disabled={m.create.isPending}
            aria-pressed={o === "promised" ? promising : undefined}
            onClick={() => { if (o === "promised") setPromising((p) => !p); else post(o); }}
          >
            {tlg.outcomeIcon[o]} {tlg.outcome[o]}
          </Button>
        ))}
      </div>
      {promising && (
        <form
          className="mt-3 flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (!date) return;
            post("promised", date);
            onPromise(date);
            setPromising(false);
            setDate("");
          }}
        >
          <label htmlFor={dateId} className="text-xs text-zinc-500 dark:text-zinc-400">
            {tlg.promisedOn}
            <Input id={dateId} type="date" required min="2000-01-01" max="2200-12-31" value={date} autoFocus onChange={(e) => { setDate(e.target.value); }} />
          </label>
          <Button type="submit" variant="primary" disabled={!date || m.create.isPending}>{tlg.savePromise}</Button>
        </form>
      )}
    </section>
  );
}
