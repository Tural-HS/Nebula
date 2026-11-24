import { useState } from "react";

import { useItemRequest, useSetRequestMessage } from "../api/forms";
import { t } from "../i18n";
import { tf } from "../i18n-forms";
import { formatDateTime } from "../lib/format";
import { useToast } from "./toast-context";
import { Button, FieldError } from "./ui";

/**
 * The submission behind a form-created item (ADR-0032), for the team: who
 * asked, from which form, and the one message the requester can read. Hidden
 * for items that did not come from a form and for guests (the API needs the
 * member role).
 */
export function RequestPanel({ ws, project, n, canWrite }: { ws: string; project: string; n: number; canWrite: boolean }) {
  const q = useItemRequest(ws, project, n, canWrite);
  if (!canWrite || !q.data) return null;
  const r = q.data;
  return (
    <section aria-labelledby="req-panel" className="space-y-2 rounded-lg bg-zinc-50 p-3 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
      <h3 id="req-panel" className="text-xs font-semibold tracking-wide text-zinc-500 dark:text-zinc-400 uppercase">{tf.request}</h3>
      <p className="text-sm">
        {tf.fromForm(r.form_title)} · {formatDateTime(r.submitted_at)}
      </p>
      {r.requester && (
        <p className="text-sm">
          <span className="text-zinc-500 dark:text-zinc-400">{tf.requester}: </span>
          {r.requester.display_name} <span className="text-xs text-zinc-500 dark:text-zinc-400">{r.requester.email}{r.requester.member ? "" : ` · ${tf.notMember}`}</span>
        </p>
      )}
      <MessageEditor key={r.message_at ?? "none"} ws={ws} project={project} n={n} initial={r.message ?? ""} />
      {r.message_by && r.message_at && (
        <p className="text-xs text-zinc-500 dark:text-zinc-400">{r.message_by.display_name} · {formatDateTime(r.message_at)}</p>
      )}
    </section>
  );
}

function MessageEditor({ ws, project, n, initial }: { ws: string; project: string; n: number; initial: string }) {
  const toast = useToast();
  const [text, setText] = useState(initial);
  const save = useSetRequestMessage(ws, project, n);
  const id = `req-msg-${String(n)}`;
  return (
    <div>
      <label htmlFor={id} className="mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400">{tf.messageToRequester}</label>
      <textarea
        id={id}
        rows={2}
        maxLength={5000}
        value={text}
        aria-describedby={`${id}-hint`}
        onChange={(e) => { setText(e.target.value); }}
        className="w-full rounded-md bg-white px-2.5 py-1.5 text-sm ring-1 ring-zinc-300 pointer-coarse:text-base focus:ring-2 focus:ring-brand-500 dark:bg-zinc-950 dark:ring-zinc-700"
      />
      <p id={`${id}-hint`} className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{tf.messageHint}</p>
      <Button
        className="mt-2"
        disabled={save.isPending || text.trim() === initial.trim()}
        onClick={() => {
          save.mutate(text.trim() === "" ? null : text, {
            onSuccess: () => { toast("success", tf.messageSaved); },
            onError: () => { toast("error", t.errorGeneric); },
          });
        }}
      >
        {tf.sendMessage}
      </Button>
      <FieldError error={save.error} />
    </div>
  );
}
