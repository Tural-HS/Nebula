import { describe, expect, it } from "vitest";

import type { SearchHit } from "../api/search";
import { groupHits, hitHref, hitId, isSearchable, isSearchShortcut, moveActive } from "./search";

const hit = (patch: Partial<SearchHit>): SearchHit => ({
  kind: "item",
  match: "text",
  project_key: "QA",
  project_name: "Keyfiyyət",
  item_number: 12,
  item_key: "QA-12",
  item_title: "Ödəniş",
  status_name: "To Do",
  status_category: "todo",
  title: [{ text: "Ödəniş", match: true }],
  snippet: null,
  comment_id: null,
  author: null,
  created_at: null,
  ...patch,
});

describe("isSearchable", () => {
  it.each([
    ["", false],
    ["  ", false],
    ["a", false],
    ["!!", false],
    ["ab", true],
    ["ə ş", false],
    ["öd", true],
    ["7", true],
    ["QA-12", true],
    ["ş".repeat(200), true],
    ["ş".repeat(201), false],
  ])("%j → %s", (q, want) => {
    expect(isSearchable(q)).toBe(want);
  });
});

describe("groupHits", () => {
  it("keeps the ranking inside each group", () => {
    const g = groupHits([hit({ item_key: "QA-1" }), hit({ kind: "comment", comment_id: "c1" }), hit({ item_key: "QA-2" })]);
    expect(g.items.map((h) => h.item_key)).toEqual(["QA-1", "QA-2"]);
    expect(g.comments.map((h) => h.comment_id)).toEqual(["c1"]);
  });
});

describe("links and ids", () => {
  it("opens the item drawer and escapes path parts", () => {
    expect(hitHref("acme", hit({}))).toBe("/w/acme/p/QA?item=12");
    expect(hitHref("a/b", hit({ project_key: "Q?A" }))).toBe("/w/a%2Fb/p/Q%3FA?item=12");
  });
  it("gives items and comments distinct row ids", () => {
    expect(hitId("s", hit({}))).toBe("s-item-QA-12");
    expect(hitId("s", hit({ kind: "comment", comment_id: "c1" }))).toBe("s-comment-c1");
  });
});

describe("moveActive", () => {
  it.each([
    ["ArrowDown", 0, 3, 1],
    ["ArrowDown", 2, 3, 0],
    ["ArrowUp", 0, 3, 2],
    ["Home", 2, 3, 0],
    ["End", 0, 3, 2],
    ["Enter", 0, 3, undefined],
    ["ArrowDown", 0, 0, undefined],
  ] as const)("%s from %i of %i → %s", (key, from, n, want) => {
    expect(moveActive(key, from, n)).toBe(want);
  });
});

describe("isSearchShortcut", () => {
  const ev = (p: Partial<KeyboardEvent>) => ({ key: "k", metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, ...p });
  it("accepts Cmd+K and Ctrl+K only", () => {
    expect(isSearchShortcut(ev({ metaKey: true }))).toBe(true);
    expect(isSearchShortcut(ev({ ctrlKey: true, key: "K" }))).toBe(true);
    expect(isSearchShortcut(ev({}))).toBe(false);
    expect(isSearchShortcut(ev({ ctrlKey: true, shiftKey: true }))).toBe(false);
    expect(isSearchShortcut(ev({ metaKey: true, key: "j" }))).toBe(false);
  });
});
