import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import { useProjects } from "../api/hooks";
import { tp } from "../i18n-project";
import { cn } from "../lib/cn";

/**
 * Jump to another project of the workspace from the breadcrumb (Yığım ↔ MAH ↔
 * LAY): ledgers are used side by side all day. A disclosure (details/summary),
 * closed by Esc and outside clicks, focus back on the toggle.
 */
export function ProjectSwitcher({ ws, current, label, suffix = "" }: {
  ws: string;
  current: string;
  /** The current project's name: the toggle itself, so the crumb is one control. */
  label: string;
  /** Sub-page kept when switching, e.g. "/recurring". */
  suffix?: string;
}) {
  const ref = useRef<HTMLDetailsElement>(null);
  const [open, setOpen] = useState(false);
  const projects = useProjects(ws);
  useEffect(() => {
    const close = (e: Event) => {
      const d = ref.current;
      if (!d?.open || !(e instanceof KeyboardEvent ? e.key === "Escape" : !d.contains(e.target as Node))) return;
      const inside = d.contains(document.activeElement);
      d.open = false;
      if (inside && e instanceof KeyboardEvent) d.querySelector("summary")?.focus();
    };
    document.addEventListener("pointerdown", close);
    document.addEventListener("keydown", close);
    return () => {
      document.removeEventListener("pointerdown", close);
      document.removeEventListener("keydown", close);
    };
  }, []);
  const others = (projects.data ?? []).filter((p) => !p.archived_at);
  if (others.length < 2) {
    if (!label) return null;
    return <span className="truncate font-medium text-zinc-900 dark:text-zinc-100" aria-current="page">{label}</span>;
  }
  return (
    <details ref={ref} className="relative min-w-0" onToggle={(e) => { setOpen(e.currentTarget.open); }}>
      <summary
        title={tp.switchProject}
        aria-current={label ? "page" : undefined}
        aria-label={label ? undefined : tp.switchProject}
        className="flex min-w-0 cursor-pointer list-none items-center gap-1 rounded px-1 font-medium text-zinc-900 hover:bg-zinc-100 pointer-coarse:min-h-11 dark:text-zinc-100 dark:hover:bg-zinc-800 [&::-webkit-details-marker]:hidden"
      >
        {label && <span className="truncate">{label}</span>}
        <span aria-hidden className="shrink-0 text-xs text-zinc-500 dark:text-zinc-400">▾</span>
        {label && <span className="sr-only">, {tp.switchProject}</span>}
      </summary>
      {open && (
        <nav aria-label={tp.switchProject} className="absolute left-0 z-30 mt-1 max-h-[70vh] w-64 max-w-[calc(100vw-2rem)] overflow-y-auto rounded-lg bg-white p-1 text-sm shadow-lg ring-1 ring-zinc-300 dark:bg-zinc-900 dark:ring-zinc-700">
          <ul>
            {others.map((p) => {
              const here = p.key === current;
              return (
                <li key={p.key}>
                  <Link
                    to={`/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(p.key)}${suffix}`}
                    aria-current={here ? "page" : undefined}
                    onClick={() => { if (ref.current) ref.current.open = false; }}
                    className={cn(
                      "flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-zinc-100 pointer-coarse:min-h-11 dark:hover:bg-zinc-800",
                      here && "bg-brand-50 font-semibold text-brand-800 dark:bg-brand-950 dark:text-brand-200",
                    )}
                  >
                    <span className="w-10 shrink-0 text-2xs text-zinc-500 tabular-nums dark:text-zinc-400">{p.key}</span>
                    <span className="min-w-0 flex-1 truncate">{p.name}</span>
                    {p.visibility === "private" && <span role="img" aria-label={tp.privateMark} title={tp.privateMark}>🔒</span>}
                  </Link>
                </li>
              );
            })}
          </ul>
        </nav>
      )}
    </details>
  );
}
