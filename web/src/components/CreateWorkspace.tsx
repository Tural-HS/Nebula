import { useState, type SyntheticEvent } from "react";
import { useNavigate } from "react-router";

import { useCreateWorkspace } from "../api/hooks";
import { Button, Dialog, FieldError, Input, Label, Select } from "./ui";
import { t } from "../i18n";
import { tk } from "../i18n-kinds";
import { tt } from "../i18n-templates";
import { activeLocale, isLocale, LOCALES, type Locale } from "../i18n/locale";
import { createWorkspaceBody, isWorkspaceKind, kindNotes, WORKSPACE_KINDS, type WorkspaceKind } from "../lib/workspaceKinds";

// Creating a workspace with a template and a team kind (ADR-0020, ADR-0033),
// from the home page header or its empty state (ADR-0037).

/** The header button: "+" on phones, so a long label never pushes the header out (ADR-0019). */
export function NewWorkspaceButton({ onClick }: { onClick: () => void }) {
  return (
    <Button variant="primary" aria-label={t.newWorkspace} className="pointer-coarse:min-w-11" onClick={onClick}>
      <span className="max-sm:hidden">{t.newWorkspace}</span>
      <span aria-hidden className="text-lg leading-none sm:hidden">+</span>
    </Button>
  );
}

export function CreateWorkspaceDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  return (
    <Dialog open={open} onClose={onClose} title={t.newWorkspace}>
      <CreateWorkspaceForm onDone={onClose} />
    </Dialog>
  );
}

// Each template is named in its own language with its first statuses, so the
// labels need no translation (ADR-0020, ADR-0025).
const TEMPLATE_LABELS: Record<Locale, string> = {
  az: "Azərbaycanca (Görüləcək → İcrada → Yoxlamada → Tamamlandı)",
  en: "English (To Do → In Progress → Done)",
  tr: "Türkçe (Yapılacak → Devam ediyor → İncelemede → Tamamlandı)",
  ru: "Русский (К выполнению → В работе → На проверке → Готово)",
};

function CreateWorkspaceForm({ onDone }: { onDone: () => void }) {
  const create = useCreateWorkspace();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [template, setTemplate] = useState<Locale>(activeLocale);
  const [kind, setKind] = useState<WorkspaceKind>("general");
  const notes = kindNotes(kind, template);

  const submit = (e: SyntheticEvent) => {
    e.preventDefault();
    create.mutate(
      createWorkspaceBody({ name, slug, template, kind }),
      {
        onSuccess: (w) => {
          onDone();
          void navigate(`/w/${encodeURIComponent(w.slug)}`);
        },
      },
    );
  };

  return (
    <form onSubmit={submit} className="space-y-4">
      <div>
        <Label htmlFor="ws-name">{t.name}</Label>
        <Input id="ws-name" value={name} onChange={(e) => { setName(e.target.value); }} required maxLength={200} autoFocus />
      </div>
      <div>
        <Label htmlFor="ws-slug">{t.slug}</Label>
        <Input id="ws-slug" value={slug} onChange={(e) => { setSlug(e.target.value.toLowerCase()); }} required pattern="[a-z0-9][a-z0-9\-]{1,62}" />
        <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{t.slugHint}</p>
      </div>
      <div>
        <Label htmlFor="ws-template">{tt.template}</Label>
        <Select id="ws-template" value={template} onChange={(e) => { if (isLocale(e.target.value)) setTemplate(e.target.value); }}>
          {LOCALES.map((l) => (
            <option key={l} value={l} lang={l}>
              {TEMPLATE_LABELS[l]}
            </option>
          ))}
        </Select>
        <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{tt.templateHint}</p>
      </div>
      {/* Team kind (ADR-0033): one column of cards on phones, two from sm up. */}
      <fieldset>
        <legend className="mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400">{tk.kind}</legend>
        <div className="grid gap-2 sm:grid-cols-2">
          {WORKSPACE_KINDS.map((k) => (
            <label
              key={k}
              className="flex cursor-pointer gap-2 rounded-lg p-3 ring-1 ring-zinc-200 has-[:checked]:bg-indigo-50 has-[:checked]:ring-2 has-[:checked]:ring-indigo-500 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-indigo-500 dark:ring-zinc-700 dark:has-[:checked]:bg-indigo-950/40"
            >
              <input
                type="radio"
                name="ws-kind"
                value={k}
                checked={kind === k}
                onChange={(e) => { if (isWorkspaceKind(e.target.value)) setKind(e.target.value); }}
                className="mt-0.5 size-4 shrink-0 accent-indigo-600"
              />
              <span className="min-w-0 [overflow-wrap:anywhere]">
                <span className="block text-sm font-medium">{tk.names[k]}</span>
                <span className="block text-xs text-zinc-500 dark:text-zinc-400">{tk.descriptions[k]}</span>
              </span>
            </label>
          ))}
        </div>
        {notes.approval && <p className="mt-2 text-xs text-amber-700 dark:text-amber-400">{tk.approvalNote}</p>}
        {notes.calendar && <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{tk.calendarNote}</p>}
        <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{tk.kindHint}</p>
      </fieldset>
      <FieldError error={create.error} />
      <div className="flex justify-end gap-2">
        <Button onClick={onDone}>{t.cancel}</Button>
        <Button type="submit" variant="primary" disabled={create.isPending}>
          {t.create}
        </Button>
      </div>
    </form>
  );
}
