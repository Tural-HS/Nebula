import { describe, expect, it } from "vitest";

import type { ImportTarget } from "../api/imports";
import {
  checkFile,
  columnLabel,
  columnLetter,
  formatBytes,
  initialMapping,
  isMappingValid,
  mappingProblems,
  sampleValues,
  setTarget,
  targetLabel,
  toPayload,
} from "./importMapping";

const targets: ImportTarget[] = [
  { id: "title", kind: "builtin", required: true },
  { id: "assignee", kind: "builtin" },
  { id: "field.branch", kind: "field", key: "branch", name: "Filial", type: "select" },
];

describe("import mapping", () => {
  it("starts from the server suggestions and ignores unknown targets or columns", () => {
    const m = initialMapping({
      headers: ["Müştəri", "Filial", "Qeyd"],
      targets,
      suggested: [
        { column: 0, target: "title" },
        { column: 1, target: "field.branch" },
        { column: 2, target: "field.gone" },
        { column: 9, target: "assignee" },
      ],
    });
    expect(m).toEqual(["title", "field.branch", ""]);
  });

  it("keeps each target on one column", () => {
    const m = setTarget(["title", "field.branch", ""], 2, "title");
    expect(m).toEqual(["", "field.branch", "title"]);
    expect(setTarget(m, 1, "")).toEqual(["", "", "title"]);
  });

  it("requires a title column and reports duplicates", () => {
    expect(mappingProblems(["", "field.branch"])).toEqual({ missingTitle: true, duplicates: [] });
    expect(mappingProblems(["title", "assignee", "assignee"])).toEqual({ missingTitle: false, duplicates: ["assignee"] });
    expect(isMappingValid(["title", ""])).toBe(true);
    expect(isMappingValid(["", ""])).toBe(false);
  });

  it("sends only mapped columns", () => {
    expect(toPayload(["title", "", "field.branch"], "task")).toEqual({
      columns: [
        { column: 0, target: "title" },
        { column: 2, target: "field.branch" },
      ],
      default_type: "task",
    });
  });

  it("names columns like a spreadsheet", () => {
    expect([0, 25, 26, 701, 702].map(columnLetter)).toEqual(["A", "Z", "AA", "ZZ", "AAA"]);
    expect(columnLabel(1, " Filial ")).toBe("B · Filial");
    expect(columnLabel(3, "")).toBe("D");
  });

  it("checks files before uploading", () => {
    expect(checkFile({ name: "Müştərilər.XLSX", size: 1000 })).toBeNull();
    expect(checkFile({ name: "a.csv", size: 0 })).toBe("empty");
    expect(checkFile({ name: "a.xls", size: 10 })).toBe("extension");
    expect(checkFile({ name: "a.pdf", size: 10 })).toBe("extension");
    expect(checkFile({ name: "a.csv", size: 10 * 1024 * 1024 + 1 })).toBe("size");
  });

  it("formats sizes, labels and samples", () => {
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(2048)).toBe("2 KB");
    expect(formatBytes(1.5 * 1024 * 1024)).toBe("1,5 MB");
    expect(targetLabel(targets[2], {})).toBe("Filial");
    expect(targetLabel(targets[0], { title: "Başlıq" })).toBe("Başlıq");
    expect(targetLabel(undefined, {})).toBe("");
    const rows = [{ cells: ["a", " "] }, { cells: ["b", "x"] }, { cells: ["c", ""] }, { cells: ["d", "y"] }];
    expect(sampleValues(rows, 0, 2)).toEqual(["a", "b"]);
    expect(sampleValues(rows, 1)).toEqual(["x", "y"]);
    expect(sampleValues(rows, 5)).toEqual([]);
  });
});
