import { describe, expect, it } from "vitest";

import { assignable } from "./hooks";
import type { Member, Role } from "./types";

const m = (id: string, role: Role): Member => ({ user_id: id, email: id + "@x", display_name: id, role });
const all = [m("owner", "owner"), m("admin", "admin"), m("carol", "member"), m("bob", "guest")];

describe("assignable", () => {
  it("offers everyone in a workspace-visible project", () => {
    expect(assignable(all, false, []).map((x) => x.user_id)).toEqual(["owner", "admin", "carol", "bob"]);
  });
  it("offers project members and workspace admins in a private project", () => {
    expect(assignable(all, true, [{ user_id: "bob" }]).map((x) => x.user_id)).toEqual(["owner", "admin", "bob"]);
  });
});
