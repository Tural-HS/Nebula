// "/" — the home page "Mənim işlərim" (ADR-0037): the signed-in user's work
// across every workspace in one request (GET /me/home). The server decides
// what the viewer may see (ADR-0013) and may decide (ADR-0034); this page
// only lays it out. ?show=<kind> turns the summary into one full list.
import { useQuery } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router";

import { api } from "../api/client";
import { formKeys, type MyRequestPage } from "../api/forms";
import { HOME_LIMIT, HOME_MAX, isHomeShow, useHome, type Home, type HomeItem, type HomeShow, type HomeWorkspace } from "../api/home";
import { useMe } from "../api/hooks";
import { useMarkRead, useNotifications } from "../api/notifications";
import { CreateWorkspaceDialog, NewWorkspaceButton } from "../components/CreateWorkspace";
import { ArchivedWorkspaces } from "../components/Lifecycle";
import { LanguageMenu } from "../components/LanguageMenu";
import { StickyNotes } from "../components/StickyNotes";
import { Layout } from "../components/Layout";
import { ThemeMenu } from "../components/ThemeMenu";
import { Button, ErrorState, Spinner, StatusBadge } from "../components/ui";
import { t } from "../i18n";
import { ta } from "../i18n-admin";
import { tai } from "../i18n-approval-inbox";
import { thm } from "../i18n-home";
import { ts } from "../i18n-search";
import { tth } from "../i18n-theme";
import { ACCENT_SCALES, DEFAULT_ACCENT, isAccent } from "../lib/accent";
import { ageDays } from "../lib/approvalInbox";
import { cn } from "../lib/cn";
import { formatDate, formatDateTime, formatRelative } from "../lib/format";
import { COUNTERS, counterHref, firstName, formatToday, homeListHref, isCalm, itemHref, orderCards, rowDate } from "../lib/home";
import { notificationHref, notificationText } from "../lib/notifications";

const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);

export function HomePage() {
  const [params] = useSearchParams();
  const raw = params.get("show");
  const show: HomeShow | null = isHomeShow(raw) ? raw : null;
  const q = useHome(show ?? "attention", show ? HOME_MAX : HOME_LIMIT);
  const [creating, setCreating] = useState(false);

  let body: ReactNode;
  if (q.isPending) body = <Spinner />;
  else if (q.isError) body = <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  else if (q.data.workspaces.length === 0) body = (
    <>
      <NoWorkspaces onCreate={() => { setCreating(true); }} />
      <ArchivedWorkspaces />
    </>
  );
  else {
    const home = q.data;
    body = (
      <>
        <CounterStrip home={home} active={show} />
        {!show && <StickyNotes />}
        {show ? (
          <FullList show={show} home={home} loading={q.isPlaceholderData} />
        ) : (
          <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
            <Attention home={home} />
            <aside className="min-w-0 space-y-6">
              <NotificationsPanel />
              <RequestsPanel />
              <WatchingPanel home={home} />
            </aside>
          </div>
        )}
        <WorkspaceCards list={home.workspaces} />
        <ArchivedWorkspaces />
      </>
    );
  }

  return (
    <Layout
      crumbs={show ? [{ label: thm.title, to: "/" }, { label: thm.list[show] }] : [{ label: thm.title }]}
      actions={<NewWorkspaceButton onClick={() => { setCreating(true); }} />}
    >
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-6xl space-y-6 p-4 md:p-6">
          <Greeting />
          {body}
          {/* Phones: the header has no room for the language picker (ADR-0019, ADR-0025);
              below xl it has none for the theme picker either (ADR-0035). */}
          <div className="flex flex-wrap items-center justify-end gap-x-4 gap-y-2 text-sm text-zinc-500 dark:text-zinc-400 xl:hidden">
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
      </div>
      <CreateWorkspaceDialog open={creating} onClose={() => { setCreating(false); }} />
    </Layout>
  );
}

function Greeting() {
  const me = useMe();
  return (
    <div>
      <h1 className="text-xl font-semibold tracking-tight [overflow-wrap:anywhere]">{thm.greeting(firstName(me.data?.display_name))}</h1>
      <p className="text-sm text-zinc-500 dark:text-zinc-400">{formatToday()}</p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Counters: KPI cards (ADR-0035); the strip scrolls sideways inside itself,
// so a 375 px page never does.
// ---------------------------------------------------------------------------

const COUNTER_TONE: Record<string, string> = {
  overdue: "text-red-700 dark:text-red-400",
  approvals: "text-amber-700 dark:text-amber-400",
};

function CounterStrip({ home, active }: { home: Home; active: HomeShow | null }) {
  return (
    <nav aria-label={thm.counters} className="-mx-4 overflow-x-auto overscroll-x-contain px-4 [scrollbar-width:thin] md:mx-0 md:px-0">
      <ul className="flex gap-2 md:grid md:grid-cols-4 md:gap-3">
        {COUNTERS.map((c) => {
          const n = home.counters[c.key];
          const on = active === c.show;
          return (
            <li key={c.key} className="min-w-[9.5rem] shrink-0 md:min-w-0">
              <Link
                to={counterHref(c.show, home.workspaces)}
                aria-label={thm.counterLabel(thm.counter[c.key], n)}
                aria-current={on ? "page" : undefined}
                data-testid={`home-counter-${c.key}`}
                className={cn(
                  "flex h-full flex-col gap-0.5 rounded-xl bg-white px-4 py-3 ring-1 ring-zinc-200 hover:ring-zinc-300 dark:bg-zinc-900 dark:ring-zinc-800 dark:hover:ring-zinc-700",
                  on && "ring-2 ring-brand-600 hover:ring-brand-600 dark:ring-brand-400 dark:hover:ring-brand-400",
                )}
              >
                <span aria-hidden className={cn("text-2xl leading-8 font-semibold tracking-tight tabular-nums", n ? COUNTER_TONE[c.key] ?? "text-zinc-900 dark:text-zinc-50" : "text-zinc-500 dark:text-zinc-400")}>
                  {n}
                </span>
                <span aria-hidden className="text-xs whitespace-nowrap text-zinc-500 dark:text-zinc-400">{thm.counter[c.key]}</span>
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

// ---------------------------------------------------------------------------
// Item rows: the attention list and the full lists.
// ---------------------------------------------------------------------------

const REASON_STYLE: Record<HomeItem["reason"], string> = {
  overdue: "bg-red-50 text-red-700 ring-red-200 dark:bg-red-950 dark:text-red-300 dark:ring-red-900",
  approval: "bg-amber-50 text-amber-800 ring-amber-200 dark:bg-amber-950 dark:text-amber-300 dark:ring-amber-900",
  week: "bg-brand-50 text-brand-700 ring-brand-200 dark:bg-brand-950 dark:text-brand-200 dark:ring-brand-800",
  assigned: "bg-zinc-100 text-zinc-700 ring-zinc-200 dark:bg-zinc-800 dark:text-zinc-300 dark:ring-zinc-700",
};

const STRIPE: Partial<Record<HomeItem["reason"], string>> = { overdue: "bg-red-600 dark:bg-red-500", approval: "bg-amber-500" };

function AccentDot({ accent }: { accent: string | null }) {
  const a = isAccent(accent) ? accent : DEFAULT_ACCENT;
  // Set through the CSSOM by React, which the CSP allows (unlike style attributes in markup).
  return <span aria-hidden className="inline-block size-2 shrink-0 rounded-full" style={{ background: ACCENT_SCALES[a][600] }} />;
}

function Where({ project, workspace }: { project: string; workspace: { name: string; accent: string | null } }) {
  return (
    <span className="flex min-w-0 items-center gap-1.5">
      <AccentDot accent={workspace.accent} />
      <span className="truncate">{project} · {workspace.name}</span>
    </span>
  );
}

function RowDateLabel({ it }: { it: HomeItem }) {
  const d = rowDate(it);
  if (!d) return null;
  if (d.kind === "approval") {
    return (
      <span className="whitespace-nowrap tabular-nums" title={formatDateTime(d.since)}>
        {thm.waiting(tai.age(ageDays(d.since)))}
        {d.dueOn && <span className={cn("ml-1.5", d.overdue && "font-medium text-red-700 dark:text-red-400")}>· {d.overdue ? tai.overdue(formatDate(d.dueOn)) : tai.due(formatDate(d.dueOn))}</span>}
      </span>
    );
  }
  return <span className={cn("whitespace-nowrap tabular-nums", d.overdue && "font-medium text-red-700 dark:text-red-400")}>{thm.due(formatDate(d.date))}</span>;
}

function ItemRow({ it }: { it: HomeItem }) {
  const stripe = STRIPE[it.reason];
  return (
    <li className="relative">
      {stripe && <span aria-hidden className={cn("absolute inset-y-0 left-0 w-1", stripe)} />}
      <Link
        to={itemHref(it)}
        data-testid="home-row"
        data-reason={it.reason}
        className="flex min-w-0 flex-col gap-1 py-2.5 pr-3 pl-4 hover:bg-zinc-50 sm:flex-row sm:items-center sm:gap-3 dark:hover:bg-zinc-800/50"
      >
        <span className="flex min-w-0 flex-1 items-center gap-2">
          <span className="shrink-0 font-mono text-xs text-zinc-500 dark:text-zinc-400">{it.key}</span>
          <span className="min-w-0 flex-1 truncate text-sm font-medium">{it.title}</span>
        </span>
        <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs text-zinc-500 sm:flex-nowrap sm:justify-end dark:text-zinc-400">
          <span className={cn("shrink-0 rounded px-1.5 py-0.5 font-medium ring-1 ring-inset", REASON_STYLE[it.reason])}>{thm.reason[it.reason]}</span>
          <span className="min-w-0 sm:max-w-48"><Where project={it.project.name} workspace={it.workspace} /></span>
          <RowDateLabel it={it} />
        </span>
      </Link>
    </li>
  );
}

const listBox = "divide-y divide-zinc-100 overflow-hidden rounded-xl bg-white ring-1 ring-zinc-200 dark:divide-zinc-800 dark:bg-zinc-900 dark:ring-zinc-800";

function SectionTitle({ id, children, extra }: { id: string; children: ReactNode; extra?: ReactNode }) {
  return (
    <div className="mb-2 flex items-baseline justify-between gap-3">
      <h2 id={id} className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">{children}</h2>
      {extra}
    </div>
  );
}

function Attention({ home }: { home: Home }) {
  if (home.items.length === 0 && isCalm(home.counters)) return <Calm home={home} />;
  return (
    <section aria-labelledby="home-attention" className="min-w-0">
      <SectionTitle
        id="home-attention"
        extra={home.total > home.items.length && (
          <Link to={homeListHref("attention")} className="text-sm font-medium text-brand-700 hover:underline dark:text-brand-300">
            {thm.showAll(home.total)}
          </Link>
        )}
      >
        {thm.attention}
      </SectionTitle>
      {home.items.length === 0 ? (
        <p className={cn(listBox, "p-4 text-sm text-zinc-500 dark:text-zinc-400")}>{thm.listEmpty}</p>
      ) : (
        <ul data-testid="home-attention" className={listBox}>
          {home.items.map((it) => <ItemRow key={`${it.workspace.slug}/${it.key}`} it={it} />)}
        </ul>
      )}
    </section>
  );
}

function FullList({ show, home, loading }: { show: HomeShow; home: Home; loading: boolean }) {
  return (
    <section aria-labelledby="home-list" className="min-w-0">
      <Link to="/" className="mb-2 inline-flex items-center gap-1 text-sm text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 pointer-coarse:min-h-11 dark:hover:text-zinc-100">
        <span aria-hidden>←</span> {thm.back}
      </Link>
      <h2 id="home-list" className="text-lg font-semibold">{thm.list[show]}</h2>
      <p className="mb-3 text-sm text-zinc-500 dark:text-zinc-400">{thm.listHint}</p>
      {loading ? (
        <Spinner />
      ) : home.items.length === 0 ? (
        <p className={cn(listBox, "p-4 text-sm text-zinc-500 dark:text-zinc-400")}>{thm.listEmpty}</p>
      ) : (
        <>
          <ul data-testid="home-list" className={listBox}>
            {home.items.map((it) => <ItemRow key={`${it.workspace.slug}/${it.key}`} it={it} />)}
          </ul>
          {home.total > home.items.length && <p className="mt-2 text-xs text-zinc-500 dark:text-zinc-400">{thm.truncated(home.items.length, home.total)}</p>}
        </>
      )}
    </section>
  );
}

// Workspaces, but nothing asks for action: say so, and offer a way on.
function Calm({ home }: { home: Home }) {
  return (
    <section aria-labelledby="home-calm" data-testid="home-calm" className="min-w-0 rounded-xl bg-white p-5 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
      <h2 id="home-calm" className="font-semibold">{thm.calm}</h2>
      <p className="mt-1 text-sm text-zinc-500 dark:text-zinc-400">{thm.calmHint}</p>
      {home.recent_projects.length > 0 && (
        <>
          <h3 className="mt-4 mb-1 text-xs font-medium tracking-wide text-zinc-500 dark:text-zinc-400 uppercase">{thm.recentProjects}</h3>
          <ul className="flex flex-wrap gap-2">
            {home.recent_projects.map((p) => (
              <li key={`${p.workspace.slug}/${p.project.key}`}>
                <Link
                  to={`/w/${encodeURIComponent(p.workspace.slug)}/p/${encodeURIComponent(p.project.key)}`}
                  className="inline-flex max-w-full items-center gap-1.5 rounded-md bg-zinc-100 px-2.5 py-1.5 text-sm hover:bg-zinc-200 pointer-coarse:min-h-11 dark:bg-zinc-800 dark:hover:bg-zinc-700"
                >
                  <Where project={p.project.name} workspace={p.workspace} />
                </Link>
              </li>
            ))}
          </ul>
        </>
      )}
      <p className="mt-4 text-sm text-zinc-500 dark:text-zinc-400">
        {thm.searchHint(ts.shortcut(isMac))}
      </p>
    </section>
  );
}

function NoWorkspaces({ onCreate }: { onCreate: () => void }) {
  return (
    <section aria-labelledby="home-none" data-testid="home-no-workspaces" className="mx-auto max-w-xl rounded-xl bg-white p-6 text-center ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
      <h2 id="home-none" className="text-base font-semibold">{thm.noWorkspaces}</h2>
      <p className="mt-2 text-sm text-zinc-500 dark:text-zinc-400">{thm.invitedHint}</p>
      <p className="mt-4 text-sm text-zinc-500 dark:text-zinc-400">{thm.createHint}</p>
      <Button variant="primary" className="mt-3 pointer-coarse:min-h-11" onClick={onCreate}>{thm.createCta}</Button>
    </section>
  );
}

// ---------------------------------------------------------------------------
// Side panels
// ---------------------------------------------------------------------------

const PANEL_ROWS = 5;
const panelBox = "divide-y divide-zinc-100 overflow-hidden rounded-xl bg-white text-sm ring-1 ring-zinc-200 dark:divide-zinc-800 dark:bg-zinc-900 dark:ring-zinc-800";
const panelLink = "block px-3 py-2 hover:bg-zinc-50 pointer-coarse:min-h-11 dark:hover:bg-zinc-800/50";

function NotificationsPanel() {
  // Same query (and cache) as the header bell.
  const q = useNotifications(true);
  const markRead = useMarkRead();
  const list = (q.data?.notifications ?? []).slice(0, PANEL_ROWS);
  return (
    <section aria-labelledby="home-notifications">
      <SectionTitle id="home-notifications">{thm.notifications}</SectionTitle>
      {q.isPending ? (
        <Spinner />
      ) : list.length === 0 ? (
        <p className={cn(panelBox, "p-3 text-zinc-500 dark:text-zinc-400")}>{thm.noNotifications}</p>
      ) : (
        <ul className={panelBox}>
          {list.map((n) => (
            <li key={n.id}>
              <Link to={notificationHref(n)} className={panelLink} onClick={() => { if (!n.read) markRead.mutate(n.id); }}>
                <span className="flex items-start gap-2">
                  <span aria-hidden className={cn("mt-1.5 size-1.5 shrink-0 rounded-full", n.read ? "bg-transparent" : "bg-brand-600")} />
                  <span className="min-w-0">
                    <span className={cn("line-clamp-2 [overflow-wrap:anywhere]", !n.read && "font-medium")}>{notificationText(n)}</span>
                    <span className="block text-xs text-zinc-500 dark:text-zinc-400" title={formatDateTime(n.created_at)}>{n.workspace_name} · {formatRelative(n.created_at)}</span>
                  </span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

// Shown only to people who sent intake-form requests (ADR-0032).
function RequestsPanel() {
  const q = useQuery({
    queryKey: [...formKeys.myRequests, "home"],
    queryFn: () => api<MyRequestPage>("GET", `/me/requests?limit=${String(PANEL_ROWS)}`),
    staleTime: 60_000,
  });
  const list = q.data?.requests ?? [];
  if (list.length === 0) return null;
  return (
    <section aria-labelledby="home-requests">
      <SectionTitle
        id="home-requests"
        extra={<Link to="/requests" className="text-xs font-medium text-brand-700 hover:underline dark:text-brand-300">{thm.allRequests}</Link>}
      >
        {thm.requests}
      </SectionTitle>
      <ul className={panelBox}>
        {list.map((r) => (
          <li key={r.id}>
            <Link to={`/requests/${encodeURIComponent(r.id)}`} className={panelLink}>
              <span className="flex min-w-0 items-center gap-2">
                <span className="shrink-0 font-mono text-xs text-zinc-500 dark:text-zinc-400">{r.key}</span>
                <span className={cn("min-w-0 flex-1 truncate", r.unseen && "font-medium")}>{r.title}</span>
                {r.status && <StatusBadge name={r.status.name} category={r.status.category} />}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

function WatchingPanel({ home }: { home: Home }) {
  if (home.watching.length === 0) return null;
  return (
    <section aria-labelledby="home-watching">
      <SectionTitle id="home-watching">{thm.watching}</SectionTitle>
      <ul className={panelBox}>
        {home.watching.map((w) => (
          <li key={`${w.workspace.slug}/${w.key}`}>
            <Link
              to={`/w/${encodeURIComponent(w.workspace.slug)}/p/${encodeURIComponent(w.project.key)}?item=${String(w.number)}`}
              className={panelLink}
            >
              <span className="flex min-w-0 items-center gap-2">
                <span className="shrink-0 font-mono text-xs text-zinc-500 dark:text-zinc-400">{w.key}</span>
                <span className="min-w-0 flex-1 truncate">{w.title}</span>
                <StatusBadge name={w.status.name} category={w.status.category} />
              </span>
              <span className="mt-0.5 flex items-center gap-2 text-xs text-zinc-500 dark:text-zinc-400">
                <Where project={w.project.name} workspace={w.workspace} />
                <span className="shrink-0" title={formatDateTime(w.updated_at)}>· {formatRelative(w.updated_at)}</span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

// ---------------------------------------------------------------------------
// Workspace cards
// ---------------------------------------------------------------------------

function WorkspaceCards({ list }: { list: HomeWorkspace[] }) {
  return (
    <section aria-labelledby="home-workspaces">
      <SectionTitle id="home-workspaces">{t.workspaces}</SectionTitle>
      <ul data-testid="workspace-cards" className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {orderCards(list).map((w) => <WorkspaceCard key={w.id} w={w} />)}
      </ul>
    </section>
  );
}

function WorkspaceCard({ w }: { w: HomeWorkspace }) {
  const accent = isAccent(w.accent) ? w.accent : DEFAULT_ACCENT;
  return (
    <li className="relative min-w-0 overflow-hidden rounded-xl bg-white ring-1 ring-zinc-200 hover:ring-zinc-300 dark:bg-zinc-900 dark:ring-zinc-800 dark:hover:ring-zinc-700">
      <span aria-hidden className="absolute inset-y-0 left-0 w-1" style={{ background: ACCENT_SCALES[accent][600] }} />
      <Link to={`/w/${encodeURIComponent(w.slug)}`} className="flex h-full flex-col gap-2 py-3 pr-4 pl-5">
        <span className="flex min-w-0 items-start justify-between gap-2">
          <span className="min-w-0 [overflow-wrap:anywhere]">
            <span className="block font-medium">{w.name}</span>
            <span className="block text-xs text-zinc-500 dark:text-zinc-400">{w.slug} · {thm.card.projects(w.projects)}</span>
          </span>
          <span className="shrink-0 rounded-full bg-zinc-100 px-2 py-0.5 text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400">{ta.roles[w.role]}</span>
        </span>
        <span className="flex flex-wrap gap-x-3 gap-y-1 text-xs">
          <Stat n={w.assigned} label={thm.card.assigned} />
          <Stat n={w.overdue} label={thm.card.overdue} tone="text-red-700 dark:text-red-400" />
          <Stat n={w.approvals} label={thm.card.approvals} tone="text-amber-700 dark:text-amber-400" />
        </span>
        <span className="mt-auto text-xs text-zinc-500 dark:text-zinc-400" title={w.last_activity ? formatDateTime(w.last_activity) : undefined}>
          {w.last_activity ? thm.card.lastActivity(formatRelative(w.last_activity)) : thm.card.noActivity}
        </span>
      </Link>
    </li>
  );
}

function Stat({ n, label, tone }: { n: number; label: string; tone?: string }) {
  return (
    <span aria-hidden className="whitespace-nowrap text-zinc-500 dark:text-zinc-400">
      <span className={cn("font-semibold tabular-nums", n ? tone ?? "text-zinc-900 dark:text-zinc-100" : "text-zinc-500 dark:text-zinc-400")}>{n}</span> {label}
    </span>
  );
}
