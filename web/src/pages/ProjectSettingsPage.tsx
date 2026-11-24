import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useParams } from "react-router";

import { api, path } from "../api/client";
import { keys, useMembers, useProjectMembers, useProjects, useWorkspace } from "../api/hooks";
import { roleAtLeast, type Project } from "../api/types";
import { Layout } from "../components/Layout";
import { useToast } from "../components/toast-context";
import { Avatar, Button, EmptyState, ErrorState, FieldError, Input, Label, Select, Spinner } from "../components/ui";
import { t } from "../i18n";
import { ta } from "../i18n-admin";
import { tf } from "../i18n-forms";
import { tp } from "../i18n-project";

export function ProjectSettingsPage() {
  const { ws = "", project = "" } = useParams();
  const workspace = useWorkspace(ws);
  const projects = useProjects(ws);
  const p = projects.data?.find((x) => x.key === project);
  const projectUrl = `/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(project)}`;

  let body;
  if (workspace.isPending || projects.isPending) body = <Spinner />;
  else if (workspace.isError) body = <ErrorState error={workspace.error} />;
  else if (!p) body = <EmptyState>{t.notFound}</EmptyState>;
  else if (!roleAtLeast(workspace.data.role, "admin")) body = <EmptyState>{tp.adminOnly}</EmptyState>;
  else body = <Settings key={p.id} ws={ws} project={p} />;

  return (
    <Layout
      crumbs={[
        { label: t.workspaces, to: "/" },
        { label: workspace.data?.name ?? ws, to: `/w/${encodeURIComponent(ws)}` },
        { label: p?.name ?? project, to: projectUrl },
        { label: tp.settings },
      ]}
    >
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-3xl space-y-6 p-4 md:p-6">
          <h1 className="text-lg font-semibold">{tp.settings}</h1>
          {body}
        </div>
      </div>
    </Layout>
  );
}

function Settings({ ws, project }: { ws: string; project: Project }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState(project.name);
  const [visibility, setVisibility] = useState(project.visibility);
  const update = useMutation({
    mutationFn: (body: { name: string; visibility: Project["visibility"] }) =>
      api<Project>("PATCH", path`/workspaces/${ws}/projects/${project.key}`, body),
    onSuccess: () => {
      toast("success", ta.saved);
      void qc.invalidateQueries({ queryKey: keys.projects(ws) });
    },
  });

  return (
    <>
      <form
        className="space-y-4 rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800"
        onSubmit={(e) => {
          e.preventDefault();
          if (visibility === "private" && project.visibility !== "private" && !window.confirm(tp.makePrivateConfirm)) return;
          update.mutate({ name, visibility });
        }}
      >
        <div>
          <Label htmlFor="ps-name">{t.name}</Label>
          <Input id="ps-name" required maxLength={200} value={name} onChange={(e) => { setName(e.target.value); }} />
        </div>
        <div>
          <Label htmlFor="ps-visibility">{tp.visibility}</Label>
          <Select id="ps-visibility" value={visibility} onChange={(e) => { setVisibility(e.target.value === "private" ? "private" : "workspace"); }}>
            <option value="workspace">{tp.visibilityOptions.workspace}</option>
            <option value="private">🔒 {tp.visibilityOptions.private}</option>
          </Select>
        </div>
        {visibility === "private" && (
          <p role="note" className="rounded-md bg-amber-50 p-3 text-xs text-amber-900 ring-1 ring-amber-200 dark:bg-amber-950 dark:text-amber-200 dark:ring-amber-900">
            {tp.webhookWarning}
          </p>
        )}
        <FieldError error={update.error} />
        <Button type="submit" variant="primary" disabled={update.isPending}>{t.save}</Button>
      </form>
      {project.visibility === "private" && <Members ws={ws} project={project} />}
      <section aria-labelledby="ps-forms" className="space-y-2 rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
        <h2 id="ps-forms" className="text-sm font-semibold">{tf.section}</h2>
        <p className="text-xs text-zinc-500">{tf.sectionHint}</p>
        <Link to={`/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(project.key)}/forms`} className="inline-flex rounded-md px-3 py-1.5 text-sm font-medium ring-1 ring-zinc-300 hover:bg-zinc-50 pointer-coarse:min-h-11 dark:ring-zinc-700 dark:hover:bg-zinc-800">
          {tf.manage}
        </Link>
      </section>
    </>
  );
}

function Members({ ws, project }: { ws: string; project: Project }) {
  const qc = useQueryClient();
  const toast = useToast();
  const all = useMembers(ws);
  const members = useProjectMembers(ws, project.key);
  const [pick, setPick] = useState("");
  const refresh = () => qc.invalidateQueries({ queryKey: ["ws", ws, "p", project.key, "members"] });
  const add = useMutation({
    mutationFn: (userID: string) => api<undefined>("PUT", path`/workspaces/${ws}/projects/${project.key}/members/${userID}`),
    onSuccess: () => { setPick(""); void refresh(); },
    onError: () => { toast("error", t.errorGeneric); },
  });
  const remove = useMutation({
    mutationFn: (userID: string) => api<undefined>("DELETE", path`/workspaces/${ws}/projects/${project.key}/members/${userID}`),
    onSuccess: () => void refresh(),
    onError: () => { toast("error", t.errorGeneric); },
  });

  if (members.isPending || all.isPending) return <Spinner />;
  if (members.isError) return <ErrorState error={members.error} />;
  const inProject = new Set(members.data.map((m) => m.user_id));
  // Admins see private projects anyway; offer the others.
  const candidates = (all.data ?? []).filter((m) => !inProject.has(m.user_id) && !roleAtLeast(m.role, "admin"));

  return (
    <section aria-label={tp.members} className="space-y-3 rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
      <h2 className="text-sm font-semibold">{tp.members}</h2>
      <p className="text-xs text-zinc-500">{tp.membersHint}</p>
      {members.data.length === 0 ? (
        <p className="text-sm text-zinc-500">{tp.noMembers}</p>
      ) : (
        <ul className="divide-y divide-zinc-100 dark:divide-zinc-800">
          {members.data.map((m) => (
            <li key={m.user_id} className="flex items-center gap-3 py-2 text-sm">
              <Avatar name={m.display_name} />
              <span className="min-w-0 flex-1">
                {m.display_name} <span className="text-xs text-zinc-500">{m.email} · {ta.roles[m.role]}</span>
              </span>
              <Button variant="ghost" className="text-red-600" onClick={() => { remove.mutate(m.user_id); }} aria-label={`${tp.remove}: ${m.display_name}`}>
                {tp.remove}
              </Button>
            </li>
          ))}
        </ul>
      )}
      {candidates.length > 0 && (
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (pick) add.mutate(pick);
          }}
        >
          <Select aria-label={tp.addMember} value={pick} onChange={(e) => { setPick(e.target.value); }} className="w-64">
            <option value="">—</option>
            {candidates.map((m) => <option key={m.user_id} value={m.user_id}>{m.display_name} ({ta.roles[m.role]})</option>)}
          </Select>
          <Button type="submit" disabled={!pick || add.isPending}>{tp.addMember}</Button>
        </form>
      )}
    </section>
  );
}
