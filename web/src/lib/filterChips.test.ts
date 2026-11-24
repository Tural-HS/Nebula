import { describe, expect, it } from "vitest";

import type { FieldDefinition, Member, Status } from "../api/types";
import { filterChips } from "./filterChips";
import { emptyState } from "./views";

const S1 = "00000000-0000-4000-8000-000000000001";
const statuses: Status[] = [{ id: S1, name: "İcrada", category: "in_progress" }];
const fields = new Map<string, FieldDefinition>([
  ["prio", { id: "f1", key: "prio", name: "Prioritet", type: "select", options: { choices: [{ id: "c1", label: "Yüksək" }] }, archived: false }],
]);
const members = new Map<string, Member>();

describe("filterChips", () => {
  it("is empty without filters", () => {
    expect(filterChips(emptyState(), statuses, fields, members)).toEqual([]);
  });
  it("names each filter and removes only it", () => {
    const s = { ...emptyState(), assignee: "me", due: "overdue" as const, blocked: true, status: [S1], fields: { prio: "c1", other: "x" } };
    const chips = filterChips(s, statuses, fields, members);
    expect(chips.map((c) => c.key)).toEqual(["assignee", "due", "blocked", "status", "field.prio", "field.other"]);
    expect(chips.find((c) => c.key === "status")?.label).toContain("İcrada");
    expect(chips.find((c) => c.key === "field.prio")?.label).toBe("Prioritet: Yüksək");
    expect(chips.find((c) => c.key === "field.prio")?.clear).toEqual({ fields: { other: "x" } });
    expect(chips.find((c) => c.key === "blocked")?.clear).toEqual({ blocked: false });
  });
});
