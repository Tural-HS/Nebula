// English strings for item links and watchers (ADR-0028).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const watchers = (n: number) => `${String(n)} ${plural("en", n, { one: "watcher", other: "watchers" })}`;

export const tl = {
  section: "Links",
  types: {
    blocks: "Blocks",
    blocked_by: "Is blocked by",
    relates: "Relates to",
    duplicates: "Duplicates",
    duplicated_by: "Is duplicated by",
  },
  empty: "No links.",
  add: "Add link",
  type: "Link type",
  search: "Search items",
  searchPlaceholder: "Key (OPS-12) or title",
  noResults: "No matching items.",
  added: "Link added",
  exists: "This link already exists.",
  remove: (key: string) => `Remove link to ${key}`,
  closed: "closed",
  blocked: "Blocked",
  blockedTitle: "An open item blocks this one",
  act: {
    added: (type: string, key: string) => `added a link: ${type} ${key}`,
    removed: (type: string, key: string) => `removed a link: ${type} ${key}`,
    removedByDelete: (key: string) => `link removed because ${key} was deleted`,
  },
  watch: "Watch",
  unwatch: "Unwatch",
  watchLabel: (watching: boolean, count: number) =>
    watching ? `You are watching; ${watchers(count)}. Stop watching` : `Watch; ${watchers(count)}`,
  watchers: "Watchers",
  watcherCount: watchers,
  noWatchers: "Nobody is watching.",
  addWatcher: "Add watcher",
  chooseMember: "Choose a member",
  removeWatcher: (name: string) => `Remove ${name} from watchers`,
} satisfies Messages["links"];
