import { useCallback, useState, type ReactNode } from "react";

import { t } from "../i18n";
import { ToastContext, type Kind } from "./toast-context";

interface Toast {
  id: number;
  kind: Kind;
  text: string;
}

let nextId = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const dismiss = useCallback((id: number) => {
    setToasts((ts) => ts.filter((x) => x.id !== id));
  }, []);
  const push = useCallback((kind: Kind, text: string) => {
    const id = nextId++;
    setToasts((ts) => [...ts.slice(-3), { id, kind, text }]);
    // Errors stay until dismissed (WCAG 2.2.1): a failed save must not vanish
    // before a slow reader or a screen-magnifier user has found it.
    if (kind !== "error") setTimeout(() => { dismiss(id); }, 3000);
  }, [dismiss]);
  return (
    <ToastContext value={push}>
      {children}
      {/* No aria-live on the stack: each toast carries its own role, and a
          role=alert nested in a live region was announced twice or not at all. */}
      <div className="pointer-events-none fixed right-4 bottom-[max(1rem,env(safe-area-inset-bottom))] left-4 z-50 flex flex-col items-end gap-2">
        {toasts.map((x) =>
          x.kind === "error" ? (
            <div key={x.id} role="alert" className="pointer-events-auto flex max-w-sm items-start gap-2 rounded-lg bg-red-600 py-2.5 pr-2 pl-4 text-sm text-white shadow-lg">
              <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">{x.text}</span>
              <button
                type="button"
                aria-label={t.close}
                title={t.close}
                onClick={() => { dismiss(x.id); }}
                className="-my-1 shrink-0 rounded px-1.5 text-base leading-6 hover:bg-red-700 focus-visible:outline-white pointer-coarse:min-h-11 pointer-coarse:min-w-11"
              >
                <span aria-hidden>✕</span>
              </button>
            </div>
          ) : (
            <div key={x.id} role="status" className="pointer-events-auto max-w-sm rounded-lg bg-zinc-900 px-4 py-2.5 text-sm text-white shadow-lg dark:bg-zinc-100 dark:text-zinc-900">
              {x.text}
            </div>
          ),
        )}
      </div>
    </ToastContext>
  );
}
