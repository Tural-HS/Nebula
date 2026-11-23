import { useState, type SyntheticEvent } from "react";
import { Link, useNavigate } from "react-router";

import { useCreateWorkspace, useWorkspaces } from "../api/hooks";
import { Layout } from "../components/Layout";
import { Button, Dialog, EmptyState, ErrorState, FieldError, Input, Label, Spinner } from "../components/ui";
import { t } from "../i18n";

export function WorkspacesPage() {
  const q = useWorkspaces();
  const [open, setOpen] = useState(false);
  return (
    <Layout
      crumbs={[{ label: t.workspaces }]}
      actions={
        <Button variant="primary" onClick={() => { setOpen(true); }}>
          {t.newWorkspace}
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
                  <span className="shrink-0 rounded-full bg-zinc-100 px-2 py-0.5 text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400">{w.role}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
      <Dialog open={open} onClose={() => { setOpen(false); }} title={t.newWorkspace}>
        <CreateWorkspaceForm onDone={() => { setOpen(false); }} />
      </Dialog>
    </Layout>
  );
}

function CreateWorkspaceForm({ onDone }: { onDone: () => void }) {
  const create = useCreateWorkspace();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");

  const submit = (e: SyntheticEvent) => {
    e.preventDefault();
    create.mutate(
      { name, slug },
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
