import { describe as suite, expect, it } from "vitest";

import type { Activity } from "../api/types";
import { actorName, describe } from "./activity";

const lookup = {
  status: (id: string) => ({ s1: "To Do", s2: "Done" })[id],
  person: (id: string) => ({ u1: "Bob" })[id],
  field: (key: string) => ({ severity: "Severity" })[key],
};

function act(action: string, data: Record<string, unknown> = {}, extra: Partial<Activity> = {}): Activity {
  return {
    id: 1, action, entity_type: "item", entity_id: "i", actor: { id: "u9", display_name: "Carol" },
    source: "user", automation_rule_id: null, data, occurred_at: "2026-11-23T10:00:00Z", ...extra,
  };
}

suite("describe", () => {
  it("renders each change of an update", () => {
    const lines = describe(
      act("item.updated", {
        changes: {
          title: { from: "Old", to: "New" },
          status_id: { from: "s1", to: "s2" },
          assignee_id: { from: null, to: "u1" },
          "fields.severity": { from: "c1", to: "c2" },
          description: { from: "a", to: "b" },
        },
      }),
      lookup,
    );
    expect(lines).toEqual([
      { text: "başlığı dəyişdi", from: "Old", to: "New" },
      { text: "statusu dəyişdi", from: "To Do", to: "Done" },
      { text: "icraçını dəyişdi", from: "Təyin edilməyib", to: "Bob" },
      { text: '"Severity" sahəsini dəyişdi' },
      { text: "təsviri yenilədi" },
    ]);
  });

  it("handles unknown references and actions without crashing", () => {
    expect(describe(act("item.updated", { changes: { status_id: { from: "gone", to: "s2" } } }), lookup)[0]?.from).toBe("?");
    expect(describe(act("something.new"), lookup)).toEqual([{ text: "something.new" }]);
    expect(describe(act("item.updated", {}), lookup)).toEqual([{ text: "item.updated" }]);
  });

  it("covers lifecycle and comment events", () => {
    expect(describe(act("item.created"), lookup)[0]?.text).toBe("tapşırığı yaratdı");
    expect(describe(act("comment.created"), lookup)[0]?.text).toBe("şərh yazdı");
  });
});

suite("actorName", () => {
  it("names automation and unknown actors", () => {
    expect(actorName(act("x"))).toBe("Carol");
    expect(actorName(act("x", {}, { actor: null, source: "automation" }))).toBe("Avtomatlaşdırma");
    expect(actorName(act("x", {}, { actor: null, source: "system" }))).toBe("Sistem");
    expect(actorName(act("x", {}, { actor: null, source: "user" }))).toBe("Kimsə"); // deleted user
  });
});

suite("due dates and overdue events", () => {
  it("shows due date changes as calendar dates", () => {
    const lines = describe(act("item.updated", { changes: { due_date: { from: null, to: "2026-11-20" } } }), lookup);
    expect(lines).toEqual([{ text: "son tarixi dəyişdi", from: "—", to: "20 noy 2026" }]);
  });
  it("describes the scheduler's overdue event, attributed to the system", () => {
    const a = act("item.overdue", { days_overdue: 3 }, { source: "system", actor: null });
    expect(describe(a, lookup)).toEqual([{ text: "son tarixi keçdi (3 gün gecikir)" }]);
    expect(actorName(a)).toBe("Sistem");
  });
});
