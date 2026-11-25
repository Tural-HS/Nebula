import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { BulkFailedError, postBulk, useBulkItems, type BulkChange, type BulkItemError } from "../api/bulk";
import { ApiError } from "../api/client";
import { keys } from "../api/hooks";
import type { FieldDefinition, FieldValue, Item, Member, Status } from "../api/types";
import { useToast } from "../components/toast-context";
import { Button, Dialog, Input, Select } from "../components/ui";
import { t } from "../i18n";
import { tc } from "../i18n-calendar";
import { undoPlan } from "../lib/bulkUndo";
import { parseDraft } from "../lib/inline";

// ---------------------------------------------------------------------------
// Bulk toolbar (ADR-0023): one change for every selected row. The server
// applies it to all of them or to none; when some item fails, the dialog
// lists each failing item and why, and nothing was changed.
// ---------------------------------------------------------------------------

const BULK_FIELD_TYPES = new Set<FieldDefinition["type"]>(["select", "user", "number", "text", "date", "url", "email", "boolean"]);

export function BulkBar({ ws, project, projectKey, selected, statuses, assignable, fields, isAdmin, onClear }: {
  ws: string;
  project: string;
  projectKey: string;
  selected: Item[];
  statuses: Status[];
  assignable: Member[];
  fields: ReadonlyMap<string, FieldDefinition>;
  isAdmin: boolean;
  onClear: () => void;
}) {
  const toast = useToast();
  const qc = useQueryClient();
  const bulk = useBulkItems(ws, project);
  const [due, setDue] = useState("");
  const [fieldKey, setFieldKey] = useState("");
  const [fieldValue, setFieldValue] = useState("");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [failed, setFailed] = useState<{ items: BulkItemError[]; conflict: boolean } | null>(null);
  const editable = [...fields.values()].filter((f) => !f.archived && BULK_FIELD_TYPES.has(f.type));
  const def = fields.get(fieldKey);

  const run = (change: BulkChange) => {
    if (selected.length === 0) return;
    bulk.mutate(
      { targets: selected, change },
      {
        onSuccess: (res) => {
          const steps = undoPlan(change, selected, res.items);
          const undo = steps.length === 0 ? undefined : {
            label: tc.undo,
            run: () => {
              void (async () => {
                try {
                  for (const st of steps) await postBulk(ws, project, st.targets, st.change);
                  toast("success", tc.bulkUndone(res.changed));
                } catch {
                  toast("error", tc.bulkUndoFailed);
                } finally {
                  void qc.invalidateQueries({ queryKey: keys.items(ws, project) });
                }
              })();
            },
          };
          toast("success", change.delete ? tc.bulkDeleted(res.changed) : tc.bulkDone(res.changed), undo);
          onClear();
          setConfirmDelete(false);
        },
        onError: (err) => {
          setConfirmDelete(false);
          if (err instanceof BulkFailedError) setFailed({ items: err.items, conflict: err.isConflict });
          else toast("error", err instanceof ApiError ? err.message : t.errorGeneric);
        },
      },
    );
  };

  const applyField = () => {
    if (!def) return;
    let v: FieldValue | undefined;
    if (def.type === "boolean") v = fieldValue === "" ? null : fieldValue === "true";
    else if (def.type === "select" || def.type === "user") v = fieldValue || null;
    else v = parseDraft(def.type === "number" ? "number" : "text", fieldValue);
    if (v === undefined) {
      toast("error", t.errorGeneric);
      return;
    }
    run({ fields: { [def.key]: v } });
  };

  const busy = bulk.isPending;
  return (
    <div
      role="toolbar"
      aria-label={tc.bulkBar}
      data-testid="bulk-bar"
      className="flex flex-wrap items-center gap-2 border-b border-brand-200 bg-brand-50 px-4 py-2 text-sm dark:border-brand-900 dark:bg-brand-950/40"
    >
      <span role="status" className="font-medium tabular-nums">{tc.selected(selected.length)}</span>
      <Select
        aria-label={tc.setStatus}
        value=""
        disabled={busy}
        className="max-w-48"
        onChange={(e) => { if (e.target.value) run({ status_id: e.target.value }); }}
      >
        <option value="">{tc.setStatus}</option>
        {statuses.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
      </Select>
      <Select
        aria-label={tc.setAssignee}
        value=""
        disabled={busy}
        className="max-w-52"
        onChange={(e) => {
          const v = e.target.value;
          if (v) run({ assignee_id: v === "none" ? null : v });
        }}
      >
        <option value="">{tc.setAssignee}</option>
        <option value="none">{t.unassigned}</option>
        {assignable.map((m) => <option key={m.user_id} value={m.user_id}>{m.display_name}</option>)}
      </Select>
      <form
        className="flex items-center gap-1"
        onSubmit={(e) => {
          e.preventDefault();
          if (due) run({ due_date: due });
        }}
      >
        <Input type="date" aria-label={tc.setDue} value={due} disabled={busy} onChange={(e) => { setDue(e.target.value); }} className="w-40" />
        <Button type="submit" disabled={busy || !due}>{tc.apply}</Button>
        <Button type="button" disabled={busy} onClick={() => { run({ due_date: null }); }}>{tc.clearDue}</Button>
      </form>
      {editable.length > 0 && (
        <form
          className="flex items-center gap-1"
          onSubmit={(e) => {
            e.preventDefault();
            applyField();
          }}
        >
          <Select aria-label={tc.setField} value={fieldKey} disabled={busy} className="max-w-44" onChange={(e) => { setFieldKey(e.target.value); setFieldValue(""); }}>
            <option value="">{tc.setField}</option>
            {editable.map((f) => <option key={f.key} value={f.key}>{f.name}</option>)}
          </Select>
          {def && <BulkFieldInput def={def} value={fieldValue} members={assignable} onChange={setFieldValue} disabled={busy} />}
          {def && <Button type="submit" disabled={busy}>{tc.apply}</Button>}
        </form>
      )}
      {isAdmin && (
        <Button variant="danger" disabled={busy} onClick={() => { setConfirmDelete(true); }}>{tc.bulkDelete}</Button>
      )}
      <Button className="ml-auto" onClick={onClear} disabled={busy}>{tc.clearSelection}</Button>

      <Dialog open={confirmDelete} onClose={() => { setConfirmDelete(false); }} title={tc.bulkDeleteTitle(selected.length)}>
        <p className="text-sm">{tc.confirmBulkDelete(selected.length)}</p>
        <div className="mt-4 flex flex-wrap justify-end gap-2">
          <Button onClick={() => { setConfirmDelete(false); }}>{t.cancel}</Button>
          <Button variant="danger" disabled={busy} onClick={() => { run({ delete: true }); }}>{tc.bulkDeleteAction(selected.length)}</Button>
        </div>
      </Dialog>
      <Dialog open={failed !== null} onClose={() => { setFailed(null); }} title={tc.bulkFailedTitle}>
        {failed && (
          <div className="text-sm" role="alert">
            <p>{tc.bulkFailed(failed.items.length)}</p>
            <ul className="mt-2 max-h-64 list-disc overflow-y-auto pl-5" data-testid="bulk-errors">
              {failed.items.map((e) => (
                <li key={e.number}>
                  <span className="font-mono">{projectKey}-{e.number}</span>: {e.message}
                </li>
              ))}
            </ul>
            {failed.conflict && <p className="mt-2 text-zinc-600 dark:text-zinc-400">{tc.bulkConflictHint}</p>}
          </div>
        )}
      </Dialog>
    </div>
  );
}

function BulkFieldInput({ def, value, members, onChange, disabled }: {
  def: FieldDefinition; value: string; members: Member[]; onChange: (v: string) => void; disabled: boolean;
}) {
  const label = `${tc.fieldValue}: ${def.name}`;
  switch (def.type) {
    case "select":
      return (
        <Select aria-label={label} value={value} disabled={disabled} className="max-w-44" onChange={(e) => { onChange(e.target.value); }}>
          <option value="">—</option>
          {(def.options.choices ?? []).filter((c) => !c.archived).map((c) => <option key={c.id} value={c.id}>{c.label}</option>)}
        </Select>
      );
    case "user":
      return (
        <Select aria-label={label} value={value} disabled={disabled} className="max-w-44" onChange={(e) => { onChange(e.target.value); }}>
          <option value="">—</option>
          {members.map((m) => <option key={m.user_id} value={m.user_id}>{m.display_name}</option>)}
        </Select>
      );
    case "boolean":
      return (
        <Select aria-label={label} value={value} disabled={disabled} className="max-w-32" onChange={(e) => { onChange(e.target.value); }}>
          <option value="">—</option>
          <option value="true">{t.yes}</option>
          <option value="false">{t.no}</option>
        </Select>
      );
    default:
      return (
        <Input
          aria-label={label}
          type={def.type === "date" ? "date" : def.type === "number" ? "text" : def.type}
          inputMode={def.type === "number" ? "decimal" : undefined}
          value={value}
          disabled={disabled}
          className="w-40"
          onChange={(e) => { onChange(e.target.value); }}
        />
      );
  }
}
