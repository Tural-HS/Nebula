import { describe, expect, it } from "vitest";

import { activeMentionQuery, toEditable, tokenize, toStored } from "./mentions";

const ann = { id: "11111111-1111-4111-8111-111111111111", name: "Ann" };
const annLee = { id: "22222222-2222-4222-8222-222222222222", name: "Ann Lee" };
const eli = { id: "33333333-3333-4333-8333-333333333333", name: "Əli Həsənov" };

describe("tokenize", () => {
  it("splits text and mentions", () => {
    expect(tokenize(`hi <@${ann.id}>, see <@${eli.id}>!`)).toEqual([
      { kind: "text", value: "hi " },
      { kind: "mention", id: ann.id },
      { kind: "text", value: ", see " },
      { kind: "mention", id: eli.id },
      { kind: "text", value: "!" },
    ]);
  });
  it("leaves look-alike text alone", () => {
    expect(tokenize("<@not-a-uuid> and <script>")).toEqual([{ kind: "text", value: "<@not-a-uuid> and <script>" }]);
  });
});

describe("toStored / toEditable", () => {
  it("prefers the longest matching name", () => {
    expect(toStored("@Ann Lee and @Ann", [ann, annLee])).toBe(`<@${annLee.id}> and <@${ann.id}>`);
  });
  it("handles non-ASCII names and word boundaries", () => {
    expect(toStored("salam @Əli Həsənov, @Annex", [eli, ann])).toBe(`salam <@${eli.id}>, @Annex`);
  });
  it("does not convert people who were not picked", () => {
    expect(toStored("@Ann", [])).toBe("@Ann");
  });
  it("escapes regex characters in names", () => {
    const odd = { id: "44444444-4444-4444-8444-444444444444", name: "A.B (ops)" };
    expect(toStored("hi @A.B (ops)", [odd])).toBe(`hi <@${odd.id}>`);
    expect(toStored("hi @AxB (ops)", [odd])).toBe("hi @AxB (ops)");
  });
  it("round-trips", () => {
    const stored = `ping <@${ann.id}> and <@${eli.id}>`;
    const editable = toEditable(stored, [ann, eli]);
    expect(editable).toBe("ping @Ann and @Əli Həsənov");
    expect(toStored(editable, [ann, eli])).toBe(stored);
  });
  it("marks unknown people", () => {
    expect(toEditable(`<@${annLee.id}>`, [])).toBe("@?");
  });
});

describe("activeMentionQuery", () => {
  it.each([
    ["hello @", 7, ""],
    ["hello @Əl", 9, "Əl"],
    ["mail a@b", 8, null],
    ["@ann", 4, "ann"],
    ["done @ann ", 10, null],
  ])("%s|%i -> %s", (text, caret, want) => {
    expect(activeMentionQuery(text, caret)).toBe(want);
  });
});
