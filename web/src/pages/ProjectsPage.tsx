import { useState, type SyntheticEvent } from "react";
import { Link, useNavigate, useParams } from "react-router";

import { useCreateProject, useProjects, useWorkspace } from "../api/hooks";
import { roleAtLeast } from "../api/types";
import { Layout } from "../components/Layout";
import { Button, Dialog, EmptyState, ErrorState, FieldError, Input, Label, Spinner } from "../components/ui";
import { t } from "../i18n";
import { tp } from "../i18n-project";
import { ta } from "../i18n-admin";

export function ProjectsPage() {
  const { ws = "" } = useParams();
  const workspace = useWorkspace(ws);
  const projects = useProjects(ws);
  const [open, setOpen] = useState(false);
  const canCreate = roleAtLeast(workspace.data?.role, "admin");

  if (workspace.isError) {
    return (
      <Layout crumbs={[{ label: t.workspaces, to: "/" }]}>
        <ErrorState error={workspace.error} />
      </Layout>
    );
  }

  return (
    <Layout
      crumbs={[{ label: t.workspaces, to: "/" }, { label: workspace.data?.name ?? ws }]}
      actions={
        <>
          <Link to={`/w/${encodeURIComponent(ws)}/settings/members`} className="rounded-md px-3 py-1.5 text-sm whitespace-nowrap text-zinc-700 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800">
            {ta.settings}
          </Link>
          {canCreate && (
            <Button variant="primary" onClick={() => { setOpen(true); }}>
              {t.newProject}
            </Button>
          )}
        </>
      }
    >
      <div className="mx-auto max-w-3xl p-4 md:p-6">
        <h1 className="mb-4 text-lg font-semibold">{t.projects}</h1>
        {projects.isPending ? (
          <Spinner />
        ) : projects.isError ? (
          <ErrorState error={projects.error} onRetry={() => void projects.refetch()} />
        ) : projects.data.length === 0 ? (
          <EmptyState>{t.noProjects}</EmptyState>
        ) : (
          <ul className="grid gap-3 sm:grid-cols-2">
            {projects.data.map((p) => (
              <li key={p.id}>
                <Link
                  to={`/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(p.key)}`}
                  className="block rounded-xl bg-white p-4 ring-1 ring-zinc-200 transition hover:ring-brand-500 dark:bg-zinc-900 dark:ring-zinc-800"
                >
                  <span className="flex items-center justify-between gap-2">
                    <span className="font-mono text-xs text-brand-600 dark:text-brand-100">{p.key}</span>
                    {p.visibility === "private" && (
                      <span title={tp.privateHint} className="rounded bg-zinc-100 px-1.5 py-0.5 text-[11px] text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300">
                        🔒 {tp.private}
                      </span>
                    )}
                  </span>
                  <span className="mt-1 block font-medium [overflow-wrap:anywhere]">{p.name}</span>
                  {p.description && <span className="mt-1 line-clamp-2 block text-sm [overflow-wrap:anywhere] text-zinc-500">{p.description}</span>}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
      <Dialog open={open} onClose={() => { setOpen(false); }} title={t.newProject}>
        <CreateProjectForm ws={ws} onDone={() => { setOpen(false); }} />
      </Dialog>
    </Layout>
  );
}

function CreateProjectForm({ ws, onDone }: { ws: string; onDone: () => void }) {
  const create = useCreateProject(ws);
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [isPrivate, setPrivate] = useState(false);

  const submit = (e: SyntheticEvent) => {
    e.preventDefault();
    create.mutate(
      { name, key, visibility: isPrivate ? "private" : "workspace" },
      {
        onSuccess: (p) => {
          onDone();
          void navigate(`/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(p.key)}`);
        },
      },
    );
  };

  return (
    <form onSubmit={submit} className="space-y-4">
      <div>
        <Label htmlFor="p-name">{t.name}</Label>
        <Input id="p-name" value={name} onChange={(e) => { setName(e.target.value); }} required maxLength={200} autoFocus />
      </div>
      <div>
        <Label htmlFor="p-key">{t.projectKey}</Label>
        <Input id="p-key" value={key} onChange={(e) => { setKey(e.target.value.toUpperCase()); }} required pattern="[A-Z][A-Z0-9]{1,9}" />
        <p className="mt-1 text-xs text-zinc-500">{t.projectKeyHint}</p>
      </div>
      <label className="flex items-start gap-2 text-sm">
        <input type="checkbox" className="mt-0.5 accent-brand-600" checked={isPrivate} onChange={(e) => { setPrivate(e.target.checked); }} />
        <span>
          🔒 {tp.privateProject}
          <span className="block text-xs text-zinc-500">{tp.privateHint}</span>
        </span>
      </label>
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
