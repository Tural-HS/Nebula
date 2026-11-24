import { describe, expect, it } from "vitest";

import type { PublicFormField } from "../api/forms";
import type { FieldDefinition, ItemType } from "../api/types";
import { slugKey } from "./slug";
import { addableFields, formLink, moveField, newForm, previewTitle, requiredByType, suggestSlug, toValues, unknownPlaceholders, valueKey } from "./forms";

const f = (key: string, type: PublicFormField["type"], required = false): PublicFormField => ({ key, label: key, required, type });

const type: ItemType = {
  id: "t1", key: "task", name: "Task", icon: null, workflow_id: "w1", statuses: [],
  fields: [
    { key: "amount", required: true, position: 0 },
    { key: "vendor", required: false, position: 1 },
    { key: "approver", required: false, position: 2 },
    { key: "old", required: false, position: 3 },
  ],
};
const def = (key: string, t: FieldDefinition["type"], archived = false): FieldDefinition => ({ id: key, key, name: key.toUpperCase(), type: t, options: {}, archived });
const defs = [def("amount", "number"), def("vendor", "text"), def("approver", "user"), def("old", "text", true), def("other", "text")];

describe("toValues", () => {
  it("drops blanks, parses numbers and dates, keeps choices", () => {
    const fields = [f("vendor", "text", true), f("amount", "number", true), f("cats", "multi_select"), f("note", "long_text"), f("when", "datetime"), f("ok", "boolean")];
    const { values, problems } = toValues(fields, { vendor: "  Vendor-A ", amount: "1 200,5".replace(" ", ""), cats: ["c1"], note: "   ", when: "2026-12-01T09:30", ok: false });
    expect(problems).toEqual([]);
    expect(values).toMatchObject({ vendor: "Vendor-A", amount: 1200.5, cats: ["c1"] });
    expect(values).not.toHaveProperty("note");
    expect(values).not.toHaveProperty("ok");
    expect(typeof values.when).toBe("string");
  });

  it("reports required, unchecked and non-numeric fields in form order", () => {
    const fields = [f("vendor", "text", true), f("confirm", "boolean", true), f("amount", "number"), f("cats", "multi_select", true)];
    expect(toValues(fields, { vendor: " ", amount: "abc", cats: [] }).problems).toEqual([
      ["vendor", "required"], ["confirm", "checked"], ["amount", "number"], ["cats", "required"],
    ]);
  });
});

describe("builder helpers", () => {
  it("maps server field paths to form keys", () => {
    expect(valueKey("values.vendor")).toBe("vendor");
    expect(valueKey("fields.vendor")).toBeUndefined();
    expect(valueKey(undefined)).toBeUndefined();
  });

  it("offers built-ins and bound input fields not yet on the form", () => {
    const opts = addableFields(type, defs, [{ key: "amount", label: "A", required: true }], (k) => k);
    expect(opts.map((o) => o.key)).toEqual(["description", "due_date", "vendor"]); // no user, archived or unbound fields
  });

  it("starts a form with the type's required fields", () => {
    const form = newForm(type, defs, "Payment request");
    expect(form.fields).toEqual([{ key: "amount", label: "AMOUNT", required: true }]);
    expect(form).toMatchObject({ item_type: "task", audience: "members", enabled: true, max_files: 0, title_template: "{{form}}" });
    expect([...requiredByType(type)]).toEqual(["amount"]);
  });

  it("moves fields and ignores moves out of range", () => {
    expect(moveField(["a", "b", "c"], 0, 2)).toEqual(["b", "c", "a"]);
    expect(moveField(["a", "b", "c"], 2, 0)).toEqual(["c", "a", "b"]);
    expect(moveField(["a", "b"], 0, 5)).toEqual(["a", "b"]);
  });

  it("checks title template placeholders like the server", () => {
    const fields = [{ key: "vendor", label: "V", required: true }, { key: "description", label: "D", required: false }];
    expect(unknownPlaceholders("Pay {{field.vendor}} for {{ requester }} ({{form}})", fields)).toEqual([]);
    expect(unknownPlaceholders("{{field.iban}} {{vendor}} {{field.description}}", fields)).toEqual(["field.iban", "vendor", "field.description"]);
  });

  it("previews the item title", () => {
    expect(previewTitle("Ödəniş tələbi: {{field.vendor}}", { vendor: "Vendor\n A" }, "Carol", "Payment")).toBe("Ödəniş tələbi: Vendor A");
    expect(previewTitle("{{field.vendor}}", {}, "Carol", "Payment")).toBe("Payment");
    expect(previewTitle("{{requester}} / {{form}}", {}, "Carol", "Payment")).toBe("Carol / Payment");
  });

  it("suggests an address and builds the link", () => {
    expect(suggestSlug("Ödəniş tələbi", slugKey)).toBe("odenis-telebi");
    expect(formLink("https://nebula.test/", "odenis-telebi")).toBe("https://nebula.test/f/odenis-telebi");
  });
});
