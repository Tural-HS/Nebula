// Helpers for the Markdown renderer (components/Markdown.tsx, ADR-0022).

const MENTION = /<@([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})>/g;
const SAFE_SCHEMES = new Set(["http", "https", "mailto"]);

/**
 * Keeps only links a reader can follow safely: http(s), mailto, same-site
 * paths and in-page anchors. Everything else (javascript:, data:, vbscript:,
 * protocol-relative //host, file:, obfuscated schemes) becomes "", which
 * renders as plain text.
 */
export function safeUrl(url: string): string {
  const u = url.trim();
  if (u.startsWith("#")) return u;
  if (u.startsWith("/")) return u.startsWith("//") || u.startsWith("/\\") ? "" : u;
  const m = /^([a-z][a-z0-9+.-]*):/i.exec(u);
  return m && SAFE_SCHEMES.has((m[1] ?? "").toLowerCase()) ? u : "";
}

// Minimal mdast shapes; the full types are transitive dependencies only.
interface MdNode {
  type: string;
  value?: string;
  children?: MdNode[];
  data?: { hName?: string; hProperties?: Record<string, string> };
}

function splitMentions(value: string): MdNode[] | null {
  const out: MdNode[] = [];
  let last = 0;
  for (const m of value.matchAll(MENTION)) {
    if (m.index > last) out.push({ type: "text", value: value.slice(last, m.index) });
    out.push({
      type: "mention",
      children: [],
      data: { hName: "span", hProperties: { dataMention: (m[1] ?? "").toLowerCase() } },
    });
    last = m.index + m[0].length;
  }
  if (out.length === 0) return null;
  if (last < value.length) out.push({ type: "text", value: value.slice(last) });
  return out;
}

function walk(node: MdNode): void {
  if (!node.children) return;
  const next: MdNode[] = [];
  for (const child of node.children) {
    // Only prose: mentions inside `code` and fenced blocks stay literal.
    const parts = child.type === "text" && child.value ? splitMentions(child.value) : null;
    if (parts) next.push(...parts);
    else {
      walk(child);
      next.push(child);
    }
  }
  node.children = next;
}

/** remark plugin: <@uuid> tokens in text become mention spans. */
export function mentionPlugin() {
  return (tree: MdNode) => {
    walk(tree);
  };
}
