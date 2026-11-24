import { useState, type SyntheticEvent } from "react";
import { Link, useNavigate } from "react-router";

import { useCreateWorkspace, useWorkspaces } from "../api/hooks";
import { LanguageMenu } from "../components/LanguageMenu";
import { ThemeMenu } from "../components/ThemeMenu";
import { Layout } from "../components/Layout";
import { Button, Dialog, EmptyState, ErrorState, FieldError, Input, Label, Select, Spinner } from "../components/ui";
import { t } from "../i18n";
import { ta } from "../i18n-admin";
import { tt } from "../i18n-templates";
import { tth } from "../i18n-theme";
import { activeLocale, isLocale, LOCALES, type Locale } from "../i18n/locale";

export function WorkspacesPage() {
  const q = useWorkspaces();
  const [open, setOpen] = useState(false);
  return (
    <Layout
      crumbs={[{ label: t.workspaces }]}
      actions={
        <Button variant="primary" aria-label={t.newWorkspace} className="pointer-coarse:min-w-11" onClick={() => { setOpen(true); }}>
          <span className="max-sm:hidden">{t.newWorkspace}</span>
          <span aria-hidden className="text-lg leading-none sm:hidden">+</span>
        </Button>
      }
    >
      <div className="mx-auto max-w-3xl p-4 md:p-6">
        {q.isPending ? (
          <Spinner />
        ) : q.isError ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : q.data.length === 0 ? (
          <EmptyState>{t.noWorkspaces}</EmptyState>
        ) : (
          <ul className="divide-y divide-zinc-200 overflow-hidden rounded-xl bg-white ring-1 ring-zinc-200 dark:divide-zinc-800 dark:bg-zinc-900 dark:ring-zinc-800">
            {q.data.map((w) => (
              <li key={w.id}>
                <Link to={`/w/${encodeURIComponent(w.slug)}`} className="flex items-center justify-between gap-3 px-4 py-3 hover:bg-zinc-50 dark:hover:bg-zinc-800">
                  <span className="min-w-0 [overflow-wrap:anywhere]">
                    <span className="block font-medium">{w.name}</span>
                    <span className="text-xs text-zinc-500">{w.slug}</span>
                  </span>
                  <span className="shrink-0 rounded-full bg-zinc-100 px-2 py-0.5 text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400">{ta.roles[w.role]}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
        {/* Phones: the header has no room for the language picker (ADR-0019, ADR-0025);
            below xl it has none for the theme picker either (ADR-0035). */}
        <div className="mt-6 flex flex-wrap items-center justify-end gap-x-4 gap-y-2 text-sm text-zinc-500 xl:hidden">
          <label className="flex items-center gap-2">
            <span>{tth.theme}</span>
            <ThemeMenu labelled className="h-11 pointer-fine:h-8" />
          </label>
          <span className="flex items-center gap-2 sm:hidden">
            <span aria-hidden>{t.language}</span>
            <LanguageMenu className="h-11" />
          </span>
        </div>
      </div>
      <Dialog open={open} onClose={() => { setOpen(false); }} title={t.newWorkspace}>
        <CreateWorkspaceForm onDone={() => { setOpen(false); }} />
      </Dialog>
    </Layout>
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

  const submit = (e: SyntheticEvent) => {
    e.preventDefault();
    create.mutate(
      { name, slug, template },
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
        <p className="mt-1 text-xs text-zinc-500">{t.slugHint}</p>
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
        <p className="mt-1 text-xs text-zinc-500">{tt.templateHint}</p>
      </div>
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
