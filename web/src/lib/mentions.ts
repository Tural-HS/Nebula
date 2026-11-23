// Mentions are stored as <@user-id> tokens (IDs are unique and stable; names
// are not). The composer shows "@Display Name" and converts on submit.

const TOKEN = /<@([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})>/g;

export type Part = { kind: "text"; value: string } | { kind: "mention"; id: string };

/** Splits a stored comment body into text and mention parts for rendering. */
export function tokenize(body: string): Part[] {
  const parts: Part[] = [];
  let last = 0;
  for (const m of body.matchAll(TOKEN)) {
    const at = m.index;
    if (at > last) parts.push({ kind: "text", value: body.slice(last, at) });
    parts.push({ kind: "mention", id: (m[1] ?? "").toLowerCase() });
    last = at + m[0].length;
  }
  if (last < body.length) parts.push({ kind: "text", value: body.slice(last) });
  return parts;
}

export interface Person {
  id: string;
  name: string;
}

/** Stored body -> editable text with "@Name" (unknown IDs become "@?"). */
export function toEditable(body: string, people: readonly Person[]): string {
  const byId = new Map(people.map((p) => [p.id.toLowerCase(), p.name]));
  return tokenize(body)
    .map((p) => (p.kind === "text" ? p.value : "@" + (byId.get(p.id) ?? "?")))
    .join("");
}

/**
 * Editable text -> stored body. Only people picked in the composer (or already
 * mentioned) are converted, longest names first so "@Ann Lee" wins over "@Ann".
 * A name must be followed by a non-word character or the end of the text.
 */
export function toStored(text: string, picked: readonly Person[]): string {
  const sorted = [...picked].sort((a, b) => b.name.length - a.name.length);
  let out = text;
  for (const p of sorted) {
    const escaped = p.name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    out = out.replace(new RegExp(`@${escaped}(?![\\p{L}\\p{N}_])`, "gu"), `<@${p.id}>`);
  }
  return out;
}

/** The "@query" being typed right before the caret, if any. */
export function activeMentionQuery(text: string, caret: number): string | null {
  const m = /(^|\s)@([\p{L}\p{N}._-]{0,40})$/u.exec(text.slice(0, caret));
  return m ? (m[2] ?? "") : null;
}
