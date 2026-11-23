import { useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";

import { ApiError } from "../api/client";
import { useCommentMutations, useComments } from "../api/hooks";
import type { Comment, Member } from "../api/types";
import { t } from "../i18n";
import { flattenBy } from "../lib/collections";
import { formatRelative } from "../lib/format";
import { activeMentionQuery, toEditable, tokenize, toStored, type Person } from "../lib/mentions";
import { useToast } from "./toast-context";
import { Avatar, Button, ErrorState, Spinner } from "./ui";

interface Props {
  ws: string;
  project: string;
  n: number;
  members: Member[];
  meId: string | undefined;
  canWrite: boolean;
  isAdmin: boolean;
}

export function Comments({ ws, project, n, members, meId, canWrite, isAdmin }: Props) {
  const q = useComments(ws, project, n);
  const m = useCommentMutations(ws, project, n);
  const toast = useToast();
  const people = useMemo<Person[]>(() => members.map((x) => ({ id: x.user_id, name: x.display_name })), [members]);
  const comments = flattenBy(q.data?.pages ?? [], (p) => p.comments, (c) => c.id);

  const onError = (err: unknown) => {
    toast("error", err instanceof ApiError ? err.message : t.errorGeneric);
  };

  if (q.isPending) return <Spinner />;
  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;

  return (
    <div className="space-y-4">
      {comments.length === 0 && <p className="text-sm text-zinc-500">{t.noComments}</p>}
      <ul className="space-y-4">
        {comments.map((c) => (
          <CommentRow
            key={c.id}
            comment={c}
            people={people}
            canEdit={canWrite && c.author?.id === meId}
            canDelete={canWrite && (c.author?.id === meId || isAdmin)}
            onSave={(body) => {
              m.update.mutate({ id: c.id, body }, { onError });
            }}
            onDelete={() => {
              if (window.confirm(t.confirmDeleteComment)) m.remove.mutate(c.id, { onError });
            }}
          />
        ))}
      </ul>
      {q.hasNextPage && (
        <Button variant="ghost" onClick={() => void q.fetchNextPage()} disabled={q.isFetchingNextPage}>
          {t.loadingMore}
        </Button>
      )}
      {canWrite && (
        <Composer
          people={people}
          pending={m.create.isPending}
          onSubmit={(body, reset) => {
            m.create.mutate(body, { onSuccess: reset, onError });
          }}
        />
      )}
    </div>
  );
}

function CommentBody({ body, people }: { body: string; people: Person[] }) {
  const names = new Map(people.map((p) => [p.id.toLowerCase(), p.name]));
  return (
    <p className="text-sm break-words whitespace-pre-wrap">
      {tokenize(body).map((part, i) =>
        part.kind === "text" ? (
          <span key={i}>{part.value}</span>
        ) : (
          <span key={i} className="rounded bg-brand-50 px-1 font-medium text-brand-700 dark:bg-brand-700/30 dark:text-brand-100">
            @{names.get(part.id) ?? t.unknownPerson}
          </span>
        ),
      )}
    </p>
  );
}

function CommentRow({ comment: c, people, canEdit, canDelete, onSave, onDelete }: {
  comment: Comment; people: Person[]; canEdit: boolean; canDelete: boolean;
  onSave: (body: string) => void; onDelete: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const author = c.author?.display_name ?? t.unknownPerson;
  return (
    <li className="flex gap-3">
      <Avatar name={author} size="md" />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-x-2 text-xs text-zinc-500">
          <span className="font-medium text-zinc-800 dark:text-zinc-200">{author}</span>
          <span title={c.created_at}>{formatRelative(c.created_at)}</span>
          {c.edited && <span>· {t.edited}</span>}
          {!c.deleted && !editing && (canEdit || canDelete) && (
            <span className="ml-auto flex gap-2">
              {canEdit && (
                <button type="button" className="hover:text-zinc-900 dark:hover:text-zinc-100" onClick={() => { setEditing(true); }}>
                  {t.edit}
                </button>
              )}
              {canDelete && (
                <button type="button" className="text-red-600 hover:text-red-700" onClick={onDelete}>
                  {t.delete}
                </button>
              )}
            </span>
          )}
        </div>
        {c.deleted || c.body === null ? (
          <p className="text-sm text-zinc-400 italic">{t.commentDeleted}</p>
        ) : editing ? (
          <Composer
            people={people}
            initial={toEditable(c.body, people)}
            initialPicked={people.filter((p) => c.mentions.includes(p.id))}
            pending={false}
            onCancel={() => { setEditing(false); }}
            onSubmit={(body) => {
              onSave(body);
              setEditing(false);
            }}
          />
        ) : (
          <CommentBody body={c.body} people={people} />
        )}
      </div>
    </li>
  );
}

function Composer({ people, initial = "", initialPicked = [], pending, onSubmit, onCancel }: {
  people: Person[]; initial?: string; initialPicked?: Person[]; pending: boolean;
  onSubmit: (body: string, reset: () => void) => void; onCancel?: () => void;
}) {
  const [text, setText] = useState(initial);
  const [picked, setPicked] = useState<Person[]>(initialPicked);
  const [query, setQuery] = useState<string | null>(null);
  const [active, setActive] = useState(0);
  const ref = useRef<HTMLTextAreaElement>(null);
  // Caret to restore after a mention is inserted. Applied in a layout effect,
  // i.e. synchronously when React writes the new value: a later frame
  // (requestAnimationFrame) let fast typing land at the end first and then
  // yanked the caret back, scrambling the text ("bu gün." -> "gün.bu").
  const caretAfterPick = useRef<number | null>(null);
  useLayoutEffect(() => {
    const pos = caretAfterPick.current;
    const el = ref.current;
    if (pos === null || !el) return;
    caretAfterPick.current = null;
    el.focus();
    el.setSelectionRange(pos, pos);
  }, [text]);

  const suggestions =
    query === null ? [] : people.filter((p) => p.name.toLocaleLowerCase("az").includes(query.toLocaleLowerCase("az"))).slice(0, 6);

  const updateQuery = (value: string, caret: number) => {
    setQuery(activeMentionQuery(value, caret));
    setActive(0);
  };

  const pick = (p: Person) => {
    const el = ref.current;
    const caret = el?.selectionStart ?? text.length;
    const before = text.slice(0, caret).replace(/@[\p{L}\p{N}._-]{0,40}$/u, "@" + p.name + " ");
    const next = before + text.slice(caret);
    setText(next);
    setPicked((xs) => (xs.some((x) => x.id === p.id) ? xs : [...xs, p]));
    setQuery(null);
    caretAfterPick.current = before.length;
  };

  const submit = () => {
    if (!text.trim() || pending) return;
    onSubmit(toStored(text.trim(), picked), () => {
      setText("");
      setPicked([]);
    });
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (suggestions.length > 0) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        setActive((i) => (i + (e.key === "ArrowDown" ? 1 : suggestions.length - 1)) % suggestions.length);
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        const p = suggestions[active];
        if (p) pick(p);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setQuery(null);
        return;
      }
    }
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      submit();
    }
  };

  return (
    <div className="relative">
      <textarea
        ref={ref}
        rows={3}
        value={text}
        maxLength={20_000}
        placeholder={t.writeComment}
        aria-label={t.writeComment}
        onChange={(e) => {
          setText(e.target.value);
          updateQuery(e.target.value, e.target.selectionStart);
        }}
        onKeyDown={onKeyDown}
        onBlur={() => { setTimeout(() => { setQuery(null); }, 150); }}
        className="w-full rounded-md bg-white px-3 py-2 text-sm ring-1 ring-zinc-300 focus:ring-2 focus:ring-brand-500 dark:bg-zinc-900 dark:ring-zinc-700"
      />
      {suggestions.length > 0 && (
        <ul role="listbox" className="absolute bottom-full left-0 z-10 mb-1 w-64 overflow-hidden rounded-md bg-white shadow-lg ring-1 ring-zinc-200 dark:bg-zinc-800 dark:ring-zinc-700">
          {suggestions.map((p, i) => (
            <li
              key={p.id}
              role="option"
              aria-selected={i === active}
              onMouseDown={(e) => {
                e.preventDefault();
                pick(p);
              }}
              className={"flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm " + (i === active ? "bg-brand-50 dark:bg-brand-700/40" : "")}
            >
              <Avatar name={p.name} />
              {p.name}
            </li>
          ))}
        </ul>
      )}
      <div className="mt-2 flex justify-end gap-2">
        {onCancel && <Button onClick={onCancel}>{t.cancel}</Button>}
        <Button variant="primary" onClick={submit} disabled={pending || !text.trim()}>
          {onCancel ? t.save : t.send}
        </Button>
      </div>
    </div>
  );
}
