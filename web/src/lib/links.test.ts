import { describe, expect, it } from "vitest";

import type { ItemLink, LinkType } from "../api/links";
import type { SearchHit } from "../api/search";
import type { StatusCategory } from "../api/types";
import { en } from "../locales/en";
import { ru } from "../locales/ru";
import { baseType, groupLinks, isClosed, linkCandidates, linkedItemHref, openBlockers } from "./links";

function split(key: string): [string, number] {
  const i = key.indexOf("-");
  return [key.slice(0, i), Number(key.slice(i + 1))];
}

function link(id: string, type: LinkType, key: string, category: StatusCategory = "todo"): ItemLink {
  const [project_key, number] = split(key);
  return {
    id, type, created_at: "2026-11-24T10:00:00Z",
    item: { key, project_key, number, title: "Item " + key, status: { id: "s", name: "S", category } },
  };
}

function hit(item_key: string, kind: "item" | "comment" = "item"): SearchHit {
  const [project_key, item_number] = split(item_key);
  return {
    kind, match: "text", project_key, project_name: project_key, item_number, item_key, item_title: "T",
    status_name: "To Do", status_category: "todo", title: [], snippet: null, comment_id: kind === "comment" ? "c" : null,
    author: null, created_at: null,
  };
}

describe("groupLinks", () => {
  it("groups by type in display order and drops empty groups", () => {
    const groups = groupLinks([link("1", "relates", "QA-3"), link("2", "blocked_by", "OPS-1"), link("3", "relates", "QA-4")]);
    expect(groups.map((g) => [g.type, g.links.map((l) => l.item.key)])).toEqual([
      ["blocked_by", ["OPS-1"]],
      ["relates", ["QA-3", "QA-4"]],
    ]);
  });
});

describe("status helpers", () => {
  it("treats done and canceled as closed", () => {
    expect([isClosed("todo"), isClosed("in_progress"), isClosed("done"), isClosed("canceled")]).toEqual([false, false, true, true]);
  });

  it("counts only open blockers", () => {
    const list = [link("1", "blocked_by", "QA-1"), link("2", "blocked_by", "QA-2", "done"), link("3", "blocks", "QA-3")];
    expect(openBlockers(list).map((l) => l.id)).toEqual(["1"]);
  });
});

describe("linkedItemHref", () => {
  it("opens the item on its own project page", () => {
    expect(linkedItemHref("acme co", link("1", "relates", "OPS-7"))).toBe("/w/acme%20co/p/OPS?item=7");
  });
});

describe("linkCandidates", () => {
  const links = [link("1", "blocked_by", "QA-2")];

  it("skips the item itself, one hit per item, and links the server would refuse", () => {
    const hits = [hit("QA-1"), hit("QA-2"), hit("QA-3"), hit("QA-3", "comment"), hit("OPS-9")];
    // QA-2 already blocks QA-1, so "blocks QA-2" is the same pair and type.
    expect(linkCandidates(hits, "QA-1", links, "blocks").map((h) => h.item_key)).toEqual(["QA-3", "OPS-9"]);
    // Another type for the same pair is a new link.
    expect(linkCandidates(hits, "QA-1", links, "relates").map((h) => h.item_key)).toEqual(["QA-2", "QA-3", "OPS-9"]);
  });

  it("maps inverse names to the stored type", () => {
    expect([baseType("blocked_by"), baseType("duplicated_by"), baseType("relates")]).toEqual(["blocks", "duplicates", "relates"]);
  });
});

describe("dictionaries", () => {
  it("pluralize watcher counts", () => {
    expect([en.links.watcherCount(1), en.links.watcherCount(2)]).toEqual(["1 watcher", "2 watchers"]);
    expect([ru.links.watcherCount(1), ru.links.watcherCount(3), ru.links.watcherCount(5)]).toEqual([
      "1 наблюдатель", "3 наблюдателя", "5 наблюдателей",
    ]);
  });
});
