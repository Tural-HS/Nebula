import { useRef, useState } from "react";

import { ta } from "../i18n-attach";
import { t } from "../i18n";
import { Markdown } from "./Markdown";
import { Button } from "./ui";
import { WritePreview } from "./WritePreview";

/**
 * An item description: rendered Markdown, edited in a textarea with a
 * preview tab (ADR-0022). Like before, leaving the editor saves; Cancel
 * discards the draft.
 */
export function DescriptionField({ value, canEdit, onSave }: { value: string | null; canEdit: boolean; onSave: (v: string | null) => void }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value ?? "");
  const box = useRef<HTMLDivElement>(null);

  const commit = () => {
    if (draft !== (value ?? "")) onSave(draft.trim() ? draft : null);
    setEditing(false);
  };
  const start = () => {
    setDraft(value ?? "");
    setEditing(true);
  };

  return (
    <div>
      <div className="mb-1 flex items-center justify-between gap-2">
        {editing ? (
          <label htmlFor="item-desc" className="text-xs font-medium text-zinc-600 dark:text-zinc-400">
            {t.description}
          </label>
        ) : (
          <span className="text-xs font-medium text-zinc-600 dark:text-zinc-400">{t.description}</span>
        )}
        {canEdit && !editing && (
          <Button variant="ghost" className="-my-1" onClick={start} aria-label={ta.editDescription}>
            {t.edit}
          </Button>
        )}
      </div>
      {editing ? (
        <div
          ref={box}
          onBlur={(e) => {
            // Save when focus leaves the editor (tabs and buttons are inside it).
            if (!box.current?.contains(e.relatedTarget)) commit();
          }}
        >
          <WritePreview preview={draft}>
            <textarea
              id="item-desc"
              rows={8}
              value={draft}
              maxLength={100_000}
              autoFocus
              onChange={(e) => { setDraft(e.target.value); }}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault();
                  commit();
                }
              }}
              className="w-full rounded-md bg-white px-3 py-2 font-mono text-sm ring-1 pointer-coarse:text-base ring-zinc-300 focus:ring-2 focus:ring-brand-500 dark:bg-zinc-900 dark:ring-zinc-700"
            />
          </WritePreview>
          <div className="mt-2 flex justify-end gap-2">
            <Button onClick={() => { setDraft(value ?? ""); setEditing(false); }}>{ta.cancel}</Button>
            <Button variant="primary" onClick={commit}>
              {ta.save}
            </Button>
          </div>
        </div>
      ) : value ? (
        <Markdown source={value} />
      ) : (
        <p className="text-sm text-zinc-500 dark:text-zinc-400">{ta.noDescription}</p>
      )}
    </div>
  );
}
