import { describe, expect, it } from "vitest";

import type { Absence } from "../api/absences";
import type { Member } from "../api/types";
import { absenceOf, absenceProblem, canBeAbsent, delegateCandidates, joinNames, standingInFor } from "./absences";

const person = (id: string, name: string) => ({ id, display_name: name });

function absence(p: Partial<Absence> & Pick<Absence, "id" | "user">): Absence {
  return {
    delegate: null, starts_on: "2026-11-25", ends_on: "2026-11-28", note: null, active: true, delegate_absent: false,
    created_by: null, created_at: "", updated_at: "", ended_at: null, can_edit: true, ...p,
  };
}

describe("absenceProblem", () => {
  const today = "2026-11-25";
  const ok = { starts_on: "2026-11-25", ends_on: "2026-11-28", delegate_id: null };
  it("accepts a valid absence, today included", () => {
    expect(absenceProblem(ok, today, true)).toBeNull();
    expect(absenceProblem({ ...ok, ends_on: "2026-11-25" }, today, true)).toBeNull();
  });
  it("mirrors the server's date rules", () => {
    expect(absenceProblem({ ...ok, starts_on: "" }, today, true)).toBe("invalid");
    expect(absenceProblem({ ...ok, starts_on: "25.11.2026" }, today, true)).toBe("invalid");
    expect(absenceProblem({ ...ok, ends_on: "2026-11-24" }, today, true)).toBe("order");
    expect(absenceProblem({ starts_on: "2026-11-20", ends_on: "2026-11-24", delegate_id: null }, today, false)).toBe("endPast");
    expect(absenceProblem({ ...ok, starts_on: "2026-11-24" }, today, true)).toBe("startPast");
    expect(absenceProblem({ ...ok, ends_on: "2027-11-27" }, today, true)).toBe("tooLong");
    expect(absenceProblem({ starts_on: "2026-11-25", ends_on: "2027-11-26", delegate_id: null }, today, true)).toBeNull(); // 366 days
    expect(absenceProblem({ starts_on: "2027-11-26", ends_on: "2027-11-27", delegate_id: null }, today, true)).toBe("tooFar");
  });
  it("lets a begun absence keep its past start", () => {
    expect(absenceProblem({ ...ok, starts_on: "2026-11-20" }, today, false)).toBeNull();
  });
  it("refuses the member as their own delegate", () => {
    expect(absenceProblem({ ...ok, user_id: "u1", delegate_id: "u1" }, today, true)).toBe("selfDelegate");
  });
});

describe("delegates and banners", () => {
  const members: Member[] = [
    { user_id: "u3", email: "", display_name: "Nəzirə", role: "member" },
    { user_id: "u1", email: "", display_name: "Elmar", role: "admin" },
    { user_id: "u2", email: "", display_name: "Bəhruz", role: "guest" },
    { user_id: "u4", email: "", display_name: "Aysel", role: "owner" },
  ];
  it("offers other non-guest members, sorted", () => {
    expect(delegateCandidates(members, "u1").map((m) => m.display_name)).toEqual(["Aysel", "Nəzirə"]);
    expect(canBeAbsent(members[2] as Member)).toBe(false);
  });
  const list = [
    absence({ id: "a1", user: person("u1", "Elmar"), delegate: person("u3", "Nəzirə") }),
    absence({ id: "a2", user: person("u4", "Aysel"), delegate: person("u3", "Nəzirə"), active: false, starts_on: "2026-12-01", ends_on: "2026-12-05" }),
    absence({ id: "a3", user: person("u4", "Aysel"), active: false, starts_on: "2026-11-30", ends_on: "2026-11-30" }),
  ];
  it("finds the current, else the next absence", () => {
    expect(absenceOf(list, "u1")?.id).toBe("a1");
    expect(absenceOf(list, "u4")?.id).toBe("a3");
    expect(absenceOf(list, "u3")).toBeUndefined();
    expect(absenceOf(undefined, "u1")).toBeUndefined();
  });
  it("lists whom a delegate stands in for today, nobody while away", () => {
    expect(standingInFor(list, "u3").map((a) => a.id)).toEqual(["a1"]);
    const away = [...list, absence({ id: "a4", user: person("u3", "Nəzirə") })];
    expect(standingInFor(away, "u3")).toEqual([]);
    expect(joinNames([person("u1", "Elmar"), person("u4", "Aysel")])).toBe("Elmar, Aysel");
  });
});
