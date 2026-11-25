import { useEffect, useId, useRef, useState, type DragEvent } from "react";

import {
  INLINE_IMAGE_TYPES,
  downloadAttachment,
  imageUrl,
  useAttachmentMutations,
  useAttachments,
  type Attachment,
} from "../api/attachments";
import { ApiError } from "../api/client";
import { ta } from "../i18n-attach";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import { formatBytes, formatRelative } from "../lib/format";
import { useToast } from "./toast-context";
import { Button, ErrorState, Spinner } from "./ui";
import { useConfirm } from "./confirm-context";

interface Props {
  ws: string;
  project: string;
  n: number;
  meId: string | undefined;
  canWrite: boolean;
  isAdmin: boolean;
}

/** An item's files: list, image previews, upload by button or drag & drop. */
export function Attachments({ ws, project, n, meId, canWrite, isAdmin }: Props) {
  const confirm = useConfirm();
  const q = useAttachments(ws, project, n);
  const m = useAttachmentMutations(ws, project, n);
  const toast = useToast();
  const input = useRef<HTMLInputElement>(null);
  const headingId = useId();
  const [dragging, setDragging] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);

  const error = (err: unknown) => {
    toast("error", err instanceof ApiError ? err.message : t.errorGeneric);
  };

  const uploadAll = async (files: File[]) => {
    const max = q.data?.limits.max_file_bytes ?? Infinity;
    for (const f of files) {
      if (f.size > max) {
        toast("error", ta.tooLarge(f.name, Math.floor(max / (1024 * 1024))));
        continue;
      }
      setBusy(f.name);
      try {
        await m.upload.mutateAsync(f);
        toast("success", ta.uploaded(f.name));
      } catch (err) {
        error(err);
      }
    }
    setBusy(null);
  };

  const enabled = q.data?.enabled ?? true;
  const canUpload = canWrite && enabled && busy === null;
  const onDrop = (e: DragEvent) => {
    if (!canWrite || !enabled) return;
    e.preventDefault();
    setDragging(false);
    if (busy === null) void uploadAll([...e.dataTransfer.files]);
  };

  return (
    <section
      aria-labelledby={headingId}
      onDragOver={(e) => {
        if (!canWrite || !enabled || !e.dataTransfer.types.includes("Files")) return;
        e.preventDefault();
        setDragging(true);
      }}
      onDragLeave={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDragging(false);
      }}
      onDrop={onDrop}
      className={cn("rounded-lg", dragging && "ring-2 ring-brand-500 ring-offset-4 dark:ring-offset-zinc-900")}
      data-testid="attachments"
    >
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <h3 id={headingId} className="text-sm font-semibold">
          {ta.attachments}
          {q.data && q.data.attachments.length > 0 && <span className="ml-1.5 font-normal text-zinc-500 dark:text-zinc-400">{q.data.attachments.length}</span>}
        </h3>
        {canWrite && enabled && (
          <>
            <input
              ref={input}
              type="file"
              multiple
              className="sr-only"
              tabIndex={-1}
              aria-label={ta.attach}
              onChange={(e) => {
                const files = [...(e.target.files ?? [])];
                e.target.value = "";
                void uploadAll(files);
              }}
            />
            <Button disabled={!canUpload} onClick={() => input.current?.click()}>
              📎 {ta.attach}
            </Button>
          </>
        )}
      </div>

      {q.isPending ? (
        <Spinner />
      ) : q.isError ? (
        <ErrorState error={q.error} onRetry={() => void q.refetch()} />
      ) : (
        <>
          {!enabled && <p className="text-sm text-zinc-500 dark:text-zinc-400">{ta.disabled}</p>}
          {q.data.attachments.length === 0 && enabled && (
            <p className="text-sm text-zinc-500 dark:text-zinc-400">
              {ta.none} {canWrite && <span className="max-md:hidden">{ta.dropHint}</span>}
            </p>
          )}
          <ul className="space-y-2">
            {q.data.attachments.map((a) => (
              <AttachmentRow
                key={a.id}
                a={a}
                ws={ws}
                project={project}
                n={n}
                canDelete={canWrite && (a.uploader?.id === meId || isAdmin)}
                onDownload={() => { downloadAttachment(ws, project, n, a).catch(error); }}
                onDelete={() => {
                  void confirm({ title: t.areYouSure, body: ta.confirmRemove(a.filename), action: t.delete, danger: true }).then((ok) => {
                    if (ok) m.remove.mutate(a.id, { onSuccess: () => { toast("success", ta.removed); }, onError: error });
                  });
                }}
              />
            ))}
          </ul>
          {busy !== null && (
            <p role="status" className="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
              {ta.uploading(busy)}
            </p>
          )}
          {dragging && <p className="mt-2 text-center text-sm font-medium text-brand-700 dark:text-brand-100">{ta.dropHere}</p>}
        </>
      )}
    </section>
  );
}

function AttachmentRow({ a, ws, project, n, canDelete, onDownload, onDelete }: {
  a: Attachment; ws: string; project: string; n: number; canDelete: boolean; onDownload: () => void; onDelete: () => void;
}) {
  const isImage = INLINE_IMAGE_TYPES.has(a.content_type);
  const [src, setSrc] = useState<string | null>(null);
  const [expanded, setExpanded] = useState(false);
  useEffect(() => {
    if (!isImage) return;
    let url: string | null = null;
    let live = true;
    imageUrl(ws, project, n, a.id)
      .then((u) => {
        url = u;
        if (live) setSrc(u);
        else URL.revokeObjectURL(u);
      })
      .catch(() => undefined); // the row still offers a download
    return () => {
      live = false;
      if (url) URL.revokeObjectURL(url);
    };
  }, [isImage, ws, project, n, a.id]);

  return (
    <li className="rounded-md ring-1 ring-zinc-200 dark:ring-zinc-800">
      <div className="flex items-center gap-3 p-2">
        {isImage && src ? (
          <button
            type="button"
            onClick={() => { setExpanded((x) => !x); }}
            aria-label={ta.previewOf(a.filename)}
            aria-expanded={expanded}
            className="shrink-0"
          >
            <img src={src} alt="" className="size-12 rounded object-cover" />
          </button>
        ) : (
          <span aria-hidden className="flex size-12 shrink-0 items-center justify-center rounded bg-zinc-100 text-xl dark:bg-zinc-800">
            {isImage ? "🖼" : "📄"}
          </span>
        )}
        <div className="min-w-0 flex-1">
          <button
            type="button"
            onClick={onDownload}
            title={ta.download}
            className="block max-w-full truncate text-left text-sm font-medium text-brand-700 hover:underline pointer-coarse:py-2 dark:text-brand-100"
          >
            {a.filename}
          </button>
          <p className="truncate text-xs text-zinc-500 dark:text-zinc-400">
            {formatBytes(a.size)} · <span title={a.created_at}>{formatRelative(a.created_at)}</span>
            {a.uploader && <> · {ta.by(a.uploader.display_name)}</>}
          </p>
        </div>
        {canDelete && (
          <Button variant="ghost" className="shrink-0 text-red-600" onClick={onDelete} aria-label={`${ta.remove}: ${a.filename}`}>
            ✕
          </Button>
        )}
      </div>
      {expanded && src && (
        <div className="border-t border-zinc-200 p-2 dark:border-zinc-800">
          <img src={src} alt={a.filename} className="mx-auto max-h-96 max-w-full rounded" />
        </div>
      )}
    </li>
  );
}
