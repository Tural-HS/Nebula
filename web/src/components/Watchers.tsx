// Watchers (ADR-0028): an eye toggle with the watcher count in the drawer
// header and a small panel listing who watches. Admins can add and remove
// other members; everyone can watch or unwatch themselves.
import { useEffect, useRef, useState } from "react";

import { ApiError } from "../api/client";
import { useAssignable } from "../api/hooks";
import { useSetWatching, useWatchers } from "../api/links";
import { tl } from "../i18n-links";
import { cn } from "../lib/cn";
import { useToast } from "./toast-context";
import { Avatar, Button, Select } from "./ui";

export function WatchControl({ ws, project, n, isAdmin }: { ws: string; project: string; n: number; isAdmin: boolean }) {
  const watchers = useWatchers(ws, project, n);
  const setWatching = useSetWatching(ws, project, n);
  const assignable = useAssignable(ws, project);
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false);
    };
    // Escape closes this panel, not the drawer, wherever focus is: adding a
    // watcher re-renders the list and focus can fall back to the body. The
    // capture listener on window runs before the drawer's own window handler.
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.stopPropagation();
      setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    window.addEventListener("keydown", onKey, true);
    return () => {
      document.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey, true);
    };
  }, [open]);

  const data = watchers.data;
  const watching = data?.watching ?? false;
  const count = data?.count ?? 0;
  const change = (userId: string, on: boolean) => {
    setWatching.mutate(
      { userId, on },
      { onError: (err) => { toast("error", err instanceof ApiError ? err.message : tl.watchers); } },
    );
  };
  const watcherIds = new Set((data?.watchers ?? []).map((w) => w.id));
  const addable = assignable.filter((m) => !watcherIds.has(m.user_id));

  return (
    <div ref={box} className="relative flex items-center">
      <Button
        variant="ghost"
        aria-pressed={watching}
        aria-label={tl.watchLabel(watching, count)}
        title={tl.watchLabel(watching, count)}
        disabled={!data || setWatching.isPending}
        onClick={() => { change("me", !watching); }}
        className={cn("min-h-11 gap-1 rounded-r-none md:min-h-0", watching && "text-brand-700 dark:text-brand-300")}
        data-testid="watch-toggle"
      >
        <span aria-hidden>{watching ? "👁" : "◎"}</span>
        <span className="text-xs">{watching ? tl.unwatch : tl.watch}</span>
      </Button>
      <Button
        variant="ghost"
        aria-expanded={open}
        aria-label={`${tl.watchers}: ${tl.watcherCount(count)}`}
        onClick={() => { setOpen((v) => !v); }}
        className="min-h-11 rounded-l-none px-2 text-xs tabular-nums md:min-h-0"
        data-testid="watcher-count"
      >
        {count}
      </Button>
      {open && (
        <div
          role="dialog"
          aria-label={tl.watchers}
          className="absolute top-full right-0 z-20 mt-1 w-64 rounded-md bg-white p-3 text-sm shadow-lg ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-700"
        >
          <p className="mb-2 font-semibold">{tl.watchers}</p>
          {count === 0 ? (
            <p className="text-zinc-500">{tl.noWatchers}</p>
          ) : (
            <ul className="max-h-60 space-y-1 overflow-y-auto">
              {data?.watchers.map((w) => (
                <li key={w.id} className="flex items-center gap-2">
                  <Avatar name={w.display_name} />
                  <span className="min-w-0 flex-1 truncate">{w.display_name}</span>
                  {isAdmin && (
                    <Button
                      variant="ghost"
                      aria-label={tl.removeWatcher(w.display_name)}
                      title={tl.removeWatcher(w.display_name)}
                      className="text-xs text-zinc-500"
                      onClick={() => { change(w.id, false); }}
                    >
                      ✕
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )}
          {isAdmin && addable.length > 0 && (
            <Select
              aria-label={tl.addWatcher}
              value=""
              onChange={(e) => { if (e.target.value) change(e.target.value, true); }}
              className="mt-3 w-full"
            >
              <option value="">{tl.addWatcher}…</option>
              {addable.map((m) => (
                <option key={m.user_id} value={m.user_id}>
                  {m.display_name}
                </option>
              ))}
            </Select>
          )}
        </div>
      )}
    </div>
  );
}
