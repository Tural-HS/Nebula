import type { ReactNode } from "react";
import { useParams } from "react-router";

import { useProjects, useWorkspace } from "../api/hooks";
import { useProjectState } from "../api/lifecycle";
import { ArchivedProjectState } from "../components/Lifecycle";
import { Layout } from "../components/Layout";
import { Spinner } from "../components/ui";
import { t } from "../i18n";
import { projectGone } from "../lib/lifecycle";

/**
 * Project pages render as usual while the project is active. When it is not
 * among the workspace's active projects, the API says why (ADR-0039): an
 * archived project shows its state with Restore (admins) and Delete (owner),
 * anything else is "not found".
 */
export function ProjectGate({ children }: { children: ReactNode }) {
  const { ws = "", project = "" } = useParams();
  const workspace = useWorkspace(ws);
  const projects = useProjects(ws);
  const missing = projects.isSuccess && !projects.data.some((p) => p.key === project.toUpperCase());
  const state = useProjectState(ws, project, missing);
  // Active after all (the list was stale, e.g. just restored): the page itself.
  if (!missing || state.isSuccess) return <>{children}</>;
  return (
    <Layout
      crumbs={[
        { label: t.workspaces, to: "/" },
        { label: workspace.data?.name ?? ws, to: `/w/${encodeURIComponent(ws)}` },
        { label: project },
      ]}
    >
      <div className="h-full overflow-y-auto">
        {state.isPending ? (
          <Spinner />
        ) : (
          <ArchivedProjectState ws={ws} projectKey={project.toUpperCase()} gone={projectGone(state.error)} role={workspace.data?.role} />
        )}
      </div>
    </Layout>
  );
}
