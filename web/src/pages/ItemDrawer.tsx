import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { ApiError } from "../api/client";
import { keys, useAssignable, useDeleteItem, useMe, useFields, useItem, useItemTypes, useMembers, useTransitions, useUpdateItem, type ItemPatch } from "../api/hooks";
import type { FieldValue, Item } from "../api/types";
import { ActivityFeed } from "../components/ActivityFeed";
import { Comments } from "../components/Comments";
import { FieldEditor } from "../components/FieldEditor";
import { useToast } from "../components/toast-context";
import { Avatar, Button, DueChip, ErrorState, Label, Select, Spinner, StatusBadge, touchField } from "../components/ui";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { formatDateTime } from "../lib/format";
import { useKeyboardInset } from "../lib/responsive";

export function ItemDrawer({ ws, project, n, canEdit, isAdmin, onClose }: {
  ws: string; project: string; n: number; canEdit: boolean; isAdmin: boolean; onClose: () => void;
}) {
  const item = useItem(ws, project, n);
  const panel = useRef<HTMLElement>(null);
  // iOS does not shrink the layout for the on-screen keyboard: lift the
  // panel's bottom edge above it, then keep the focused field in view.
  const inset = useKeyboardInset();
  useEffect(() => {
    const el = document.activeElement;
    if (inset > 0 && el instanceof HTMLElement && panel.current?.contains(el)) el.scrollIntoView({ block: "nearest" });
  }, [inset]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => { window.removeEventListener("keydown", onKey); };
  }, [onClose]);

  return (
    <>
      <div className="fixed inset-0 z-30 bg-zinc-950/20" onClick={onClose} aria-hidden />
      {/* Full screen on phones, a side panel from md up. Safe-area padding
          keeps content clear of the notch and the home indicator. */}
      <aside
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-label={item.data?.key ?? t.loading}
        style={inset > 0 ? { bottom: inset } : undefined}
        className="fixed inset-y-0 right-0 z-40 flex w-full max-w-xl flex-col overflow-y-auto overscroll-contain bg-white pr-[env(safe-area-inset-right)] pb-[env(safe-area-inset-bottom)] shadow-2xl md:pr-0 dark:bg-zinc-900"
      >
        {item.isPending ? (
          <Spinner />
        ) : item.isError ? (
          <ErrorState error={item.error} />
        ) : (
          <ItemDetails key={item.data.id} ws={ws} project={project} item={item.data} canEdit={canEdit} isAdmin={isAdmin} onClose={onClose} />
        )}
      </aside>
    </>
  );
}

function ItemDetails({ ws, project, item, canEdit, isAdmin, onClose }: {
  ws: string; project: string; item: Item; canEdit: boolean; isAdmin: boolean; onClose: () => void;
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const update = useUpdateItem(ws, project);
  const del = useDeleteItem(ws, project);
  const members = useMembers(ws);
  const assignable = useAssignable(ws, project);
  const types = useItemTypes(ws);
  const fields = useFields(ws);
  const transitions = useTransitions(ws, project, item.number);
  const [title, setTitle] = useState(item.title);
  const [desc, setDesc] = useState(item.description ?? "");
  const [due, setDue] = useState(item.due_date ?? "");

  const memberName = (id: string | null) => (id ? (members.data?.find((m) => m.user_id === id)?.display_name ?? "…") : null);

  // Edits are applied one after another, each with the version returned by the
  // previous one. Without this, two quick edits (status, then title) both send
  // the same old version and the second is rejected as a conflict with
  // yourself. Real conflicts with other people are still detected.
  const version = useRef(item.version);
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  useEffect(() => {
    version.current = Math.max(version.current, item.version);
  }, [item.version]);

  const onSaveError = (err: unknown) => {
    if (err instanceof ApiError && err.isConflict) {
      toast("error", t.conflict);
      void qc.invalidateQueries({ queryKey: keys.item(ws, project, item.number) });
    } else if (err instanceof ApiError && err.field === "status_id") {
      toast("error", t.transitionNotAllowed);
    } else {
      toast("error", err instanceof ApiError ? err.message : t.errorGeneric);
    }
  };

  const save = (patch: Omit<ItemPatch, "version">) => {
    queue.current = queue.current.then(async () => {
      try {
        const saved = await update.mutateAsync({ n: item.number, patch: { ...patch, version: version.current } });
        version.current = saved.version;
        void qc.invalidateQueries({ queryKey: keys.transitions(ws, project, item.number) });
      } catch (err) {
        onSaveError(err);
      }
    });
  };

  const type = types.data?.find((x) => x.id === item.type.id);
  const boundFields = [...(type?.fields ?? [])].sort((a, b) => a.position - b.position);
  const defs = new Map((fields.data ?? []).map((f) => [f.key, f]));
  const statusOptions = [item.status, ...(transitions.data ?? [])];

  return (
    <div className="flex flex-col">
      {/* Sticky, so closing never needs scrolling back up on a phone. */}
      <div className="sticky top-0 z-10 flex items-center justify-between gap-2 border-b border-zinc-200 bg-white px-4 pt-[max(0.5rem,env(safe-area-inset-top))] pb-2 md:px-5 md:py-3 dark:border-zinc-800 dark:bg-zinc-900">
        <span className="min-w-0 truncate font-mono text-sm text-zinc-500">
          {item.key} · {item.type.name}
        </span>
        <div className="flex shrink-0 gap-2">
          {canEdit && (
            <Button
              variant="ghost"
              className="text-red-600"
              onClick={() => {
                if (!window.confirm(t.confirmDelete)) return;
                del.mutate(item.number, {
                  onSuccess: () => {
                    toast("success", t.deleted);
                    onClose();
                  },
                  onError: () => { toast("error", t.errorGeneric); },
                });
              }}
            >
              {t.delete}
            </Button>
          )}
          <Button variant="ghost" onClick={onClose} aria-label={t.close} className="min-w-11 text-lg md:min-w-0 md:text-sm">
            ✕
          </Button>
        </div>
      </div>

      <div className="space-y-5 p-4 md:p-5">
        <textarea
          aria-label={t.title}
          value={title}
          disabled={!canEdit}
          rows={1}
          maxLength={500}
          onChange={(e) => { setTitle(e.target.value); }}
          onBlur={() => {
            if (title.trim() && title !== item.title) save({ title });
            else setTitle(item.title);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              e.currentTarget.blur();
            }
          }}
          className="w-full resize-none rounded-md bg-transparent text-xl font-semibold [overflow-wrap:anywhere] outline-none hover:bg-zinc-50 focus:bg-zinc-50 dark:hover:bg-zinc-800 dark:focus:bg-zinc-800"
        />

        <dl className="grid grid-cols-[6.5rem_minmax(0,1fr)] items-center gap-x-3 gap-y-3 text-sm sm:grid-cols-[8rem_minmax(0,1fr)] sm:gap-x-4">
          <dt className="text-zinc-500">{t.status}</dt>
          <dd>
            {canEdit ? (
              <Select
                aria-label={t.status}
                value={item.status.id}
                onChange={(e) => { if (e.target.value !== item.status.id) save({ status_id: e.target.value }); }}
                className="max-w-56"
              >
                {statusOptions.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </Select>
            ) : (
              <StatusBadge name={item.status.name} category={item.status.category} />
            )}
          </dd>

          <dt className="text-zinc-500">{t.assignee}</dt>
          <dd>
            {canEdit ? (
              <Select
                aria-label={t.assignee}
                value={item.assignee_id ?? ""}
                onChange={(e) => { save({ assignee_id: e.target.value || null }); }}
                className="max-w-56"
              >
                <option value="">{t.unassigned}</option>
                {assignable.map((m) => (
                  <option key={m.user_id} value={m.user_id}>
                    {m.display_name}
                  </option>
                ))}
              </Select>
            ) : (
              <span>{memberName(item.assignee_id) ?? t.unassigned}</span>
            )}
          </dd>

          <dt className="text-zinc-500">
            <label htmlFor="item-due">{t.dueDate}</label>
          </dt>
          <dd className="flex flex-wrap items-center gap-2">
            {canEdit ? (
              <>
                <input
                  id="item-due"
                  type="date"
                  min="1900-01-01"
                  max="2200-12-31"
                  value={due}
                  onChange={(e) => { setDue(e.target.value); }}
                  // Saved on blur: typing a date fires change events for
                  // half-typed values (year 0002) that must not be stored.
                  onBlur={() => {
                    if (due !== (item.due_date ?? "")) save({ due_date: due || null });
                  }}
                  className={cn(
                    touchField,
                    "rounded-md bg-white px-2 py-1 text-sm ring-1 ring-zinc-300 focus:ring-2 focus:ring-brand-500 dark:bg-zinc-900 dark:ring-zinc-700",
                    item.overdue && "text-red-700 ring-red-300 dark:text-red-300",
                  )}
                />
                {item.due_date && (
                  <Button variant="ghost" onClick={() => { setDue(""); save({ due_date: null }); }}>
                    {t.clearDate}
                  </Button>
                )}
                {item.overdue && <span className="text-xs font-medium text-red-700 dark:text-red-300">⏰ {t.overdue}</span>}
              </>
            ) : item.due_date ? (
              <DueChip date={item.due_date} overdue={item.overdue} />
            ) : (
              "—"
            )}
          </dd>

          <dt className="text-zinc-500">{t.reporter}</dt>
          <dd className="flex items-center gap-2">
            {item.reporter_id && <Avatar name={memberName(item.reporter_id) ?? "?"} />}
            {memberName(item.reporter_id) ?? "—"}
          </dd>

          <dt className="text-zinc-500">{t.updated}</dt>
          <dd className="text-zinc-600 dark:text-zinc-400">{formatDateTime(item.updated_at)}</dd>
        </dl>

        <div>
          <Label htmlFor="item-desc">{t.description}</Label>
          <textarea
            id="item-desc"
            rows={6}
            value={desc}
            disabled={!canEdit}
            maxLength={100_000}
            onChange={(e) => { setDesc(e.target.value); }}
            onBlur={() => {
              if (desc !== (item.description ?? "")) save({ description: desc.trim() ? desc : null });
            }}
            className="w-full rounded-md bg-white px-3 py-2 text-sm whitespace-pre-wrap ring-1 pointer-coarse:text-base ring-zinc-300 focus:ring-2 focus:ring-brand-500 dark:bg-zinc-900 dark:ring-zinc-700"
          />
        </div>

        {boundFields.length > 0 && (
          <section>
            <h3 className="mb-3 text-sm font-semibold">{t.customFields}</h3>
            <div className="grid grid-cols-[6.5rem_minmax(0,1fr)] items-center gap-x-3 gap-y-3 text-sm sm:grid-cols-[8rem_minmax(0,1fr)] sm:gap-x-4">
              {boundFields.map((b) => {
                const def = defs.get(b.key);
                if (!def) return null;
                const id = `field-${b.key}`;
                return (
                  <FieldRow key={b.key} id={id} label={def.name + (b.required ? " *" : "")}>
                    <FieldEditor
                      id={id}
                      def={def}
                      value={item.fields[b.key]}
                      members={members.data ?? []}
                      disabled={!canEdit || def.archived}
                      onCommit={(v: FieldValue) => { save({ fields: { [b.key]: v } }); }}
                    />
                  </FieldRow>
                );
              })}
            </div>
          </section>
        )}

        <CollabTabs ws={ws} project={project} n={item.number} canWrite={canEdit} isAdmin={isAdmin} />
      </div>
    </div>
  );
}

function FieldRow({ id, label, children }: { id: string; label: string; children: React.ReactNode }) {
  return (
    <>
      <label htmlFor={id} className="text-zinc-500">
        {label}
      </label>
      <div>{children}</div>
    </>
  );
}

function CollabTabs({ ws, project, n, canWrite, isAdmin }: { ws: string; project: string; n: number; canWrite: boolean; isAdmin: boolean }) {
  const [tab, setTab] = useState<"comments" | "activity">("comments");
  const members = useMembers(ws);
  const mentionable = useAssignable(ws, project);
  const me = useMe();
  return (
    <section className="border-t border-zinc-200 pt-4 dark:border-zinc-800">
      <div role="tablist" className="mb-4 flex gap-4 text-sm">
        {(["comments", "activity"] as const).map((x) => (
          <button
            key={x}
            type="button"
            role="tab"
            aria-selected={tab === x}
            onClick={() => { setTab(x); }}
            className={
              "-mb-px border-b-2 pb-1.5 font-medium pointer-coarse:min-h-11 " +
              (tab === x ? "border-brand-600 text-zinc-900 dark:text-zinc-100" : "border-transparent text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200")
            }
          >
            {x === "comments" ? t.comments : t.activity}
          </button>
        ))}
      </div>
      {tab === "comments" ? (
        <Comments ws={ws} project={project} n={n} members={members.data ?? []} mentionable={mentionable} meId={me.data?.id} canWrite={canWrite} isAdmin={isAdmin} />
      ) : (
        <ActivityFeed ws={ws} project={project} n={n} />
      )}
    </section>
  );
}
