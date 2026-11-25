import { useId, useState, type KeyboardEvent, type SyntheticEvent } from "react";

import { NOTE_COLORS, useNoteApi, useNotes, type Note, type NoteColor } from "../api/notes";
import { thm } from "../i18n-home";
import { cn } from "../lib/cn";
import { useToast } from "./toast-context";
import { Button } from "./ui";

// Personal sticky notes on the home page (ADR-0052): a row of coloured
// cards; pinned ones first. Only their author sees them.

const SHOWN = 8;

/** Paper colour per note; text stays zinc-900 / zinc-100, AA on every one. */
const PAPER: Record<NoteColor, string> = {
  yellow: "bg-amber-100 ring-amber-300/70 dark:bg-amber-950/60 dark:ring-amber-800/60",
  green: "bg-emerald-100 ring-emerald-300/70 dark:bg-emerald-950/60 dark:ring-emerald-800/60",
  blue: "bg-sky-100 ring-sky-300/70 dark:bg-sky-950/60 dark:ring-sky-800/60",
  pink: "bg-pink-100 ring-pink-300/70 dark:bg-pink-950/60 dark:ring-pink-800/60",
  purple: "bg-violet-100 ring-violet-300/70 dark:bg-violet-950/60 dark:ring-violet-800/60",
};
const SWATCH: Record<NoteColor, string> = {
  yellow: "bg-amber-300", green: "bg-emerald-400", blue: "bg-sky-400", pink: "bg-pink-400", purple: "bg-violet-400",
};

export function StickyNotes() {
  const notes = useNotes();
  const napi = useNoteApi();
  const toast = useToast();
  const [adding, setAdding] = useState(false);
  const [all, setAll] = useState(false);
  const id = useId();
  if (notes.isError) return null; // the home page works without them
  const list = notes.data ?? [];
  const shown = all ? list : list.slice(0, SHOWN);

  const remove = (n: Note) => {
    napi.remove.mutate(n.id, {
      onSuccess: () => {
        toast("success", thm.notes.removed, {
          label: thm.notes.undo,
          run: () => { napi.create.mutate({ body: n.body, color: n.color, pinned: n.pinned }); },
        });
      },
      onError: () => { toast("error", thm.notes.failed); },
    });
  };

  return (
    <section aria-labelledby={`${id}-h`} className="space-y-2">
      <div className="flex items-baseline gap-2">
        <h2 id={`${id}-h`} className="text-sm font-semibold">{thm.notes.title}</h2>
        <span className="text-xs text-zinc-500 dark:text-zinc-400">{thm.notes.hint}</span>
        {!adding && (
          <Button variant="ghost" className="ml-auto" onClick={() => { setAdding(true); }}>{thm.notes.add}</Button>
        )}
      </div>
      {list.length === 0 && !adding && notes.isSuccess && (
        <p className="text-sm text-zinc-500 dark:text-zinc-400">{thm.notes.empty}</p>
      )}
      {(list.length > 0 || adding) && (
        <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {adding && (
            <li>
              <NoteEditor
                initial=""
                color="yellow"
                pending={napi.create.isPending}
                onCancel={() => { setAdding(false); }}
                onSave={(body, color) => {
                  napi.create.mutate({ body, color }, {
                    onSuccess: () => { setAdding(false); },
                    onError: () => { toast("error", thm.notes.failed); },
                  });
                }}
              />
            </li>
          )}
          {shown.map((n) => (
            <li key={n.id}>
              <NoteCard
                note={n}
                onSave={(body, color) => { napi.update.mutate({ id: n.id, body: { body, color } }, { onError: () => { toast("error", thm.notes.failed); } }); }}
                onPin={() => { napi.update.mutate({ id: n.id, body: { pinned: !n.pinned } }, { onError: () => { toast("error", thm.notes.failed); } }); }}
                onRemove={() => { remove(n); }}
              />
            </li>
          ))}
        </ul>
      )}
      {list.length > SHOWN && (
        <Button variant="ghost" onClick={() => { setAll((v) => !v); }}>{all ? thm.notes.less : thm.notes.more(list.length - SHOWN)}</Button>
      )}
    </section>
  );
}

function NoteCard({ note, onSave, onPin, onRemove }: {
  note: Note;
  onSave: (body: string, color: NoteColor) => void;
  onPin: () => void;
  onRemove: () => void;
}) {
  const [editing, setEditing] = useState(false);
  if (editing) {
    return (
      <NoteEditor
        initial={note.body}
        color={note.color}
        pending={false}
        onCancel={() => { setEditing(false); }}
        onSave={(body, color) => { onSave(body, color); setEditing(false); }}
      />
    );
  }
  const preview = note.body.length > 60 ? `${note.body.slice(0, 60)}…` : note.body;
  return (
    <article
      aria-label={preview}
      className={cn("group flex h-full min-h-28 flex-col rounded-lg p-3 shadow-sm ring-1", PAPER[note.color])}
    >
      <p className="flex-1 text-sm whitespace-pre-wrap text-zinc-900 [overflow-wrap:anywhere] dark:text-zinc-100">
        {note.pinned && <span className="mr-1" role="img" aria-label={thm.notes.pin}>📌</span>}
        {note.body}
      </p>
      <div className="mt-2 flex gap-1 text-xs opacity-80 group-focus-within:opacity-100 group-hover:opacity-100">
        <button type="button" className="rounded px-1.5 py-0.5 text-zinc-700 hover:bg-black/5 focus-visible:outline-2 focus-visible:outline-brand-500 dark:text-zinc-300 dark:hover:bg-white/10" onClick={() => { setEditing(true); }}>
          {thm.notes.edit}
        </button>
        <button type="button" aria-pressed={note.pinned} className="rounded px-1.5 py-0.5 text-zinc-700 hover:bg-black/5 focus-visible:outline-2 focus-visible:outline-brand-500 dark:text-zinc-300 dark:hover:bg-white/10" onClick={onPin}>
          {note.pinned ? thm.notes.unpin : thm.notes.pin}
        </button>
        <button type="button" className="ml-auto rounded px-1.5 py-0.5 text-red-800 hover:bg-black/5 focus-visible:outline-2 focus-visible:outline-brand-500 dark:text-red-300 dark:hover:bg-white/10" onClick={onRemove}>
          {thm.notes.remove}
        </button>
      </div>
    </article>
  );
}

function NoteEditor({ initial, color: initialColor, pending, onSave, onCancel }: {
  initial: string;
  color: NoteColor;
  pending: boolean;
  onSave: (body: string, color: NoteColor) => void;
  onCancel: () => void;
}) {
  const [body, setBody] = useState(initial);
  const [color, setColor] = useState<NoteColor>(initialColor);
  const id = useId();
  const submit = (e: SyntheticEvent) => {
    e.preventDefault();
    if (body.trim()) onSave(body.trim(), color);
  };
  const keys = (e: KeyboardEvent) => {
    if (e.key === "Escape") onCancel();
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) submit(e);
  };
  return (
    <form onSubmit={submit} className={cn("flex h-full min-h-28 flex-col gap-2 rounded-lg p-3 shadow-sm ring-1", PAPER[color])}>
      <textarea
        aria-label={thm.notes.label}
        className="min-h-20 flex-1 resize-y rounded bg-white/60 p-1.5 text-sm text-zinc-900 outline-none focus-visible:ring-2 focus-visible:ring-brand-500 dark:bg-black/20 dark:text-zinc-100"
        placeholder={thm.notes.placeholder}
        maxLength={2000}
        value={body}
        autoFocus
        onChange={(e) => { setBody(e.target.value); }}
        onKeyDown={keys}
      />
      <fieldset className="flex items-center gap-1.5">
        <legend className="sr-only">{thm.notes.color}</legend>
        {NOTE_COLORS.map((c) => (
          <label key={c} className="relative">
            <input type="radio" name={`${id}-color`} value={c} checked={color === c} onChange={() => { setColor(c); }} className="peer sr-only" />
            <span className={cn("block size-5 cursor-pointer rounded-full ring-1 ring-black/20 peer-checked:ring-2 peer-checked:ring-zinc-900 peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-brand-500 dark:peer-checked:ring-zinc-100", SWATCH[c])} />
            <span className="sr-only">{thm.notes.colors[c]}</span>
          </label>
        ))}
        <span className="ml-auto flex gap-1">
          <Button type="button" variant="ghost" onClick={onCancel}>{thm.notes.cancel}</Button>
          <Button type="submit" variant="primary" disabled={!body.trim() || pending}>{thm.notes.save}</Button>
        </span>
      </fieldset>
    </form>
  );
}
