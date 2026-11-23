import { describe, expect, it } from "vitest";

import type { Item, ItemType, Status } from "../api/types";
import { buildColumns, columnCount, dropTargets, flattenPages, initials, otherColumn, pickTargetStatus, type BoardColumn } from "./board";

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

// A second workflow (tasks) that shares "Done" with bugs but has its own
// "Backlog" and a differently cased "in progress".
const tBacklog: Status = { id: "t-backlog", name: "Backlog", category: "todo" };
const tProg: Status = { id: "t-prog", name: " in progress ", category: "in_progress" };
const tDone: Status = { id: "t-done", name: "Done", category: "done" };
const taskType: ItemType = {
  id: "t-task", key: "task", name: "Task", icon: null, workflow_id: "wf2", fields: [],
  statuses: [tBacklog, tProg, tDone],
};
const label = (c: string) => `[${c}]`;
const col = (cols: BoardColumn[], name: string) => {
  const c = cols.find((x) => x.name.toLowerCase() === name.toLowerCase());
  if (!c) throw new Error("no column " + name);
  return c;
};

describe("buildColumns by status", () => {
  const cols = buildColumns([bugType, taskType], "status", label);

  it("merges equal statuses of different workflows, ordered by category then position", () => {
    expect(cols.map((c) => c.name)).toEqual(["To Do", "Backlog", "In Progress", "Review", "Done"]);
    expect(col(cols, "In Progress").statusIds).toEqual(["s-prog", "t-prog"]);
    expect(col(cols, "Done").statusIds).toEqual(["s-done", "t-done"]);
  });
  it("keeps unrelated statuses apart", () => {
    expect(col(cols, "Backlog").statusIds).toEqual(["t-backlog"]);
  });
  it("does not merge equal names of different categories", () => {
    const odd: ItemType = { ...taskType, id: "t-odd", statuses: [{ id: "o-done", name: "Done", category: "canceled" }] };
    const c = buildColumns([bugType, odd], "status", label).filter((x) => x.name === "Done");
    expect(c.map((x) => x.category)).toEqual(["done", "canceled"]);
  });
  it("handles a workspace without types", () => {
    expect(buildColumns([], "status", label)).toEqual([]);
  });
});

describe("buildColumns by category", () => {
  it("has four columns holding every status of that category", () => {
    const cols = buildColumns([bugType, taskType], "category", label);
    expect(cols.map((c) => c.name)).toEqual(["[todo]", "[in_progress]", "[done]", "[canceled]"]);
    expect(cols[1]?.statusIds).toEqual(["s-prog", "s-review", "t-prog"]);
    expect(cols[3]?.statusIds).toEqual([]);
  });
});

describe("pickTargetStatus", () => {
  const cols = buildColumns([bugType, taskType], "status", label);
  const it1 = item("1", todo);

  it("picks the item's own status in a merged column", () => {
    expect(pickTargetStatus(it1, col(cols, "Done"), [bugType, taskType], [prog, done])?.id).toBe("s-done");
  });
  it("respects the workflow's allowed transitions", () => {
    expect(pickTargetStatus(it1, col(cols, "Done"), [bugType], [prog])).toBeNull();
  });
  it("never moves an item into another workflow's status", () => {
    expect(pickTargetStatus(it1, col(cols, "Backlog"), [bugType, taskType], [prog, done])).toBeNull();
  });
  it("returns null on its own column", () => {
    expect(pickTargetStatus(it1, col(cols, "To Do"), [bugType], [todo, prog])).toBeNull();
  });
  it("picks the first allowed status of a category column", () => {
    const byCat = buildColumns([bugType], "category", label);
    expect(pickTargetStatus(it1, byCat[1] as BoardColumn, [bugType], [review])?.id).toBe("s-review");
  });
  it("falls back to the allowed list when the type is unknown", () => {
    expect(pickTargetStatus(it1, col(cols, "Done"), [], [done])?.id).toBe("s-done");
  });
});

describe("dropTargets", () => {
  it("lists exactly the columns the item may move to", () => {
    const cols = buildColumns([bugType, taskType], "status", label);
    const keys = dropTargets(item("1", todo), cols, [bugType, taskType], [prog, review]);
    expect([...keys].map((k) => cols.find((c) => c.key === k)?.name)).toEqual(["In Progress", "Review"]);
  });
});

describe("otherColumn", () => {
  const cols = buildColumns([bugType], "status", label);
  it("collects counted statuses that belong to no column, read-only", () => {
    const o = otherColumn(cols, ["s-todo", "gone-1", "gone-2"], "Other");
    expect(o?.statusIds).toEqual(["gone-1", "gone-2"]);
    expect(o && pickTargetStatus(item("1", todo), o, [bugType], [done])).toBeNull();
  });
  it("is absent when every status has a column", () => {
    expect(otherColumn(cols, ["s-todo", "s-done"], "Other")).toBeNull();
  });
});

describe("columnCount", () => {
  it("sums the column's statuses and is undefined until counts load", () => {
    const merged = col(buildColumns([bugType, taskType], "status", label), "Done");
    expect(columnCount(merged, { "s-done": 2, "t-done": 3, "s-todo": 9 })).toBe(5);
    expect(columnCount(merged, undefined)).toBeUndefined();
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
