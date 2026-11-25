import { describe, expect, it } from "vitest";

import type { Item } from "../api/types";
import { undoPlan } from "./bulkUndo";

const item = (number: number, version: number, patch: Partial<Item> = {}): Item =>
  ({ number, version, status: { id: "todo" }, assignee_id: null, due_date: null, fields: {}, ...patch }) as unknown as Item;

describe("undoPlan", () => {
  it("groups the items by the value they had", () => {
    const before = [item(1, 1, { status: { id: "todo" } as Item["status"] }), item(2, 4, { status: { id: "doing" } as Item["status"] }), item(3, 2, { status: { id: "todo" } as Item["status"] })];
    const after = [item(1, 2), item(2, 5), item(3, 3)];
    const plan = undoPlan({ status_id: "done" }, before, after);
    expect(plan).toHaveLength(2);
    expect(plan.find((s) => s.change.status_id === "todo")?.targets.map((t) => [t.number, t.version])).toEqual([[1, 2], [3, 3]]);
    expect(plan.find((s) => s.change.status_id === "doing")?.targets.map((t) => t.number)).toEqual([2]);
  });
  it("restores a field to its old value, null when it was empty", () => {
    const before = [item(1, 1, { fields: { paid: true } }), item(2, 1)];
    const plan = undoPlan({ fields: { paid: false } }, before, [item(1, 2), item(2, 2)]);
    expect(plan.map((s) => s.change)).toEqual(expect.arrayContaining([{ fields: { paid: true } }, { fields: { paid: null } }]));
  });
  it("skips unchanged items and cannot undo a delete", () => {
    expect(undoPlan({ assignee_id: "u1" }, [item(1, 3)], [item(1, 3)])).toEqual([]);
    expect(undoPlan({ delete: true }, [item(1, 1)], [])).toEqual([]);
  });
});
