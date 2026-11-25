import { useState, type SyntheticEvent } from "react";
import { Link, useNavigate, useParams } from "react-router";

import { useCreateProject, useProjects, useWorkspace } from "../api/hooks";
import { roleAtLeast } from "../api/types";
import { ApprovalInboxLink } from "../components/Approvals";
import { Layout } from "../components/Layout";
import { ArchivedProjectsList } from "../components/Lifecycle";
import { Button, Dialog, EmptyState, ErrorState, FieldError, Input, Label, Select, Spinner } from "../components/ui";
import { t } from "../i18n";
import { tp } from "../i18n-project";
import { trp } from "../i18n-reports";
import { ta } from "../i18n-admin";
import { tt } from "../i18n-templates";

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
          {/* An icon on phones: with the round-6 header items the text no
              longer fits 375 px (ADR-0019). */}
          <Link
            to={`/w/${encodeURIComponent(ws)}/settings/members`}
            aria-label={ta.settings}
            className="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm whitespace-nowrap text-zinc-700 hover:bg-zinc-100 max-sm:min-h-11 max-sm:min-w-11 max-sm:px-0 dark:text-zinc-300 dark:hover:bg-zinc-800"
          >
            <span className="max-sm:hidden">{ta.settings}</span>
            <span aria-hidden className="text-lg leading-none sm:hidden">⚙</span>
          </Link>
          {canCreate && (
            <Button variant="primary" aria-label={t.newProject} className="pointer-coarse:min-w-11" onClick={() => { setOpen(true); }}>
              <span className="max-sm:hidden">{t.newProject}</span>
              <span aria-hidden className="text-lg leading-none sm:hidden">+</span>
            </Button>
          )}
        </>
      }
    >
      <div className="mx-auto max-w-3xl p-4 md:p-6">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
          <h1 className="text-lg font-semibold">{t.projects}</h1>
          <div className="flex flex-wrap gap-2">
            {/* Guests see it only with at least one visible project: the
                report counts nothing else (ADR-0041). */}
            {(projects.data?.length ?? 0) > 0 && (
              <Link
                to={`/w/${encodeURIComponent(ws)}/reports`}
                className="inline-flex min-w-0 items-center gap-2 rounded-lg bg-white px-3 py-2 text-sm ring-1 ring-zinc-200 hover:ring-brand-500 pointer-coarse:min-h-11 dark:bg-zinc-900 dark:ring-zinc-800"
              >
                <span aria-hidden>📊</span>
                <span className="truncate font-medium">{trp.nav}</span>
              </Link>
            )}
            <ApprovalInboxLink ws={ws} />
          </div>
        </div>
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
                      <span title={tp.privateHint} className="rounded bg-zinc-100 px-1.5 py-0.5 text-2xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300">
                        🔒 {tp.private}
                      </span>
                    )}
                  </span>
                  <span className="mt-1 block font-medium [overflow-wrap:anywhere]">{p.name}</span>
                  {p.description && <span className="mt-1 line-clamp-2 block text-sm [overflow-wrap:anywhere] text-zinc-500 dark:text-zinc-400">{p.description}</span>}
                </Link>
              </li>
            ))}
          </ul>
        )}
        <ArchivedProjectsList ws={ws} enabled={canCreate} />
      </div>
      <Dialog open={open} onClose={() => { setOpen(false); }} title={t.newProject}>
        <CreateProjectForm ws={ws} onDone={() => { setOpen(false); }} />
      </Dialog>
    </Layout>
  );
}

function CreateProjectForm({ ws, onDone }: { ws: string; onDone: () => void }) {
  const create = useCreateProject(ws);
  const projects = useProjects(ws);
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [isPrivate, setPrivate] = useState(false);
  const [from, setFrom] = useState("");
  const [copy, setCopy] = useState({ members: true, recurrences: true, views: true });

  const submit = (e: SyntheticEvent) => {
    e.preventDefault();
    create.mutate(
      { name, key, visibility: isPrivate ? "private" : "workspace", ...(from ? { template: { from, ...copy } } : {}) },
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
        <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{t.projectKeyHint}</p>
      </div>
      <label className="flex items-start gap-2 text-sm">
        <input type="checkbox" className="mt-0.5 accent-brand-600" checked={isPrivate} onChange={(e) => { setPrivate(e.target.checked); }} />
        <span>
          🔒 {tp.privateProject}
          <span className="block text-xs text-zinc-500 dark:text-zinc-400">{tp.privateHint}</span>
        </span>
      </label>
      {(projects.data?.length ?? 0) > 0 && (
        <div>
          <Label htmlFor="p-template">{tt.projectTemplate}</Label>
          <Select id="p-template" value={from} onChange={(e) => {
            const src = projects.data?.find((p) => p.key === e.target.value);
            setFrom(e.target.value);
            // A department template is usually private like its source.
            if (src) setPrivate(src.visibility === "private");
          }}>
            <option value="">{tt.projectTemplateNone}</option>
            {projects.data?.map((p) => <option key={p.key} value={p.key}>{`${p.key} · ${p.name}`}</option>)}
          </Select>
          {from && (
            <fieldset className="mt-2 space-y-1">
              <legend className="text-xs text-zinc-500 dark:text-zinc-400">{tt.projectTemplateHint}</legend>
              {(["members", "recurrences", "views"] as const).map((k) => (
                <label key={k} className="flex items-center gap-2 text-sm pointer-coarse:min-h-11">
                  <input type="checkbox" className="accent-brand-600" checked={copy[k]} onChange={(e) => { setCopy({ ...copy, [k]: e.target.checked }); }} />
                  {k === "members" ? tt.copyMembers : k === "recurrences" ? tt.copyRecurrences : tt.copyViews}
                </label>
              ))}
            </fieldset>
          )}
        </div>
      )}
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
