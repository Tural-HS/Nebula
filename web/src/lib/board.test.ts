import { describe, expect, it } from "vitest";

import type { Item, ItemType, Status } from "../api/types";
import { flattenPages, groupByCategory, initials, pickTargetStatus } from "./board";

const todo: Status = { id: "s-todo", name: "To Do", category: "todo" };
const prog: Status = { id: "s-prog", name: "In Progress", category: "in_progress" };
const review: Status = { id: "s-review", name: "Review", category: "in_progress" };
const done: Status = { id: "s-done", name: "Done", category: "done" };

const bugType: ItemType = {
  id: "t-bug", key: "bug", name: "Bug", icon: null, workflow_id: "wf", fields: [],
  statuses: [todo, prog, review, done],
};

function item(id: string, status: Status): Item {
  return {
    id, key: `OPS-${id}`, number: Number(id), project_id: "p", type: { id: "t-bug", key: "bug", name: "Bug" },
    status, title: "x", description: null, assignee_id: null, reporter_id: null, fields: {}, version: 1,
    created_at: "", updated_at: "",
  };
}

describe("groupByCategory", () => {
  it("puts every item in its category column", () => {
    const g = groupByCategory([item("1", todo), item("2", prog), item("3", review), item("4", done)]);
    expect(g.todo.map((i) => i.id)).toEqual(["1"]);
    expect(g.in_progress.map((i) => i.id)).toEqual(["2", "3"]);
    expect(g.done.map((i) => i.id)).toEqual(["4"]);
    expect(g.canceled).toEqual([]);
  });
});

describe("pickTargetStatus", () => {
  const it1 = item("1", todo);

  it("picks the first allowed status of the item's workflow in the category", () => {
    expect(pickTargetStatus(it1, "in_progress", [bugType], [prog, review])?.id).toBe("s-prog");
  });
  it("skips statuses the workflow does not allow right now", () => {
    expect(pickTargetStatus(it1, "in_progress", [bugType], [review])?.id).toBe("s-review");
  });
  it("returns null when no allowed status exists in the category", () => {
    expect(pickTargetStatus(it1, "done", [bugType], [prog])).toBeNull();
  });
  it("returns null when dropped on its own column", () => {
    expect(pickTargetStatus(it1, "todo", [bugType], [todo, prog])).toBeNull();
  });
  it("falls back to the allowed list when the type is unknown", () => {
    expect(pickTargetStatus(it1, "done", [], [done])?.id).toBe("s-done");
  });
});

describe("flattenPages", () => {
  it("drops duplicates across pages", () => {
    const a = item("1", todo);
    const b = item("2", todo);
    expect(flattenPages([{ items: [a, b] }, { items: [b] }]).map((i) => i.id)).toEqual(["1", "2"]);
  });
});

describe("initials", () => {
  it.each([
    ["Alice Developer", "AD"],
    ["əli", "Ə"],
    ["  ", "?"],
    ["Tural Həsən Oğlu", "TO"],
  ])("%s -> %s", (name, want) => {
    expect(initials(name)).toBe(want);
  });
});
