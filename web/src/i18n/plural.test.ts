import { describe, expect, it } from "vitest";

import { plural } from "./plural";

describe("plural", () => {
  const ru = { one: "задача", few: "задачи", many: "задач", other: "задачи" };
  it("picks Russian forms", () => {
    expect([1, 2, 5, 11, 21, 22, 25, 1.5].map((n) => plural("ru", n, ru))).toEqual(["задача", "задачи", "задач", "задач", "задача", "задачи", "задач", "задачи"]);
  });
  it("picks English forms", () => {
    expect([0, 1, 2].map((n) => plural("en", n, { one: "item", other: "items" }))).toEqual(["items", "item", "items"]);
  });
});
