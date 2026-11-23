// File attachments on items (ADR-0022). Uploads are multipart; downloads are
// fetched with the bearer token and handed to the browser as blob: URLs.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiBlob, api, apiUpload, path } from "./client";
import { collabKeys } from "./hooks";

export interface Attachment {
  id: string;
  item_id: string;
  comment_id: string | null;
  uploader: { id: string; display_name: string } | null;
  filename: string;
  content_type: string;
  size: number;
  sha256: string;
  created_at: string;
}

export interface AttachmentList {
  attachments: Attachment[];
  enabled: boolean;
  limits: { max_file_bytes: number; max_per_item: number };
}

/** Types the server will serve inline (matches inlineImageTypes in Go). */
export const INLINE_IMAGE_TYPES = new Set(["image/png", "image/jpeg", "image/gif", "image/webp"]);

export const attachmentKeys = {
  list: (ws: string, project: string, n: number) => ["ws", ws, "p", project, "item", n, "attachments"] as const,
};

const base = (ws: string, project: string, n: number) => path`/workspaces/${ws}/projects/${project}/items/${n}/attachments`;

export const contentPath = (ws: string, project: string, n: number, id: string, inline = false) =>
  base(ws, project, n) + path`/${id}/content` + (inline ? "?inline=1" : "");

export const useAttachments = (ws: string, project: string, n: number) =>
  useQuery({ queryKey: attachmentKeys.list(ws, project, n), queryFn: () => api<AttachmentList>("GET", base(ws, project, n)) });

export function useAttachmentMutations(ws: string, project: string, n: number) {
  const qc = useQueryClient();
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: attachmentKeys.list(ws, project, n) });
    void qc.invalidateQueries({ queryKey: collabKeys.activity(ws, project, n) });
  };
  return {
    upload: useMutation({
      mutationFn: async (file: File) => {
        const form = new FormData();
        form.append("file", file, file.name);
        return (await apiUpload(base(ws, project, n), form)).body as Attachment;
      },
      onSettled: invalidate,
    }),
    remove: useMutation({
      mutationFn: (id: string) => api<undefined>("DELETE", base(ws, project, n) + path`/${id}`),
      onSuccess: invalidate,
    }),
  };
}

/** Saves a file through a temporary blob: URL (the browser's download UI). */
export async function downloadAttachment(ws: string, project: string, n: number, a: Attachment): Promise<void> {
  const blob = await apiBlob(contentPath(ws, project, n, a.id));
  const url = URL.createObjectURL(blob);
  try {
    const link = document.createElement("a");
    link.href = url;
    link.download = a.filename;
    link.rel = "noopener";
    document.body.append(link);
    link.click();
    link.remove();
  } finally {
    // Give the browser a moment to start the download before revoking.
    setTimeout(() => { URL.revokeObjectURL(url); }, 30_000);
  }
}

/** Loads an inline image preview as a blob: URL (revoked by the caller). */
export async function imageUrl(ws: string, project: string, n: number, id: string): Promise<string> {
  return URL.createObjectURL(await apiBlob(contentPath(ws, project, n, id, true)));
}
