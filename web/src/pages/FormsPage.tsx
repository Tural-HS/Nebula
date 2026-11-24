import { useMemo, useState } from "react";
import { useParams } from "react-router";

import { useDeleteForm, useForms, useSaveForm, type Form, type FormFieldDef, type FormInput, type PublicFormField } from "../api/forms";
import { useAssignable, useFields, useItemTypes, useMe, useProjects, useWorkspace } from "../api/hooks";
import { roleAtLeast, type FieldDefinition, type ItemType } from "../api/types";
import { FormFields } from "../components/FormFields";
import { Layout } from "../components/Layout";
import { useToast } from "../components/toast-context";
import { Button, EmptyState, ErrorState, FieldError, Input, Label, Select, Spinner } from "../components/ui";
import { t } from "../i18n";
import { tf } from "../i18n-forms";
import { tp } from "../i18n-project";
import { addableFields, formLink, isBuiltin, moveField, newForm, previewTitle, requiredByType, suggestSlug, unknownPlaceholders, type Draft } from "../lib/forms";
import { slugKey } from "../lib/slug";

/** /w/:ws/p/:project/forms — the intake form builder (ADR-0032), admins only. */
export function FormsPage() {
  const { ws = "", project = "" } = useParams();
  const workspace = useWorkspace(ws);
  const projects = useProjects(ws);
  const p = projects.data?.find((x) => x.key === project);
  const base = `/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(project)}`;
  let body;
  if (workspace.isPending || projects.isPending) body = <Spinner />;
  else if (workspace.isError) body = <ErrorState error={workspace.error} />;
  else if (!p) body = <EmptyState>{t.notFound}</EmptyState>;
  else if (!roleAtLeast(workspace.data.role, "admin")) body = <EmptyState>{tp.adminOnly}</EmptyState>;
  else body = <Forms ws={ws} project={p.key} />;
  return (
    <Layout
      crumbs={[
        { label: t.workspaces, to: "/" },
        { label: workspace.data?.name ?? ws, to: `/w/${encodeURIComponent(ws)}` },
        { label: p?.name ?? project, to: base },
        { label: tp.settings, to: `${base}/settings` },
        { label: tf.section },
      ]}
    >
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-6xl space-y-4 p-4 md:p-6">{body}</div>
      </div>
    </Layout>
  );
}

function Forms({ ws, project }: { ws: string; project: string }) {
  const forms = useForms(ws, project);
  const types = useItemTypes(ws);
  const fields = useFields(ws);
  const [editing, setEditing] = useState<Form | "new" | null>(null);
  const toast = useToast();
  const del = useDeleteForm(ws, project);
  const save = useSaveForm(ws, project);

  if (forms.isPending || types.isPending || fields.isPending) return <Spinner />;
  if (forms.isError) return <ErrorState error={forms.error} />;
  if (types.isError || fields.isError) return <ErrorState error={types.error ?? fields.error} />;

  if (editing) {
    return (
      <Editor
        key={editing === "new" ? "new" : editing.id}
        ws={ws}
        project={project}
        form={editing === "new" ? null : editing}
        types={types.data}
        defs={fields.data}
        onDone={() => { setEditing(null); }}
      />
    );
  }

  const copy = (slug: string) => {
    void navigator.clipboard.writeText(formLink(window.location.origin, slug)).then(
      () => { toast("success", tf.linkCopied); },
      () => { toast("error", t.errorGeneric); },
    );
  };

  return (
    <section aria-labelledby="forms-h" className="mx-auto max-w-3xl space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <div className="min-w-0 flex-1">
          <h1 id="forms-h" className="text-lg font-semibold">{tf.section}</h1>
          <p className="text-sm text-zinc-500">{tf.sectionHint}</p>
        </div>
        <Button variant="primary" onClick={() => { setEditing("new"); }}>{tf.newForm}</Button>
      </div>
      {forms.data.length === 0 ? (
        <EmptyState>{tf.noForms}</EmptyState>
      ) : (
        <ul className="divide-y divide-zinc-100 rounded-xl bg-white ring-1 ring-zinc-200 dark:divide-zinc-800 dark:bg-zinc-900 dark:ring-zinc-800">
          {forms.data.map((f) => (
            <li key={f.id} className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">
                  {f.title}{" "}
                  <span className={f.enabled ? "text-xs text-emerald-700 dark:text-emerald-400" : "text-xs text-zinc-500"}>
                    · {f.enabled ? tf.enabled : tf.disabled}
                  </span>
                </p>
                <p className="truncate font-mono text-xs text-zinc-500">/f/{f.slug} · {tf.submissions(f.submissions)} · {tf.audienceOptions[f.audience]}</p>
              </div>
              <div className="flex flex-wrap gap-1">
                <Button variant="ghost" onClick={() => { copy(f.slug); }}>{tf.copyLink}</Button>
                <a href={`/f/${encodeURIComponent(f.slug)}`} className="inline-flex items-center rounded-md px-3 py-1.5 text-sm text-zinc-700 hover:bg-zinc-100 pointer-coarse:min-h-11 dark:text-zinc-300 dark:hover:bg-zinc-800">{tf.open}</a>
                <Button
                  variant="ghost"
                  disabled={save.isPending}
                  onClick={() => { save.mutate({ id: f.id, body: toInput({ ...f, enabled: !f.enabled }) }); }}
                >
                  {f.enabled ? tf.disabled : tf.enabled}
                </Button>
                <Button onClick={() => { setEditing(f); }}>{t.edit}</Button>
                <Button
                  variant="ghost"
                  className="text-red-600"
                  onClick={() => {
                    if (window.confirm(tf.deleteConfirm(f.title))) del.mutate(f.id, { onSuccess: () => { toast("success", t.deleted); } });
                  }}
                >
                  {t.delete}
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
      <FieldError error={del.error ?? save.error} />
    </section>
  );
}

function toInput(f: Form): FormInput {
  return {
    slug: f.slug, title: f.title, intro: f.intro, item_type: f.item_type.key, title_template: f.title_template, fields: f.fields,
    default_assignee_id: f.default_assignee_id, initial_status_id: f.initial_status_id, audience: f.audience, enabled: f.enabled,
    max_files: f.max_files,
  };
}

/** The preview's inputs: what the server would send to a requester. */
function previewFields(form: FormInput, defs: readonly FieldDefinition[]): PublicFormField[] {
  const byKey = new Map(defs.map((d) => [d.key, d]));
  return form.fields.flatMap((f): PublicFormField[] => {
    if (f.key === "description") return [{ ...f, type: "long_text" }];
    if (f.key === "due_date") return [{ ...f, type: "date" }];
    const d = byKey.get(f.key);
    if (!d || d.type === "user") return [];
    return [{ ...f, type: d.type, choices: (d.options.choices ?? []).filter((c) => !c.archived) }];
  });
}

function Editor({ ws, project, form, types, defs, onDone }: {
  ws: string; project: string; form: Form | null; types: ItemType[]; defs: FieldDefinition[]; onDone: () => void;
}) {
  const toast = useToast();
  const me = useMe();
  const assignable = useAssignable(ws, project);
  const save = useSaveForm(ws, project);
  const [d, setD] = useState<FormInput>(() => (form ? toInput(form) : newForm(types.find((x) => x.key === "task") ?? types[0], defs)));
  const [slugTouched, setSlugTouched] = useState(form !== null);
  const [drag, setDrag] = useState<number | null>(null);
  const [pick, setPick] = useState("");
  const [preview, setPreview] = useState<Draft>({});
  const type = types.find((x) => x.key === d.item_type);
  const required = requiredByType(type);
  const fieldName = (k: string) => (isBuiltin(k) ? tf.builtin[k] : defs.find((x) => x.key === k)?.name ?? k);
  const addable = addableFields(type, defs, d.fields, (k) => tf.builtin[k]);
  const badPlaceholders = unknownPlaceholders(d.title_template, d.fields);
  const pFields = useMemo(() => previewFields(d, defs), [d, defs]);
  const set = <K extends keyof FormInput>(k: K, v: FormInput[K]) => { setD((cur) => ({ ...cur, [k]: v })); };
  const setField = (i: number, patch: Partial<FormFieldDef>) => {
    set("fields", d.fields.map((f, j) => (j === i ? { ...f, ...patch } : f)));
  };
  const display = Object.fromEntries(
    Object.entries(preview).flatMap(([k, v]) => {
      if (typeof v === "string") {
        const choice = pFields.find((f) => f.key === k)?.choices?.find((c) => c.id === v);
        return [[k, choice?.label ?? v]];
      }
      return [];
    }),
  );

  return (
    <form
      aria-labelledby="fe-h"
      className="grid gap-6 lg:grid-cols-2"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate({ id: form?.id ?? null, body: d }, {
          onSuccess: () => { toast("success", form ? t.saved : tf.created); onDone(); },
        });
      }}
    >
      <div className="space-y-4">
        <h1 id="fe-h" className="text-lg font-semibold">{form ? tf.editForm : tf.newForm}</h1>
        <div className="space-y-4 rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
          <div>
            <Label htmlFor="fe-title">{tf.title}</Label>
            <Input
              id="fe-title"
              required
              maxLength={200}
              value={d.title}
              onChange={(e) => {
                const title = e.target.value;
                setD((cur) => ({ ...cur, title, slug: slugTouched ? cur.slug : suggestSlug(title, slugKey) }));
              }}
            />
          </div>
          <div>
            <Label htmlFor="fe-slug">{tf.slug}</Label>
            <Input id="fe-slug" required pattern="[a-z0-9][a-z0-9\-]{2,62}" value={d.slug} aria-describedby="fe-slug-hint"
              onChange={(e) => { setSlugTouched(true); set("slug", e.target.value.toLowerCase()); }} />
            <p id="fe-slug-hint" className="mt-1 text-xs text-zinc-500">{tf.slugHint}</p>
          </div>
          <div>
            <Label htmlFor="fe-intro">{tf.intro}</Label>
            <textarea id="fe-intro" rows={3} maxLength={5000} value={d.intro ?? ""}
              onChange={(e) => { set("intro", e.target.value || null); }}
              className="w-full rounded-md bg-white px-2.5 py-1.5 text-sm ring-1 ring-zinc-300 pointer-coarse:text-base dark:bg-zinc-900 dark:ring-zinc-700" />
          </div>
          <div>
            <Label htmlFor="fe-type">{tf.itemType}</Label>
            <Select id="fe-type" value={d.item_type}
              onChange={(e) => {
                const next = types.find((x) => x.key === e.target.value);
                const fresh = newForm(next, defs);
                // Keep built-ins and the labels already written; add what the new type requires.
                const keep = d.fields.filter((f) => isBuiltin(f.key) || next?.fields.some((b) => b.key === f.key));
                const merged = [...keep, ...fresh.fields.filter((f) => !keep.some((k) => k.key === f.key))];
                setD((cur) => ({ ...cur, item_type: e.target.value, fields: merged, initial_status_id: null }));
              }}>
              {types.map((x) => <option key={x.id} value={x.key}>{x.name}</option>)}
            </Select>
          </div>
        </div>

        <fieldset className="space-y-3 rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
          <legend className="text-sm font-semibold">{tf.fields}</legend>
          <p className="text-xs text-zinc-500">{tf.fieldsHint}</p>
          <ol className="space-y-2">
            {d.fields.map((f, i) => {
              const name = f.label || fieldName(f.key);
              const locked = required.has(f.key);
              return (
                <li
                  key={f.key}
                  draggable
                  onDragStart={() => { setDrag(i); }}
                  onDragOver={(e) => { e.preventDefault(); }}
                  onDrop={(e) => { e.preventDefault(); if (drag !== null) set("fields", moveField(d.fields, drag, i)); setDrag(null); }}
                  onDragEnd={() => { setDrag(null); }}
                  className={"rounded-lg p-3 ring-1 ring-zinc-200 dark:ring-zinc-700 " + (drag === i ? "opacity-50" : "")}
                >
                  <div className="mb-2 flex items-center gap-2">
                    <span aria-hidden title={tf.dragHandle(name)} className="cursor-grab text-zinc-400 select-none">⠿</span>
                    <span className="min-w-0 flex-1 truncate font-mono text-xs text-zinc-500">{fieldName(f.key)} · {f.key}</span>
                    <Button variant="ghost" aria-label={tf.moveUp(name)} disabled={i === 0} onClick={() => { set("fields", moveField(d.fields, i, i - 1)); }}>↑</Button>
                    <Button variant="ghost" aria-label={tf.moveDown(name)} disabled={i === d.fields.length - 1} onClick={() => { set("fields", moveField(d.fields, i, i + 1)); }}>↓</Button>
                    <Button variant="ghost" className="text-red-600" aria-label={tf.removeField(name)} disabled={locked} title={locked ? tf.requiredByType : undefined}
                      onClick={() => { set("fields", d.fields.filter((_, j) => j !== i)); }}>✕</Button>
                  </div>
                  <div className="grid gap-2 sm:grid-cols-2">
                    <div>
                      <Label htmlFor={`fe-l-${f.key}`}>{tf.label}</Label>
                      <Input id={`fe-l-${f.key}`} required maxLength={200} value={f.label} onChange={(e) => { setField(i, { label: e.target.value }); }} />
                    </div>
                    <div>
                      <Label htmlFor={`fe-h-${f.key}`}>{tf.help}</Label>
                      <Input id={`fe-h-${f.key}`} maxLength={1000} value={f.help ?? ""} onChange={(e) => { setField(i, { help: e.target.value }); }} />
                    </div>
                  </div>
                  <label className="mt-2 flex items-center gap-2 text-sm">
                    <input type="checkbox" className="size-4 accent-brand-600" checked={f.required || locked} disabled={locked}
                      onChange={(e) => { setField(i, { required: e.target.checked }); }} />
                    {tf.required}
                    {locked && <span className="text-xs text-zinc-500">({tf.requiredByType})</span>}
                  </label>
                </li>
              );
            })}
          </ol>
          {addable.length > 0 && (
            <div className="flex gap-2">
              <Select aria-label={tf.chooseField} value={pick} onChange={(e) => { setPick(e.target.value); }}>
                <option value="">{tf.chooseField}</option>
                {addable.map((o) => <option key={o.key} value={o.key}>{o.name}</option>)}
              </Select>
              <Button disabled={!pick} onClick={() => {
                const o = addable.find((x) => x.key === pick);
                if (o) set("fields", [...d.fields, { key: o.key, label: o.name, required: required.has(o.key) }]);
                setPick("");
              }}>{tf.addField}</Button>
            </div>
          )}
        </fieldset>

        <div className="space-y-4 rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
          <div>
            <Label htmlFor="fe-tpl">{tf.titleTemplate}</Label>
            <Input id="fe-tpl" required maxLength={500} value={d.title_template} aria-describedby="fe-tpl-hint" aria-invalid={badPlaceholders.length > 0 || undefined}
              onChange={(e) => { set("title_template", e.target.value); }} />
            <p id="fe-tpl-hint" className="mt-1 text-xs text-zinc-500">
              {tf.titleTemplateHint}
              {badPlaceholders.length > 0 && <span className="block text-red-600">{badPlaceholders.map((b) => `{{${b}}}`).join(", ")}</span>}
            </p>
          </div>
          <div>
            <Label htmlFor="fe-aud">{tf.audience}</Label>
            <Select id="fe-aud" value={d.audience} aria-describedby="fe-aud-hint" onChange={(e) => { set("audience", e.target.value === "accounts" ? "accounts" : "members"); }}>
              <option value="members">{tf.audienceOptions.members}</option>
              <option value="accounts">{tf.audienceOptions.accounts}</option>
            </Select>
            <p id="fe-aud-hint" className="mt-1 text-xs text-zinc-500">{tf.audienceHint[d.audience]}</p>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <Label htmlFor="fe-st">{tf.initialStatus}</Label>
              <Select id="fe-st" value={d.initial_status_id ?? ""} onChange={(e) => { set("initial_status_id", e.target.value || null); }}>
                <option value="">{tf.workflowInitial}</option>
                {(type?.statuses ?? []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
              </Select>
            </div>
            <div>
              <Label htmlFor="fe-as">{tf.defaultAssignee}</Label>
              <Select id="fe-as" value={d.default_assignee_id ?? ""} onChange={(e) => { set("default_assignee_id", e.target.value || null); }}>
                <option value="">{tf.none}</option>
                {assignable.map((m) => <option key={m.user_id} value={m.user_id}>{m.display_name}</option>)}
              </Select>
            </div>
            <div>
              <Label htmlFor="fe-files">{tf.maxFiles}</Label>
              <Input id="fe-files" type="number" min={0} max={10} value={d.max_files}
                onChange={(e) => { set("max_files", Math.max(0, Math.min(10, Math.trunc(Number(e.target.value) || 0)))); }} />
            </div>
            <label className="flex items-center gap-2 self-end text-sm">
              <input type="checkbox" className="size-4 accent-brand-600" checked={d.enabled} onChange={(e) => { set("enabled", e.target.checked); }} />
              {tf.isEnabled}
            </label>
          </div>
        </div>
        <FieldError error={save.error} />
        <div className="flex gap-2">
          <Button type="submit" variant="primary" disabled={save.isPending}>{t.save}</Button>
          <Button onClick={onDone}>{t.cancel}</Button>
        </div>
      </div>

      <aside aria-labelledby="fe-prev" className="space-y-3 lg:sticky lg:top-4 lg:self-start">
        <h2 id="fe-prev" className="text-sm font-semibold">{tf.preview}</h2>
        <p className="text-xs text-zinc-500">{tf.previewHint}</p>
        <div className="space-y-4 rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
          <p className="text-lg font-semibold">{d.title || "…"}</p>
          {d.intro && <p className="text-sm whitespace-pre-line text-zinc-600 dark:text-zinc-400">{d.intro}</p>}
          <FormFields fields={pFields} draft={preview} errors={{}} prefix="fp" onChange={(k, v) => { setPreview((p) => ({ ...p, [k]: v })); }} />
          <p className="border-t border-zinc-100 pt-3 text-xs text-zinc-500 dark:border-zinc-800">
            {tf.titleTemplate}: <span className="font-medium text-zinc-800 dark:text-zinc-200">{previewTitle(d.title_template, display, me.data?.display_name ?? "", d.title)}</span>
          </p>
        </div>
      </aside>
    </form>
  );
}
