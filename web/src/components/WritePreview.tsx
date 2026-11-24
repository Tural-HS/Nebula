import { useId, useState, type ReactNode } from "react";

import { ta } from "../i18n-attach";
import type { Person } from "../lib/mentions";
import { Markdown } from "./Markdown";

/**
 * "Write / Preview" tabs around a plain textarea (ADR-0022). The editor stays
 * mounted while previewing, so text, caret and undo history survive.
 */
export function WritePreview({ preview, people, children }: { preview: string; people?: readonly Person[]; children: ReactNode }) {
  const [tab, setTab] = useState<"write" | "preview">("write");
  const id = useId();
  return (
    <div>
      <div role="tablist" aria-label={ta.markdownHint} className="mb-1.5 flex gap-3 text-xs">
        {(["write", "preview"] as const).map((x) => (
          <button
            key={x}
            type="button"
            role="tab"
            id={`${id}-${x}`}
            aria-selected={tab === x}
            aria-controls={`${id}-panel-${x}`}
            onClick={() => { setTab(x); }}
            className={
              "-mb-px border-b-2 pb-1 font-medium pointer-coarse:min-h-11 " +
              (tab === x ? "border-brand-600 text-zinc-900 dark:text-zinc-100" : "border-transparent text-zinc-500 dark:text-zinc-400 hover:text-zinc-800 dark:hover:text-zinc-200")
            }
          >
            {x === "write" ? ta.write : ta.previewTab}
          </button>
        ))}
      </div>
      <div role="tabpanel" id={`${id}-panel-write`} aria-labelledby={`${id}-write`} hidden={tab !== "write"}>
        {children}
        <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400 max-md:hidden">{ta.markdownHint}</p>
      </div>
      {tab === "preview" && (
        <div
          role="tabpanel"
          id={`${id}-panel-preview`}
          aria-labelledby={`${id}-preview`}
          className="min-h-20 rounded-md px-3 py-2 ring-1 ring-zinc-200 dark:ring-zinc-800"
        >
          {preview.trim() ? <Markdown source={preview} people={people} /> : <p className="text-sm text-zinc-500 dark:text-zinc-400">{ta.nothingToPreview}</p>}
        </div>
      )}
    </div>
  );
}
