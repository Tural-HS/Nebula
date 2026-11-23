import { useCallback, useState, type ReactNode } from "react";

import { cn } from "../lib/cn";
import { ToastContext, type Kind } from "./toast-context";

interface Toast {
  id: number;
  kind: Kind;
  text: string;
}

let nextId = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const push = useCallback((kind: Kind, text: string) => {
    const id = nextId++;
    setToasts((ts) => [...ts.slice(-3), { id, kind, text }]);
    setTimeout(() => {
      setToasts((ts) => ts.filter((x) => x.id !== id));
    }, kind === "error" ? 6000 : 3000);
  }, []);
  return (
    <ToastContext value={push}>
      {children}
      <div aria-live="polite" className="pointer-events-none fixed right-4 bottom-[max(1rem,env(safe-area-inset-bottom))] left-4 z-50 flex flex-col items-end gap-2">
        {toasts.map((x) => (
          <div
            key={x.id}
            role={x.kind === "error" ? "alert" : "status"}
            className={cn(
              "pointer-events-auto max-w-sm rounded-lg px-4 py-2.5 text-sm shadow-lg",
              x.kind === "error" ? "bg-red-600 text-white" : "bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900",
            )}
          >
            {x.text}
          </div>
        ))}
      </div>
    </ToastContext>
  );
}
