import { describe, expect, it } from "vitest";

import type { Item } from "../api/types";
import { hasSignal } from "./signals";

const base = { due_date: null } as unknown as Item;
const with_ = (patch: Record<string, unknown>): Item => Object.assign({}, base, patch);

describe("hasSignal", () => {
  it("is false for a plain row", () => {
    expect(hasSignal(base, true)).toBe(false);
    expect(hasSignal(base, false)).toBe(false);
  });

  it("counts the due date only when the table has no due column", () => {
    const due = with_({ due_date: "2026-12-01" });
    expect(hasSignal(due, true)).toBe(true);
    expect(hasSignal(due, false)).toBe(false);
  });

  it("counts approvals, subtasks, blocking links and SLAs", () => {
    expect(hasSignal(with_({ approval: { approvals: 0, min_approvals: 1 } }), false)).toBe(true);
    expect(hasSignal(with_({ subtasks: { count: 2, done: 1 } }), false)).toBe(true);
    expect(hasSignal(with_({ subtasks: { count: 0, done: 0 } }), false)).toBe(false);
    expect(hasSignal(with_({ blocked: true }), false)).toBe(true);
    expect(hasSignal(with_({ sla: { state: "ok" } }), false)).toBe(true);
  });
});
