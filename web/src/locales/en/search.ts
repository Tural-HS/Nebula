// English strings for workspace-wide search (ADR-0024).
import type { Messages } from "../../i18n/locale";

export const ts = {
  open: "Search",
  placeholder: "Search items, comments or keys (QA-12)…",
  dialogTitle: "Search workspace",
  inputLabel: "Search query",
  shortcut: (mac: boolean) => (mac ? "⌘K" : "Ctrl K"),
  hint: "Type at least 2 characters. Azerbaijani letters ə, ö, ü, ş, ç, ğ, ı also match without diacritics: “odenis” → “ödəniş”.",
  items: "Items",
  comments: "Comments",
  count: (shown: number, total: number) => (total > shown ? `${String(shown)} of ${String(total)}` : String(total)),
  empty: "Nothing found.",
  fuzzy: "No exact matches. Similar titles:",
  keyMatch: "Key",
  commentBy: (author: string) => `Comment by ${author}`,
  someone: "A user",
  more: "More results",
  rateLimited: "Searching too often, please wait a moment…",
  tooLong: "The query is too long (200 characters max).",
  navHint: "↑↓ to select, Enter to open, Esc to close",
} satisfies Messages["search"];
