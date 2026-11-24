// English strings for attachments and the Markdown editor (ADR-0022).
import type { Messages } from "../../i18n/locale";

export const ta = {
  attachments: "Files",
  attach: "Attach file",
  dropHere: "Drop files here",
  dropHint: "or drag and drop",
  uploading: (name: string) => `Uploading ${name}…`,
  uploaded: (name: string) => `${name} uploaded`,
  none: "No files.",
  download: "Download",
  remove: "Delete file",
  confirmRemove: (name: string) => `Delete the file "${name}"? This cannot be undone.`,
  removed: "File deleted",
  tooLarge: (name: string, mb: number) => `${name}: file is larger than ${String(mb)} MB`,
  disabled: "File attachments are disabled on this server.",
  by: (name: string) => `uploaded by ${name}`,
  preview: "Preview",
  previewOf: (name: string) => `Preview of ${name}`,
  image: "image",
  write: "Write",
  previewTab: "Preview",
  nothingToPreview: "Nothing to preview.",
  markdownHint: "Markdown supported: **bold**, _italic_, `code`, lists, tables, [link](https://…)",
  editDescription: "Edit description",
  noDescription: "No description.",
  save: "Save",
  cancel: "Cancel",
  act: {
    added: (name: string) => `attached "${name}"`,
    removed: (name: string) => `deleted the file "${name}"`,
  },
} satisfies Messages["attach"];
